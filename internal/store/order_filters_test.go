package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestOrderStatusFilterCountsAndPagination(t *testing.T) {
	db, e := Open(filepath.Join(t.TempDir(), "orders.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	account, _ := Create(db, "a")
	other, _ := Create(db, "b")
	add := func(a, id, state, op, isolation string, refund bool) {
		t.Helper()
		p := object{"opOrderPackageId": id, "appPackageStatus": state, "appPackageOperateStatus": op, "isIsolation": isolation, "items": []object{{"opOrderItemId": id, "isRefund": "0"}}}
		if refund {
			p["items"].([]object)[0]["isRefund"] = "1"
		}
		b, _ := json.Marshal(p)
		if _, e := db.Exec(`INSERT INTO packages(account_id,package_id,payload,captured_at,source_file) VALUES(?,?,?,'2026','test')`, a, id, string(b)); e != nil {
			t.Fatal(e)
		}
	}
	add(account, "one", "wait_seller_send", "wait_audit", "0", false)
	add(account, "two", "wait_seller_send", "wait_process", "0", false)
	add(account, "three", "wait_seller_send", "wait_ship", "0", false)
	add(account, "four", "wait_seller_send", "ship_success", "0", false)
	add(account, "five", "wait_seller_send", "ship_fail", "0", false)
	add(account, "six", "wait_receiver_confirm", "ship_success", "0", false)
	add(account, "seven", "wait_seller_send", "wait_audit", "1", false)
	add(account, "eight", "finished", "ship_success", "0", true)
	add(account, "nine", "cancelled", "wait_audit", "0", false)
	add(account, "ten", "returned", "ship_success", "0", false)
	add(account, "eleven", "unpaid", "wait_audit", "0", false)
	add(other, "outside", "wait_seller_send", "wait_audit", "0", false)
	expected := map[string]int{"all": 11, "waitProcess": 2, "waitShip": 1, "submitPlatform": 2, "waitReceiverConfirm": 1, "isolation": 1, "closed": 3}
	for status, count := range expected {
		d, e := Orders(db, account, "", 1, 20, status)
		if e != nil {
			t.Fatal(e)
		}
		if d["total"] != count {
			t.Fatalf("%s total=%v", status, d["total"])
		}
		for k, v := range expected {
			if d["counts"].(map[string]int)[k] != v {
				t.Fatalf("%s counts=%v", status, d["counts"])
			}
		}
	}
	d, e := Orders(db, account, "one", 1, 20, "waitProcess")
	if e != nil || d["total"] != 1 || d["counts"].(map[string]int)["waitProcess"] != 2 {
		t.Fatal(d, e)
	}
	d, e = Orders(db, account, "", 2, 1, "waitProcess")
	if e != nil || len(d["items"].([]object)) != 1 || d["total"] != 2 {
		t.Fatal(d, e)
	}
}

func TestPendingPurchaseTabs(t *testing.T) {
	db, e := Open(filepath.Join(t.TempDir(), "pending.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	account, _ := Create(db, "pending")
	for _, v := range []struct{ id, state, purchase string }{{"a", "wait_seller_send", "0"}, {"b", "wait_seller_send", "1"}, {"c", "wait_seller_send", "2"}, {"d", "wait_receiver_confirm", "2"}} {
		p := object{"opOrderPackageId": v.id, "appPackageStatus": v.state, "appPackageOperateStatus": "wait_process", "appPurchaseStatus": v.purchase, "items": []object{}}
		b, _ := json.Marshal(p)
		if _, e := db.Exec(`INSERT INTO packages(account_id,package_id,payload,captured_at,source_file) VALUES(?,?,?,'2026','test')`, account, v.id, string(b)); e != nil {
			t.Fatal(e)
		}
	}
	for tab, want := range map[string]int{"waitPurchase": 1, "hasPurchase": 2} {
		d, e := Orders(db, account, "", 1, 20, "waitProcess", tab)
		if e != nil {
			t.Fatal(e)
		}
		c := d["counts"].(map[string]int)
		if d["total"] != want || d["purchaseTab"] != tab || c["waitProcess"] != 3 || c["waitPurchase"] != 1 || c["hasPurchase"] != 2 {
			t.Fatal(d)
		}
	}
	d, e := Orders(db, account, `"opOrderPackageId":"a"`, 1, 20, "waitProcess", "hasPurchase")
	if e != nil || d["total"] != 0 || d["counts"].(map[string]int)["hasPurchase"] != 2 {
		t.Fatal(d, e)
	}
	d, e = Orders(db, account, "", 1, 20, "all", "hasPurchase")
	if e != nil || d["total"] != 4 || d["purchaseTab"] != "" {
		t.Fatal(d, e)
	}
}
