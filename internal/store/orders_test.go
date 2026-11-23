package store

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyMigrationReplay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, e := sql.Open("sqlite3", path)
	if e != nil {
		t.Fatal(e)
	}
	_, e = db.Exec(`CREATE TABLE accounts(id TEXT PRIMARY KEY,name TEXT NOT NULL UNIQUE,created_at TEXT NOT NULL);INSERT INTO accounts VALUES('aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','legacy','2026-01-01');`)
	if e != nil {
		t.Fatal(e)
	}
	db.Close()
	db, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	a, _ := Accounts(db)
	if len(a) != 1 || a[0].ID != "1" || a[0].StorageKey != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatal(a)
	}
	write := func(file, endpoint, body string) {
		t.Helper()
		env := map[string]any{"accountId": a[0].StorageKey, "endpoint": endpoint, "status": 200, "capturedAt": "2026-11-16T05:00:00Z", "bodyBase64": base64.StdEncoding.EncodeToString([]byte(body))}
		b, _ := json.Marshal(env)
		p := filepath.Join(dir, "raw", a[0].StorageKey)
		os.MkdirAll(p, 0700)
		if e = os.WriteFile(filepath.Join(p, file), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	write("1.json", translationEndpoint, `{"result":"success","packageList":[{"opOrderPackageId":"123","items":{"4":{"opOrderItemId":"4","skuSubCnName":"酒红色,M"}}}]}`)
	write("2.json", listEndpoint, `{"result":"success","packageList":[{"opOrderPackageId":"123","opOrderId":"9007199254740999","orderInfo":{"platformOrderSn":"90071992547409999"},"items":{"4":{"opOrderItemId":"4","platformSkuId":"90071992547409999","quantity":"1","title":"shirt","skuSubName":"Rojo vino, M"}}}]}`)
	if n, e := Replay(db, dir); e != nil || n != 2 {
		t.Fatal(n, e)
	}
	if n, e := Replay(db, dir); e != nil || n != 0 {
		t.Fatal(n, e)
	}
	for _, table := range []string{"orders", "packages", "order_items", "translations"} {
		var n int
		if e = db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); e != nil || n != 1 {
			t.Fatal(table, n, e)
		}
		var schema string
		db.QueryRow("SELECT sql FROM sqlite_master WHERE name=?", table).Scan(&schema)
	}
	result, e := Orders(db, "1", "酒红色", 1, 20)
	if e != nil || result["total"].(int) != 1 {
		t.Fatal(result, e)
	}
	row := result["items"].([]object)[0]
	item := row["items"].([]object)[0]
	if item["skuSubCnName"] != "酒红色,M" || text(item["platformSkuId"]) != "90071992547409999" {
		t.Fatal(item)
	}
	second, e := Create(db, "second")
	if e != nil || second != "2" {
		t.Fatal(second, e)
	}
	result, e = Orders(db, second, "", 1, 20)
	if e != nil || result["total"].(int) != 0 {
		t.Fatal(result, e)
	}
}

