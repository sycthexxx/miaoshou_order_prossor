package store

import (
	"encoding/json"
	"testing"
)

func TestSourceStatusIndependentAndManualInvalidProtected(t *testing.T) {
	db, a, b := captureFixture(t)
	captureProduct(t, db, a, "100", "")
	captureProduct(t, db, b, "100", "")
	status, err := GetSourceStatus(db, a, "100")
	if err != nil || status.State != "unknown" || status.Source != "" {
		t.Fatal(status, err)
	}
	supplier, err := CaptureSupplier(db, 1, a, "100", "原厂家")
	if err != nil {
		t.Fatal(err)
	}
	mapping, err := SaveProcurementMapping(db, 1, a, "100", 0, sampleMapping("黑色"))
	if err != nil {
		t.Fatal(err)
	}
	set := func(in SourceStatusInput) SourceStatusResult {
		t.Helper()
		r, e := SetSourceStatus(db, a, "100", in)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	if !set(SourceStatusInput{State: "valid", Source: "plugin"}).Changed {
		t.Fatal("first valid not changed")
	}
	invalid := set(SourceStatusInput{State: "invalid", Source: "manual", Reason: "手工确认"})
	if !invalid.Changed {
		t.Fatal(invalid)
	}
	blocked := set(SourceStatusInput{State: "valid", Source: "plugin"})
	if !blocked.RetainedManual || blocked.Changed || blocked.Status != invalid.Status {
		t.Fatal(blocked)
	}
	blocked = set(SourceStatusInput{State: "invalid", Source: "plugin", Evidence: "offer_removed", Reason: "自动下架"})
	if blocked.Status != invalid.Status {
		t.Fatal("plugin overwrote manual reason", blocked)
	}
	if set(SourceStatusInput{State: "unknown", Source: "plugin"}).Status != invalid.Status {
		t.Fatal("inconclusive page cleared status")
	}
	restored := set(SourceStatusInput{State: "valid", Source: "manual"})
	if !restored.Changed || restored.Status.State != "valid" {
		t.Fatal(restored)
	}
	repeated := set(SourceStatusInput{State: "valid", Source: "manual"})
	if repeated.Changed || repeated.Status.CheckedAt == "" {
		t.Fatal(repeated)
	}
	if _, err = SetSourceStatus(db, a, "100", SourceStatusInput{State: "invalid", Source: "plugin", Reason: "网络失败"}); err == nil {
		t.Fatal("unconfirmed invalid accepted")
	}
	invalid = set(SourceStatusInput{State: "invalid", Source: "plugin", Reason: "商品已下架", Evidence: "offer_removed"})
	captureProduct(t, db, a, "100", "")
	status, err = GetSourceStatus(db, a, "100")
	if err != nil || status != invalid.Status {
		t.Fatal("sync replaced status", status, err)
	}
	captureProduct(t, db, a, "200", "")
	status, _ = GetSourceStatus(db, a, "200")
	if status.State != "unknown" {
		t.Fatal(status)
	}
	status, _ = GetSourceStatus(db, b, "100")
	if status.State != "unknown" {
		t.Fatal("cross-account state", status)
	}
	if _, err = SetSourceStatus(db, b, "200", SourceStatusInput{State: "valid", Source: "manual"}); err == nil {
		t.Fatal("foreign source accepted")
	}
	var sid int64
	db.QueryRow(`SELECT supplier_id FROM source_suppliers WHERE account_id=? AND product_id='100'`, a).Scan(&sid)
	if sid != supplier["supplierId"].(int64) {
		t.Fatal("supplier changed")
	}
	after, err := GetProcurementMapping(db, a, "100")
	beforeJSON, _ := json.Marshal(mapping)
	afterJSON, _ := json.Marshal(after)
	if err != nil || string(beforeJSON) != string(afterJSON) {
		t.Fatal("mapping changed", err)
	}
	var seq int
	var dbName, path string
	if err = db.QueryRow("PRAGMA database_list").Scan(&seq, &dbName, &path); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	persisted, err := GetSourceStatus(reopened, a, "100")
	if err != nil || persisted != invalid.Status {
		t.Fatal("restart lost independent state", persisted, err)
	}
	var rows int
	db.QueryRow(`SELECT count(*) FROM source_statuses WHERE account_id=? AND offer_id='100'`, a).Scan(&rows)
	if rows != 1 {
		t.Fatal("status did not upsert", rows)
	}
}
func TestInvalidSourceKeepsDemandTotals(t *testing.T) {
	db, a, _ := captureFixture(t)
	captureProduct(t, db, a, "100", "")
	payload := `{"opOrderPackageId":"1","appPackageStatus":"wait_seller_send","appPackageOperateStatus":"wait_audit","appPurchaseStatus":"0","items":[{"opOrderItemId":"i","platform":"tiktok","shopId":"shop","platformItemId":"100","skuSubName":"红,M","quantity":"3"}]}`
	if _, err := db.Exec(`INSERT INTO packages(account_id,package_id,payload,captured_at,source_file) VALUES(?,'1',?,'2026','fixture')`, a, payload); err != nil {
		t.Fatal(err)
	}
	before, err := Procurement(db, a, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = SetSourceStatus(db, a, "100", SourceStatusInput{State: "invalid", Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	after, err := Procurement(db, a, false)
	if err != nil || after["quantity"] != before["quantity"] || after["packages"] != before["packages"] {
		t.Fatal(after, err)
	}
	groups := after["groups"].([]object)
	if len(groups) != 1 || groups[0]["sourceStatus"].(SourceStatus).State != "invalid" {
		t.Fatal(groups)
	}
}
