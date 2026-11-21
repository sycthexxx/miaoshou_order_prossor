package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestIncrementalOrderReplayAndScopePrecedence(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ac, err := Create(db, "orders")
	if err != nil {
		t.Fatal(err)
	}
	other, err := Create(db, "other")
	if err != nil {
		t.Fatal(err)
	}
	account := Account{ID: ac, StorageKey: StorageKey(db, ac)}
	setScope := func(scope, at string, ids ...string) {
		t.Helper()
		tx, e := db.Begin()
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback()
		if e = saveOrderScope(tx, ac, scope, at, []string{"verified snapshot"}, ids); e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(); e != nil {
			t.Fatal(e)
		}
	}
	num := "587279992876074977"
	p := object{"platformOrderSn": num, "subPackageList": []object{{"orderInfo": object{"opOrderId": "mergedOrder", "platformOrderSn": "childNumber"}}}, "opOrderPackageId": "1", "appPackageStatus": "wait_seller_send", "appPackageOperateStatus": "wait_audit", "appPurchaseStatus": "0", "items": []object{{"opOrderItemId": "i", "quantity": "1"}}}
	event := OrderEventFile{Version: 1, AccountKey: account.StorageKey, MessageID: "m1", OrderNo: num, Kind: "new_order", CapturedAt: "2026-11-20T02:00:00.000Z", Packages: []object{p}}
	apply := func(expectErr bool) {
		t.Helper()
		b, e := json.Marshal(event)
		if e != nil {
			t.Fatal(e)
		}
		e = ImportOrderEvent(context.Background(), db, account, b)
		if (e != nil) != expectErr {
			t.Fatalf("unexpected import result %v", e)
		}
	}
	check := func(tab, purchase string, want int) {
		t.Helper()
		got, e := Orders(db, ac, num, 1, 20, tab, purchase)
		if e != nil || got["total"] != want {
			t.Fatalf("%s/%s expected %d, got %v %v", tab, purchase, want, got, e)
		}
	}
	for _, s := range []string{"waitPurchase", "hasPurchase", "closed"} {
		setScope(s, "2026-11-20T01:00:00.000Z", "foreign")
	}
	apply(false)
	apply(false)
	check("waitProcess", "waitPurchase", 1)
	var childNumber string
	if err := db.QueryRow("SELECT order_number FROM orders WHERE account_id=? AND external_order_id='mergedOrder'", ac).Scan(&childNumber); err != nil || childNumber != "childNumber" {
		t.Fatalf("merged order index: %q: %v", childNumber, err)
	}
	event.Kind = "in_cancel"
	event.CapturedAt = "2026-11-20T03:00:00.000Z"
	apply(false)
	check("waitProcess", "waitPurchase", 1)
	event.Kind = "cancelled"
	event.CapturedAt = "2026-11-20T04:00:00.000Z"
	p["appPackageStatus"] = "cancelled"
	apply(false)
	check("waitProcess", "waitPurchase", 0)
	check("closed", "", 1)
	event.Kind = "new_order"
	event.MessageID = "out-of-order"
	event.CapturedAt = "2026-11-20T02:30:00.000Z"
	p["appPackageStatus"] = "wait_seller_send"
	apply(false)
	check("closed", "", 1)
	event.MessageID = "bogus"
	p["platformOrderSn"] = num + "0"
	apply(true)
	p["platformOrderSn"] = num
	ch, cursor, e := OrderEvents(db, ac, nil)
	if e != nil || len(ch) != 0 || cursor == 0 {
		t.Fatal(ch, cursor, e)
	}
	var zero int64
	ch, _, e = OrderEvents(db, ac, &zero)
	if e != nil || len(ch) != 4 || ch[2].Kind != "cancelled" {
		t.Fatal(ch, e)
	}
	ch, _, e = OrderEvents(db, other, &zero)
	if e != nil || len(ch) != 0 {
		t.Fatal(ch, e)
	}
	scopes, e := loadOrderScopes(db, ac)
	if e != nil || !scopes["closed"]["foreign"] {
		t.Fatal(scopes, e)
	}
	setScope("closed", "2026-11-20T05:00:00.000Z", "foreign")
	check("closed", "", 0)
}

func TestEventOrderMatchesMergeWithoutSubstring(t *testing.T) {
	p := object{"subPackageList": []object{{"platformOrderSn": "000100"}, {"platformOrderSn": "000101"}}}
	if !eventOrderMatches(p, "000101") || eventOrderMatches(p, "101") {
		t.Fatal("merged exact order match failed")
	}
}