// A second response is a new archive, but must update the existing business rows.
func TestRepeatedOrderUpdatesPreserveIDs(t *testing.T) {
	dir := t.TempDir()
	db, e := Open(filepath.Join(dir, "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	account, e := Create(db, "update-test")
	if e != nil {
		t.Fatal(e)
	}
	write := func(file, at, status, quantity string) {
		t.Helper()
		body := fmt.Sprintf(`{"result":"success","packageList":[{"opOrderPackageId":"pkg1","opOrderId":"order1","appPackageStatusText":%q,"orderInfo":{"platformOrderSn":"platform1","status":%q},"items":{"item1":{"opOrderItemId":"item1","platformSkuId":"sku1","quantity":%q,"title":"衣服","skuSubName":"红色,M"}}}]}`, status, status, quantity)
		env := map[string]any{"accountId": account, "endpoint": listEndpoint, "status": 200, "capturedAt": at, "bodyBase64": base64.StdEncoding.EncodeToString([]byte(body))}
		raw, _ := json.Marshal(env)
		p := filepath.Join(dir, "raw", account)
		if e := os.MkdirAll(p, 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(filepath.Join(p, file), raw, 0600); e != nil {
			t.Fatal(e)
		}
		if _, e := Replay(db, dir); e != nil {
			t.Fatal(e)
		}
	}
	write("1.json", "2026-11-16T01:00:00Z", "待处理", "1")
	ids := map[string]int64{}
	for _, table := range []string{"orders", "packages", "order_items"} {
		var id int64
		if e := db.QueryRow("SELECT id FROM " + table).Scan(&id); e != nil {
			t.Fatal(e)
		}
		ids[table] = id
	}
	write("2.json", "2026-11-16T02:00:00Z", "已发货", "3")
	// Late replay of an older file must not roll back the updated order.
	write("3.json", "2026-11-16T00:00:00Z", "待处理", "1")
	for table, id := range ids {
		var count int
		var current int64
		if e := db.QueryRow("SELECT count(*),min(id) FROM "+table).Scan(&count, &current); e != nil {
			t.Fatal(e)
		}
		if count != 1 || current != id {
			t.Fatalf("%s: count=%d id=%d want=%d", table, count, current, id)
		}
	}
	var quantity int
	if e := db.QueryRow("SELECT quantity FROM order_items").Scan(&quantity); e != nil || quantity != 3 {
		t.Fatal(quantity, e)
	}
	result, e := Orders(db, account, "", 1, 20)
	if e != nil {
		t.Fatal(e)
	}
	if result["items"].([]object)[0]["appPackageStatusText"] != "已发货" {
		t.Fatal(result)
	}
	var archives int
	db.QueryRow("SELECT count(*) FROM imported_archives").Scan(&archives)
	if archives != 3 {
		t.Fatal(archives)
	}
}

func TestReplayOrderLogistics(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, err := Create(db, "logistics")
	if err != nil {
		t.Fatal(err)
	}
	key := StorageKey(db, account)
	rawDir := filepath.Join(dir, "raw", key)
	if err = os.MkdirAll(rawDir, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(file, endpoint, at, body string) {
		t.Helper()
		raw, err := json.Marshal(map[string]any{"accountId": key, "endpoint": endpoint, "status": 200, "capturedAt": at, "bodyBase64": base64.StdEncoding.EncodeToString([]byte(body))})
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(rawDir, file), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("1.json", listEndpoint, "2026-11-23T00:00:00Z", `{"result":"success","packageList":[{"opOrderPackageId":"7","platform":"tiktok","platformPackageStatus":"FULFILLING","logisticsNo":"DCS123","items":[]}]}`)
	write("2.json", outboundEndpoint, "2026-11-23T02:00:00Z", `{"result":"success","opOrderPackageIdAndOutboundInfoMap":{"7":{"opOrderPackageId":"7","items":{"55":{"opOrderItemId":"55","purchaseOrderId":"98","relatePurchaseOrderIds":["98"]}},"purchaseOrders":{"98":{"purchaseOrderId":"98","purchaseOrderSn":"5127948757295409109","purchasePlatformName":"1688","purchaseOrderStatus":"has_sign","purchaseOrderPackages":{"1":{"purchaseOrderLogisticsName":"中通快递","purchaseOrderWaybillCode":"79050020721591"},"2":{"purchaseOrderLogisticsName":"顺丰","purchaseOrderWaybillCode":"123456"}}},"99":{"purchaseOrderId":"99","isUnrelated":"1"}}}}}`)
	write("3.json", outboundEndpoint, "2026-11-23T01:00:00Z", `{"result":"success","opOrderPackageIdAndOutboundInfoMap":{"7":{"opOrderPackageId":"7","purchaseOrders":{}}}}`)
	if _, err = Replay(db, dir); err != nil {
		t.Fatal(err)
	}
	result, err := Orders(db, account, "", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	row := result["items"].([]object)[0]
	var purchases []firstMilePurchase
	if err = json.Unmarshal(row["firstMilePurchases"].(json.RawMessage), &purchases); err != nil {
		t.Fatal(err)
	}
	if row["platformPackageStatus"] != "FULFILLING" || row["logisticsNo"] != "DCS123" || len(purchases) != 1 || purchases[0].Status != "has_sign" || len(purchases[0].Shipments) != 2 ||
		len(purchases[0].ItemIDs) != 1 || purchases[0].ItemIDs[0] != "55" {
		t.Fatal(row, purchases)
	}
}
