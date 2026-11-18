package compare

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCasesEvidenceLatestAndDedup(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a")
	os.Mkdir(dir, 0700)
	write := func(name, scope, status, qty, refund string) {
		p := map[string]any{"opOrderPackageId": "1", "appPurchaseStatus": status, "orderInfo": map[string]any{"platformOrderSn": "fake"}, "items": map[string]any{"i": map[string]any{"opOrderItemId": "i", "quantity": qty, "refundQuantity": refund, "isGift": "0"}}}
		body, _ := json.Marshal(map[string]any{"result": "success", "packageList": []any{p}})
		b, _ := json.Marshal(map[string]any{"accountId": "a", "capturedAt": name, "endpoint": "/api/order/package/render_list/searchOrderPackageList", "status": 200, "requestBody": "source=orderProcess&appPackageTab=waitProcess&purchaseTab=" + scope, "bodyBase64": base64.StdEncoding.EncodeToString(body)})
		os.WriteFile(filepath.Join(dir, name+".json"), b, 0600)
	}
	write("01", "waitPurchase", "0", "3", "0")
	write("02", "hasPurchase", "2", "3", "1")
	write("03", "hasPurchase", "2", "4", "1")
	sample, cases, err := ReadCases(dir)
	if err != nil || len(sample.Packages) != 1 || sample.CapturedAt != "03" {
		t.Fatal(sample, cases, err)
	}
	for _, e := range cases {
		if !e.Found {
			t.Fatal(e)
		}
		if e.Category == "purchaseChanged" && (e.PreviousAt != "01" || e.CapturedAt != "02" || e.BeforeStatus != "0" || e.AfterStatus != "2") {
			t.Fatal(e)
		}
	}
	p, _ := Decode(sample.Packages[0])
	rows, _ := itemRecords(p)
	if String(rows[0].p["quantity"]) != "4" {
		t.Fatal("did not use latest package")
	}
}
func TestNoEvidenceDoesNotInventCases(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a")
	os.Mkdir(dir, 0700)
	sample, cases, err := ReadCases(dir)
	if err != nil || len(sample.Packages) != 0 || len(cases) != 3 {
		t.Fatal(sample, cases, err)
	}
	for _, e := range cases {
		if e.Found || e.Reason == "" {
			t.Fatal(e)
		}
	}
}
