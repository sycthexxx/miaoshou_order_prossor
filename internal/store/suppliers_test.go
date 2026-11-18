package store

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestSupplierReplayAndManyProducts(t *testing.T) {
	dir := t.TempDir()
	db, e := Open(filepath.Join(dir, "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	account, _ := Create(db, "a")
	other, _ := Create(db, "b")
	write := func(file, at, name string) {
		t.Helper()
		body := fmt.Sprintf(`{"result":"success","opOrderPackageIdAndOutboundInfoMap":{"package":{"opOrderPackageId":"package","purchaseOrders":{"purchase":{"purchaseOrderId":"purchase","purchasePlatform":"1688","purchaseOrderSeller":%q,"platformOrderSn":"587170045979363177","purchaseOrderSn":"5127597578307025941"}}}}}`, name)
		raw, _ := json.Marshal(object{"accountId": account, "endpoint": outboundEndpoint, "capturedAt": at, "status": 200, "bodyBase64": base64.StdEncoding.EncodeToString([]byte(body))})
		p := filepath.Join(dir, "raw", account)
		os.MkdirAll(p, 0700)
		os.WriteFile(filepath.Join(p, file), raw, 0600)
		if _, e := Replay(db, dir); e != nil {
			t.Fatal(e)
		}
	}
	write("1.json", "2026-11-16T12:00:00Z", "正格服饰厂")
	write("2.json", "2026-11-16T13:00:00Z", "正格服饰厂")
	sellers, e := Suppliers(db, account)
	if e != nil || len(sellers) != 1 {
		t.Fatal(sellers, e)
	}
	sellerID := sellers[0]["id"].(int64)
	orders := sellers[0]["orders"].([]object)
	if len(orders) != 1 || orders[0]["purchaseNo"] != "5127597578307025941" {
		t.Fatal(orders)
	}
	tx, _ := db.Begin()
	e = saveOutbound(tx, account, "2026-11-16T14:00:00Z", "test", object{"opOrderPackageId": "package2", "purchaseOrders": object{"purchase2": object{"purchaseOrderId": "purchase2", "purchasePlatform": "1688", "purchaseOrderSeller": "正格服饰厂", "platformOrderSn": "order2", "purchaseOrderSn": "purchaseNo2"}}})
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	for _, product := range []string{"offer1", "offer2"} {
		tx, _ := db.Begin()
		e = saveProduct(tx, account, "2026", "test", object{"platform": "tiktok", "shopId": "shop", "platformItemId": product, "sourceList": []any{object{"source": "1688", "sourceItemId": product}}})
		if e != nil {
			t.Fatal(e)
		}
		tx.Commit()
		if e = AssignSupplier(db, account, "1688", "", product, sellerID); e != nil {
			t.Fatal(e)
		}
	}
	if e = AssignSupplier(db, account, "1688", "", "offer1", sellerID); e != nil {
		t.Fatal(e)
	}
	sellers, _ = Suppliers(db, account)
	if len(sellers) != 1 || len(sellers[0]["products"].([]object)) != 2 {
		t.Fatal(sellers)
	}
	if e = AssignSupplier(db, account, "1688", "", "offer1", 9999); e == nil {
		t.Fatal("unknown seller accepted")
	}
	if e = AssignSupplier(db, other, "1688", "", "offer1", sellerID); e == nil {
		t.Fatal("cross-account accepted")
	}
	if e = AssignSupplier(db, account, "1688", "", "unknown", sellerID); e == nil {
		t.Fatal("unknown source accepted")
	}
	report := object{"groups": []object{{"key": `source:["1688","","offer1"]`}, {"key": `source:["1688","","offer2"]`}}}
	if e = attachSuppliers(db, account, report); e != nil {
		t.Fatal(e)
	}
	for _, g := range report["groups"].([]object) {
		if g["supplier"].(object)["id"] != sellerID {
			t.Fatal(g)
		}
	}
	write("3.json", "2026-11-16T11:00:00Z", "旧名称")
	sellers, _ = Suppliers(db, account)
	if len(sellers) != 1 {
		t.Fatal("stale response created duplicate seller", sellers)
	}
	for _, s := range sellers {
		if s["id"] == sellerID && len(s["orders"].([]object)) != 2 {
			t.Fatal("older evidence overwrote latest")
		}
	}
	if e = AssignSupplier(db, account, "1688", "", "offer1", 0); e != nil {
		t.Fatal(e)
	}
	sellers, _ = Suppliers(db, account)
	for _, s := range sellers {
		if s["id"] == sellerID && len(s["products"].([]object)) != 1 {
			t.Fatal(s)
		}
	}
}
