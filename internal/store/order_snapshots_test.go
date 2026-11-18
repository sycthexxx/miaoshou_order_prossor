package store

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestScopeReplayAuthoritativeMembership(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, _ := Create(db, "a")
	other, _ := Create(db, "b")
	rawDir := filepath.Join(dir, "raw", account)
	os.MkdirAll(rawDir, 0700)
	write := func(file, at, request string, total int, packages ...object) {
		t.Helper()
		list := []object{}
		list = append(list, packages...)
		body, _ := json.Marshal(object{"result": "success", "total": total, "page": 1, "packageList": list})
		b, _ := json.Marshal(object{"accountId": account, "endpoint": listEndpoint, "status": 200, "capturedAt": at, "bodyBase64": base64.StdEncoding.EncodeToString(body), "requestBody": request})
		if err := os.WriteFile(filepath.Join(rawDir, file), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	pkg := func(id, state, purchase string) object {
		return object{"opOrderPackageId": id, "appPackageStatus": state, "appPackageOperateStatus": "wait_audit", "appPurchaseStatus": purchase, "items": []object{{"opOrderItemId": id, "quantity": "1"}}}
	}
	req := "source=orderProcess&appPackageTab=waitProcess&purchaseTab=waitPurchase&page=1&pageSize=200"
	write("01.json", "2026-01-01T00:00:00Z", "", 1, pkg("old", "wait_seller_send", "2"))
	write("02.json", "2026-01-02T00:00:00Z", req, 2, pkg("pending", "pending", "0"), pkg("p1", "wait_seller_send", "1"))
	write("03.json", "2026-01-03T00:00:00Z", "source=orderProcess&appPackageTab=waitProcess&purchaseTab=hasPurchase&page=1&pageSize=200", 1, pkg("sent", "wait_seller_send", "2"))
	if _, err := Replay(db, dir); err != nil {
		t.Fatal(err)
	}
	check := func(wait, has int) {
		t.Helper()
		d, err := Orders(db, account, "", 1, 100, "waitProcess")
		if err != nil {
			t.Fatal(err)
		}
		c := d["counts"].(map[string]int)
		if c["waitPurchase"] != wait || c["hasPurchase"] != has || c["all"] != 4 {
			t.Fatal(c)
		}
	}
	check(2, 1)
	p, err := Procurement(db, account, true)
	if err != nil {
		t.Fatal(err)
	}
	if p["packages"] != 2 {
		t.Fatal(p)
	}
	// Partial, filtered, and duplicate-ID pages must never replace a full scope.
	write("04.json", "2026-01-04T00:00:00Z", req, 2, pkg("pending", "pending", "0"))
	write("05.json", "2026-01-05T00:00:00Z", req+"&shopId=123", 0)
	write("06.json", "2026-01-06T00:00:00Z", req, 2, pkg("pending", "pending", "0"), pkg("pending", "pending", "0"))
	if _, err := Replay(db, dir); err != nil {
		t.Fatal(err)
	}
	check(2, 1)
	scopes, err := loadOrderScopes(db, other)
	if err != nil || len(scopes) != 0 {
		t.Fatal(scopes, err)
	}
	// Complete empty means clear this scope only, while retaining history.
	write("07.json", "2026-01-07T00:00:00Z", req, 0)
	if _, err := Replay(db, dir); err != nil {
		t.Fatal(err)
	}
	check(0, 1)
	// Replaying archives after parser upgrades cannot let old snapshots win.
	if _, err = db.Exec("UPDATE imported_archives SET parser_version=2"); err != nil {
		t.Fatal(err)
	}
	if _, err := Replay(db, dir); err != nil {
		t.Fatal(err)
	}
	check(0, 1)
}

func TestMultiPageScopeManifestValidation(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, _ := Create(db, "a")
	os.MkdirAll(filepath.Join(dir, "raw", a), 0700)
	os.MkdirAll(filepath.Join(dir, "snapshots", a), 0700)
	for page := 1; page <= 2; page++ {
		id := fmt.Sprint(page)
		body, _ := json.Marshal(object{"result": "success", "total": 2, "page": page, "packageList": []object{{"opOrderPackageId": id, "items": []object{}}}})
		raw, _ := json.Marshal(object{"accountId": a, "endpoint": listEndpoint, "status": 200, "capturedAt": "2026-01-01T00:00:00Z", "requestBody": fmt.Sprintf("source=orderProcess&appPackageTab=waitProcess&purchaseTab=waitPurchase&page=%d&pageSize=1", page), "bodyBase64": base64.StdEncoding.EncodeToString(body)})
		os.WriteFile(filepath.Join(dir, "raw", a, id+".json"), raw, 0600)
	}
	if _, err = Replay(db, dir); err != nil {
		t.Fatal(err)
	}
	scopes, _ := loadOrderScopes(db, a)
	if len(scopes) != 0 {
		t.Fatal("partial pages published", scopes)
	}
	manifest := object{"version": 1, "accountId": a, "scope": "waitPurchase", "capturedAt": "2026-01-02T00:00:00Z", "total": 2, "archives": []string{"1.json", "2.json"}, "packageIds": []string{"1", "2"}}
	save := func() {
		b, _ := json.Marshal(manifest)
		os.WriteFile(filepath.Join(dir, "snapshots", a, "pass.json"), b, 0600)
	}
	save()
	if _, err = Replay(db, dir); err != nil {
		t.Fatal(err)
	}
	scopes, _ = loadOrderScopes(db, a)
	if len(scopes["waitPurchase"]) != 2 {
		t.Fatal(scopes)
	}
	// A forged smaller/duplicate pass cannot clear valid prior membership.
	manifest["capturedAt"] = "2026-01-03T00:00:00Z"
	manifest["archives"] = []string{"1.json", "1.json"}
	save()
	if _, err = Replay(db, dir); err != nil {
		t.Fatal(err)
	}
	var at string
	db.QueryRow(`SELECT captured_at FROM order_scope_snapshots`).Scan(&at)
	if at != "2026-01-02T00:00:00Z" {
		t.Fatal(at)
	}
}

func TestNewerPurchaseScopeWinsOverlap(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, _ := Create(db, "a")
	tx, _ := db.Begin()
	if err = saveOrderScope(tx, a, "hasPurchase", "2026-01-01", []string{"old"}, []string{"x"}); err != nil {
		t.Fatal(err)
	}
	if err = saveOrderScope(tx, a, "waitPurchase", "2026-01-02", []string{"new"}, []string{"x"}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	s, err := loadOrderScopes(db, a)
	if err != nil {
		t.Fatal(err)
	}
	_, scope := s.Membership(object{"opOrderPackageId": "x", "appPackageStatus": "pending", "appPurchaseStatus": "1"})
	if scope != "waitPurchase" {
		t.Fatal(scope)
	}
}
