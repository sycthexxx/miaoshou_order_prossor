// Package compare compares only explicitly mapped, non-buyer package fields.
package compare

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Object map[string]any
type Sample struct {
	Account    string
	CapturedAt string
	Packages   []json.RawMessage
}

func Decode(raw []byte) (Object, error) {
	var p Object
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.UseNumber()
	err := d.Decode(&p)
	return p, err
}
func String(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case nil:
		return ""
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}
func Value(p Object, path string) any {
	var v any = map[string]any(p)
	for _, part := range strings.Split(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[part]
	}
	return v
}

// ReadSample reads one explicitly selected account directory, never writes it,
// and takes the newest valid pending/not-purchased archive.
func ReadSample(dir string, limit int) (Sample, error) {
	var result Sample
	if limit < 1 || limit > 5 {
		return result, errors.New("sample limit must be 1..5")
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return result, err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			return result, errors.New("cannot read local archive")
		}
		var env struct {
			Account  string `json:"accountId"`
			At       string `json:"capturedAt"`
			Endpoint string `json:"endpoint"`
			Status   int    `json:"status"`
			Request  string `json:"requestBody"`
			Body     string `json:"bodyBase64"`
		}
		if json.Unmarshal(b, &env) != nil || env.Account != filepath.Base(filepath.Clean(dir)) || env.Status != 200 || env.Endpoint != "/api/order/package/render_list/searchOrderPackageList" {
			continue
		}
		q, err := url.ParseQuery(env.Request)
		if err != nil || q.Get("source") != "orderProcess" || q.Get("appPackageTab") != "waitProcess" || q.Get("purchaseTab") != "waitPurchase" {
			continue
		}
		body, err := base64.StdEncoding.DecodeString(env.Body)
		if err != nil {
			continue
		}
		var data struct {
			Result   string            `json:"result"`
			Packages []json.RawMessage `json:"packageList"`
		}
		if json.Unmarshal(body, &data) != nil || data.Result != "success" {
			continue
		}
		result = Sample{Account: env.Account, CapturedAt: env.At, Packages: []json.RawMessage{}}
		for _, raw := range data.Packages {
			p, err := Decode(raw)
			if err != nil || String(p["opOrderPackageId"]) == "" || String(Value(p, "orderInfo.platformOrderSn")) == "" {
				continue
			}
			result.Packages = append(result.Packages, raw)
			if len(result.Packages) == limit {
				break
			}
		}
		if len(result.Packages) > 0 {
			return result, nil
		}
	}
	return result, errors.New("no usable pending/not-purchased archive in selected account directory")
}

type Difference struct {
	Field  string `json:"field"`
	Local  string `json:"local,omitempty"`
	Remote string `json:"remote,omitempty"`
	Kind   string `json:"kind"`
}
type FieldCheck struct {
	Field  string `json:"field"`
	Status string `json:"status"`
}
type ItemIdentity struct {
	Source       string `json:"source"`
	RowID        string `json:"opOrderItemId,omitempty"`
	PackageRowID string `json:"opOrderPackageItemId,omitempty"`
	ProductID    string `json:"platformItemId,omitempty"`
	SKUID        string `json:"platformSkuId,omitempty"`
	Spec         string `json:"spec,omitempty"`
	Quantity     string `json:"quantity,omitempty"`
	Gift         string `json:"gift"`
}
type ItemMatch struct {
	Local  ItemIdentity `json:"local"`
	Remote ItemIdentity `json:"remote"`
	Method string       `json:"method"`
}
type CollectionCounts struct {
	Items     int `json:"items"`
	GiftItems int `json:"giftItems"`
	Invalid   int `json:"invalidEntries"`
}
type ItemCoverage struct {
	Local           CollectionCounts `json:"local"`
	Remote          CollectionCounts `json:"remote"`
	Matched         int              `json:"matched"`
	Matches         []ItemMatch      `json:"matches"`
	UnmatchedLocal  []ItemIdentity   `json:"unmatchedLocal"`
	UnmatchedRemote []ItemIdentity   `json:"unmatchedRemote"`
}
type Report struct {
	PackageID   string       `json:"packageId"`
	Channel     string       `json:"channel"`
	Found       bool         `json:"found"`
	Compared    int          `json:"compared"`
	Differences []Difference `json:"differences"`
	Fields      []FieldCheck `json:"fields"`
	Items       ItemCoverage `json:"itemCoverage"`
}
type itemRecord struct {
	p        Object
	identity ItemIdentity
}

