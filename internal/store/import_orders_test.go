package store

import (
	"context"
	"encoding/json"
	"order_processor/internal/adapters/miaoshou/openapi"
	"order_processor/internal/domain/orders"
	"path/filepath"
	"testing"
)

func testAPIPackage(id, order, items string) json.RawMessage {
	return json.RawMessage(`{"opOrderPackageId":"` + id + `","platform":"tiktok","shopId":"shop","appPackageStatus":"wait_seller_send","orderInfo":{"platformOrderSn":"` + order + `"},"items":` + items + `}`)
}
func TestOpenAPIImportMergesRowsAndRollsBack(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, err := Create(db, "test")
	if err != nil {
		t.Fatal(err)
	}
	old := `{"opOrderPackageId":"1","items":[{"opOrderItemId":"b","opOrderPackageItemId":"pb","quantity":8,"refundQuantity":2,"skuSubCnName":"蓝色"},{"opOrderItemId":"a","opOrderPackageItemId":"pa","quantity":4,"refundQuantity":1,"skuSubCnName":"红色"},{"opOrderItemId":"gone","quantity":4,"refundQuantity":0}]}`
	if _, err = db.Exec(`INSERT INTO packages(account_id,package_id,payload,captured_at,source_file) VALUES(?,?,?,?,?)`, account, "1", old, "2026-01-01T00:00:00.000Z", "web.json"); err != nil {
		t.Fatal(err)
	}
	snapshot := miaoshouopenapi.PendingSnapshot{Packages: []json.RawMessage{testAPIPackage("1", "o1", `[{"opOrderItemId":"a","opOrderPackageItemId":"pa","quantity":5,"isGift":false},{"opOrderItemId":"b","opOrderPackageItemId":"pb","quantity":8,"isGift":true},{"opOrderItemId":"new","quantity":3}]`)}}
	result, err := importPendingFixture(context.Background(), db, account, "2026-01-02T00:00:00.000Z", "openapi/run", snapshot)
	if err != nil || result.SupplementedLines != 2 || result.MissingRefundLines != 1 || result.WaitPurchase != 1 {
		t.Fatal(result, err)
	}
	orders, err := Orders(db, account, "", 1, 100, "waitProcess", "waitPurchase")
	if err != nil {
		t.Fatal(err)
	}
	p := orders["items"].([]object)[0]
	items := objects(p["items"])
	if len(items) != 3 || text(items[0]["skuSubCnName"]) != "红色" || text(items[0]["refundQuantity"]) != "1" || text(items[1]["isTiktokGift"]) != "1" {
		t.Fatal("row supplements or gift normalization lost")
	}
	report := procurementSummary([]object{p}, false)
	if report["quantity"] != int64(7) || len(report["pendingRefundLines"].([]object)) != 0 {
		t.Fatal(report)
	}
	report = procurementSummary([]object{p}, true)
	if report["quantity"] != int64(13) {
		t.Fatal("gift total", report)
	}
	var n int
	if err = db.QueryRow("SELECT count(*) FROM order_items WHERE account_id=?", account).Scan(&n); err != nil || n != 3 {
		t.Fatal(n, err)
	}
	// A failure after the first package write must roll back the entire snapshot.
	snapshot.Packages = append(snapshot.Packages, testAPIPackage("2", "o2", `[{"opOrderItemId":"bad","quantity":"invalid"}]`))
	if _, err = importPendingFixture(context.Background(), db, account, "2026-01-03T00:00:00.000Z", "openapi/bad", snapshot); err == nil {
		t.Fatal("accepted invalid item")
	}
	var at string
	db.QueryRow("SELECT captured_at FROM packages WHERE account_id=? AND package_id='1'", account).Scan(&at)
	if at != "2026-01-02T00:00:00.000Z" {
		t.Fatal("partial transaction committed")
	}
	snapshot.Packages = snapshot.Packages[:1]
	if _, err = importPendingFixture(context.Background(), db, account, "2026-01-01T12:00:00.000Z", "openapi/stale", snapshot); err == nil {
		t.Fatal("accepted stale capture")
	}
	// Empty but complete pending data retires both scopes, preserving history.
	if _, err = importPendingFixture(context.Background(), db, account, "2026-01-04T00:00:00.000Z", "openapi/empty", miaoshouopenapi.PendingSnapshot{}); err != nil {
		t.Fatal(err)
	}
	orders, err = Orders(db, account, "", 1, 100, "waitProcess", "waitPurchase")
	if err != nil || orders["total"] != 0 {
		t.Fatal(orders, err)
	}
}

func TestImportBatchUsesCanonicalRecordID(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, _ := Create(db, "test")
	batch := orders.Batch{Source: "file", CapturedAt: "2026-01-01T00:00:00Z", Archive: "capture.json", Records: []orders.Record{{ID: "package-1", Fields: orders.Fields{"items": []any{map[string]any{"opOrderItemId": "item-1", "quantity": 1}}}}}}
	if _, err = ImportBatch(context.Background(), db, account, batch); err != nil {
		t.Fatal(err)
	}
	data, err := Orders(db, account, "", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	rows := data["items"].([]object)
	if len(rows) != 1 || rows[0]["opOrderPackageId"] != "package-1" || rows[0]["dataSource"] != "file" {
		t.Fatal("normalized record identity was not stored")
	}
}
