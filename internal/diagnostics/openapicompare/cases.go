package compare

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

type CaseEvidence struct {
	LatestLocalAt string `json:"latestLocalAt,omitempty"`
	Category      string `json:"category"`
	Found         bool   `json:"found"`
	Reason        string `json:"reason,omitempty"`
	PackageID     string `json:"packageId,omitempty"`
	CapturedAt    string `json:"capturedAt,omitempty"`
	PreviousAt    string `json:"previousAt,omitempty"`
	BeforeStatus  string `json:"beforePurchaseStatus,omitempty"`
	AfterStatus   string `json:"afterPurchaseStatus,omitempty"`
	BeforeScope   string `json:"beforeScope,omitempty"`
	AfterScope    string `json:"afterScope,omitempty"`
	ItemID        string `json:"itemId,omitempty"`
	Quantity      string `json:"quantity,omitempty"`
	Refund        string `json:"refundQuantity,omitempty"`
	RefundField   string `json:"refundField,omitempty"`
}
type observation struct {
	raw       json.RawMessage
	p         Object
	at, scope string
}

// ReadCases selects at most one evidenced representative for each case. It
// deduplicates package IDs before any API request and never infers a refund.
func ReadCases(dir string) (Sample, []CaseEvidence, error) {
	sample := Sample{Account: filepath.Base(filepath.Clean(dir)), Packages: []json.RawMessage{}}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return sample, nil, err
	}
	observations := []observation{}
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			return sample, nil, errors.New("cannot read local archive")
		}
		var env struct {
			Account  string `json:"accountId"`
			At       string `json:"capturedAt"`
			Endpoint string `json:"endpoint"`
			Status   int    `json:"status"`
			Request  string `json:"requestBody"`
			Body     string `json:"bodyBase64"`
		}
		if json.Unmarshal(b, &env) != nil || env.Account != sample.Account || env.Status != 200 || env.Endpoint != "/api/order/package/render_list/searchOrderPackageList" {
			continue
		}
		body, err := base64.StdEncoding.DecodeString(env.Body)
		if err != nil {
			continue
		}
		var d struct {
			Result string            `json:"result"`
			Rows   []json.RawMessage `json:"packageList"`
		}
		if json.Unmarshal(body, &d) != nil || d.Result != "success" {
			continue
		}
		q, err := url.ParseQuery(env.Request)
		if err != nil {
			continue
		}
		scope := ""
		if q.Get("source") == "orderProcess" && q.Get("appPackageTab") == "waitProcess" {
			scope = q.Get("purchaseTab")
			if scope != "waitPurchase" && scope != "hasPurchase" {
				scope = ""
			}
		}
		for _, raw := range d.Rows {
			p, err := Decode(raw)
			if err != nil || String(p["opOrderPackageId"]) == "" || String(Value(p, "orderInfo.platformOrderSn")) == "" {
				continue
			}
			observations = append(observations, observation{raw: raw, p: p, at: env.At, scope: scope})
		}
	}
	sort.SliceStable(observations, func(i, j int) bool { return observations[i].at < observations[j].at })
	latest := map[string]observation{}
	cases := map[string]CaseEvidence{}
	refundPriority := 2
	for _, o := range observations {
		id := String(o.p["opOrderPackageId"])
		status := String(o.p["appPurchaseStatus"])
		if o.scope == "hasPurchase" && o.at > cases["hasPurchase"].CapturedAt {
			cases["hasPurchase"] = CaseEvidence{Category: "hasPurchase", Found: true, PackageID: id, CapturedAt: o.at, AfterStatus: status, AfterScope: o.scope}
		}
		if prev, ok := latest[id]; ok && o.at > prev.at {
			before := String(prev.p["appPurchaseStatus"])
			changedStatus := before != "" && status != "" && before != status
			changedScope := prev.scope != "" && o.scope != "" && prev.scope != o.scope
			if (changedStatus || changedScope) && o.at > cases["purchaseChanged"].CapturedAt {
				cases["purchaseChanged"] = CaseEvidence{Category: "purchaseChanged", Found: true, PackageID: id, CapturedAt: o.at, PreviousAt: prev.at, BeforeStatus: before, AfterStatus: status, BeforeScope: prev.scope, AfterScope: o.scope}
			}
		}
		latest[id] = o
		items, _ := itemRecords(o.p)
		for _, item := range items {
			q, err := strconv.ParseInt(String(item.p["quantity"]), 10, 64)
			if err != nil || q <= 0 {
				continue
			}
			for _, field := range []string{"refundQuantity", "refundQty"} {
				refund, err := strconv.ParseInt(String(item.p[field]), 10, 64)
				if err != nil || refund <= 0 || refund >= q {
					continue
				}
				priority := 1
				if item.identity.Gift == "regular" {
					priority = 0
				}
				if priority < refundPriority || (priority == refundPriority && o.at > cases["partialRefund"].CapturedAt) {
					refundPriority = priority
					cases["partialRefund"] = CaseEvidence{Category: "partialRefund", Found: true, PackageID: id, CapturedAt: o.at, ItemID: item.identity.RowID, Quantity: strconv.FormatInt(q, 10), Refund: strconv.FormatInt(refund, 10), RefundField: field}
				}
			}
		}
	}
	evidence := []CaseEvidence{}
	seen := map[string]bool{}
	for _, category := range []string{"hasPurchase", "purchaseChanged", "partialRefund"} {
		e, ok := cases[category]
		if !ok {
			e = CaseEvidence{Category: category, Reason: "no matching evidence in readable local archives"}
		}
		if e.Found {
			e.LatestLocalAt = latest[e.PackageID].at
		}
		evidence = append(evidence, e)
		if !e.Found || seen[e.PackageID] {
			continue
		}
		seen[e.PackageID] = true
		// Query against the latest observed representation, while retaining the
		// historical timestamps that establish the case independently.
		o := latest[e.PackageID]
		sample.Packages = append(sample.Packages, o.raw)
		if o.at > sample.CapturedAt {
			sample.CapturedAt = o.at
		}
	}
	return sample, evidence, nil
}