func itemRecords(p Object) ([]itemRecord, CollectionCounts) {
	out := []itemRecord{}
	counts := CollectionCounts{}
	for _, field := range []string{"items", "giftItems"} {
		rows := []any{}
		switch v := p[field].(type) {
		case []any:
			rows = v
		case map[string]any:
			keys := []string{}
			for k := range v {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				rows = append(rows, v[k])
			}
		case nil:
		default:
			counts.Invalid++
		}
		if field == "items" {
			counts.Items = len(rows)
		} else {
			counts.GiftItems = len(rows)
		}
		for _, v := range rows {
			m, ok := v.(map[string]any)
			if !ok {
				counts.Invalid++
				continue
			}
			gift := "unknown"
			if field == "giftItems" || normalized("isGift", m["isGift"]) == "1" || normalized("isTiktokGift", m["isTiktokGift"]) == "1" {
				gift = "gift"
			} else if normalized("isGift", m["isGift"]) == "0" || normalized("isTiktokGift", m["isTiktokGift"]) == "0" {
				gift = "regular"
			}
			identity := ItemIdentity{Source: field, RowID: String(m["opOrderItemId"]), PackageRowID: String(m["opOrderPackageItemId"]), ProductID: String(m["platformItemId"]), SKUID: String(m["platformSkuId"]), Spec: bounded(String(m["skuSubName"])), Quantity: String(m["quantity"]), Gift: gift}
			out = append(out, itemRecord{p: m, identity: identity})
		}
	}
	return out, counts
}

// LocalItemCoverage permits a local-only diagnostic without exposing a full item.
func LocalItemCoverage(p Object) ItemCoverage {
	rows, counts := itemRecords(p)
	out := ItemCoverage{Local: counts, Matches: []ItemMatch{}, UnmatchedLocal: []ItemIdentity{}, UnmatchedRemote: []ItemIdentity{}}
	for _, r := range rows {
		out.UnmatchedLocal = append(out.UnmatchedLocal, r.identity)
	}
	return out
}

func bounded(s string) string {
	r := []rune(s)
	if len(r) > 256 {
		return string(r[:256]) + "…"
	}
	return s
}
func normalized(field string, v any) string {
	s := strings.TrimSpace(String(v))
	if strings.HasSuffix(field, "skuSubName") {
		s = strings.ReplaceAll(s, "，", ",")
		parts := strings.Split(s, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return strings.Join(parts, ",")
	}
	if strings.HasSuffix(field, "isGift") || strings.HasSuffix(field, "isTiktokGift") {
		if s == "true" {
			return "1"
		}
		if s == "false" {
			return "0"
		}
	}
	return s
}
func Compare(local, remote Object, channel string) Report {
	r := Report{PackageID: String(local["opOrderPackageId"]), Channel: channel, Found: remote != nil, Differences: []Difference{}, Fields: []FieldCheck{}, Items: LocalItemCoverage(local)}
	if remote == nil {
		return r
	}
	check := func(field string, a, b any) {
		status := "equal"
		if a == nil && b == nil {
			status = "missingBoth"
		} else if a == nil {
			status = "missingLocal"
		} else if b == nil {
			status = "missingRemote"
		} else {
			r.Compared++
			if normalized(field, a) != normalized(field, b) {
				status = "different"
			}
		}
		r.Fields = append(r.Fields, FieldCheck{Field: field, Status: status})
		if status == "different" {
			r.Differences = append(r.Differences, Difference{Field: field, Local: bounded(normalized(field, a)), Remote: bounded(normalized(field, b)), Kind: status})
		} else if status == "missingLocal" || status == "missingRemote" {
			r.Differences = append(r.Differences, Difference{Field: field, Kind: status})
		}
	}
	for _, field := range []string{"opOrderPackageId", "appPackageNo", "platform", "site", "shopId", "appPackageStatus", "appPackageOperateStatus", "appPurchaseStatus", "orderInfo.opOrderId", "orderInfo.platformOrderSn", "orderInfo.platformOrderStatus"} {
		check(field, Value(local, field), Value(remote, field))
	}
	l, lc := itemRecords(local)
	rr, rc := itemRecords(remote)
	r.Items = ItemCoverage{Local: lc, Remote: rc, Matches: []ItemMatch{}, UnmatchedLocal: []ItemIdentity{}, UnmatchedRemote: []ItemIdentity{}}
	used := map[int]bool{}
	matched := map[int]bool{}
	fields := []string{"opOrderItemId", "opOrderPackageItemId", "platformItemId", "platformSkuId", "skuSubName", "skuSubCnName", "quantity", "refundQuantity", "picUrl", "originalPicUrl", "isGift", "isTiktokGift"}
	compatible := func(a, b itemRecord) bool {
		return a.identity.Gift == b.identity.Gift || a.identity.Gift == "unknown" || b.identity.Gift == "unknown"
	}
	key := func(row itemRecord, method string) string {
		switch method {
		case "packageRowID":
			return row.identity.PackageRowID
		case "orderRowID":
			return row.identity.RowID
		default:
			if row.identity.ProductID == "" || row.identity.SKUID == "" || row.identity.Gift == "unknown" {
				return ""
			}
			return row.identity.Gift + "/" + row.identity.ProductID + "/" + row.identity.SKUID
		}
	}
	for _, method := range []string{"packageRowID", "orderRowID", "uniqueProductSKU"} {
		if method == "uniqueProductSKU" && (String(local["shopId"]) == "" || String(local["shopId"]) != String(remote["shopId"])) {
			continue
		}
		for i, a := range l {
			if matched[i] {
				continue
			}
			k := key(a, method)
			if k == "" {
				continue
			}
			nl, nr, index := 0, 0, -1
			for _, x := range l {
				if key(x, method) == k {
					nl++
				}
			}
			for j, b := range rr {
				if key(b, method) == k && compatible(a, b) {
					nr++
					index = j
				}
			}
			if nl != 1 || nr != 1 || used[index] {
				continue
			}
			matched[i] = true
			used[index] = true
			r.Items.Matched++
			r.Items.Matches = append(r.Items.Matches, ItemMatch{Local: a.identity, Remote: rr[index].identity, Method: method})
			for _, field := range fields {
				check("items["+itoa(i)+"]."+field, a.p[field], rr[index].p[field])
			}
		}
	}
	for i, a := range l {
		if matched[i] {
			continue
		}
		r.Items.UnmatchedLocal = append(r.Items.UnmatchedLocal, a.identity)
		r.Differences = append(r.Differences, Difference{Field: "items[" + itoa(i) + "]", Kind: "itemUnmatchedOrAmbiguous"})
		for _, field := range fields {
			r.Fields = append(r.Fields, FieldCheck{Field: "items[" + itoa(i) + "]." + field, Status: "notComparedUnmatched"})
		}
	}
	for j, b := range rr {
		if !used[j] {
			r.Items.UnmatchedRemote = append(r.Items.UnmatchedRemote, b.identity)
		}
	}
	if len(r.Items.UnmatchedRemote) > 0 {
		r.Differences = append(r.Differences, Difference{Field: "items", Kind: "unmatchedRemoteItems"})
	}
	return r
}

func itoa(i int) string              { b, _ := json.Marshal(i); return string(b) }
func Print(w io.Writer, v any) error { return json.NewEncoder(w).Encode(v) }
