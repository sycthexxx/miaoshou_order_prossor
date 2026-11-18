package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func captureFixture(t *testing.T) (*sql.DB, string, string) {
	t.Helper()
	db, e := Open(filepath.Join(t.TempDir(), "capture.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	account, e := Create(db, "主账户")
	if e != nil {
		t.Fatal(e)
	}
	other, e := Create(db, "其他账户")
	if e != nil {
		t.Fatal(e)
	}
	return db, account, other
}
func captureProduct(t *testing.T, db *sql.DB, account, product string, sites ...string) {
	t.Helper()
	sources := []any{}
	for _, site := range sites {
		sources = append(sources, object{"source": "1688", "sourceSite": site, "sourceItemId": product})
	}
	tx, e := db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	e = saveProduct(tx, account, "2026-11-16T12:00:00Z", "fixture", object{"platform": "tiktok", "shopId": "shop", "platformItemId": product, "sourceList": sources})
	if e != nil {
		tx.Rollback()
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
}
func captureCount(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if e := db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}
func TestCaptureSupplierReuseAndIdempotence(t *testing.T) {
	db, account, _ := captureFixture(t)
	captureProduct(t, db, account, "100", "")
	captureProduct(t, db, account, "200", "")
	result, e := CaptureSupplier(db, account, "100", "  正格服饰厂  ")
	if e != nil {
		t.Fatal(e)
	}
	if result["created"] != true || result["assigned"] != int64(1) || result["retained"] != 0 || result["supplierName"] != "正格服饰厂" {
		t.Fatal(result)
	}
	id := result["supplierId"].(int64)
	if id <= 0 {
		t.Fatal("supplier must have a positive generated ID")
	}
	var before string
	if e = db.QueryRow("SELECT updated_at FROM source_suppliers WHERE product_id='100'").Scan(&before); e != nil {
		t.Fatal(e)
	}
	result, e = CaptureSupplier(db, account, "200", "正格服饰厂")
	if e != nil {
		t.Fatal(e)
	}
	if result["created"] != false || result["supplierId"] != id || result["assigned"] != int64(1) {
		t.Fatal(result)
	}
	result, e = CaptureSupplier(db, account, "100", " 正格服饰厂 ")
	if e != nil {
		t.Fatal(e)
	}
	if result["created"] != false || result["assigned"] != int64(0) || result["retained"] != 1 {
		t.Fatal(result)
	}
	var after string
	var afterID int64
	if e = db.QueryRow("SELECT supplier_id,updated_at FROM source_suppliers WHERE product_id='100'").Scan(&afterID, &after); e != nil {
		t.Fatal(e)
	}
	if after != before || afterID != id {
		t.Fatal("existing assignment changed", before, after, id, afterID)
	}
	if captureCount(t, db, "suppliers") != 1 || captureCount(t, db, "source_suppliers") != 2 {
		t.Fatal("unexpected supplier or assignment count")
	}
	result, e = CaptureSupplier(db, account, "100", "另一家新工厂")
	if e != nil {
		t.Fatal(e)
	}
	if result["created"] != true || result["assigned"] != int64(0) || result["updated"] != int64(1) || result["retained"] != 0 || result["supplierId"] == id {
		t.Fatal(result)
	}
	var unchanged int64
	if e = db.QueryRow("SELECT supplier_id FROM source_suppliers WHERE product_id='200'").Scan(&unchanged); e != nil {
		t.Fatal(e)
	}
	if unchanged != id || captureCount(t, db, "suppliers") != 2 {
		t.Fatal("reassignment changed or removed old supplier's other product")
	}

}
func TestCaptureSupplierMixedSitesUpdatesDifferentNames(t *testing.T) {
	db, account, _ := captureFixture(t)
	captureProduct(t, db, account, "100", "", "cn", "cn", "keep")
	captureProduct(t, db, account, "200", "")
	res, e := db.Exec("INSERT INTO suppliers(account_id,platform,name) VALUES(?,'1688','人工指定厂家')", account)
	if e != nil {
		t.Fatal(e)
	}
	manual, _ := res.LastInsertId()
	res, e = db.Exec("INSERT INTO suppliers(account_id,platform,name) VALUES(?,'1688','页面厂家')", account)
	if e != nil {
		t.Fatal(e)
	}
	existing, _ := res.LastInsertId()
	for _, assignment := range []struct {
		product, site string
		id            int64
	}{{"100", "", manual}, {"100", "keep", existing}, {"200", "", manual}} {
		if e = AssignSupplier(db, account, "1688", assignment.site, assignment.product, assignment.id); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = db.Exec("UPDATE source_suppliers SET updated_at='before-capture'"); e != nil {
		t.Fatal(e)
	}
	r, e := CaptureSupplier(db, account, "100", "页面厂家")
	if e != nil {
		t.Fatal(e)
	}
	if r["assigned"] != int64(1) || r["updated"] != int64(1) || r["retained"] != 1 || r["created"] != false || r["supplierId"] != existing {
		t.Fatal(r)
	}
	for _, site := range []string{"", "cn", "keep"} {
		var id int64
		var at string
		if e = db.QueryRow("SELECT supplier_id,updated_at FROM source_suppliers WHERE product_id='100' AND site=?", site).Scan(&id, &at); e != nil {
			t.Fatal(e)
		}
		if id != existing {
			t.Fatal("site not reassigned", site, id)
		}
		if (site == "keep") != (at == "before-capture") {
			t.Fatal("only same-name mapping should keep timestamp", site, at)
		}
	}
	var otherID int64
	var otherAt string
	if e = db.QueryRow("SELECT supplier_id,updated_at FROM source_suppliers WHERE product_id='200'").Scan(&otherID, &otherAt); e != nil {
		t.Fatal(e)
	}
	if otherID != manual || otherAt != "before-capture" {
		t.Fatal("old supplier's other product changed")
	}
	if captureCount(t, db, "source_suppliers") != 4 || captureCount(t, db, "suppliers") != 2 {
		t.Fatal("duplicate site or supplier")
	}
	r, e = CaptureSupplier(db, account, "100", "  页面厂家  ")
	if e != nil {
		t.Fatal(e)
	}
	if r["assigned"] != int64(0) || r["updated"] != int64(0) || r["retained"] != 3 || r["supplierName"] != "页面厂家" {
		t.Fatal(r)
	}
}
func TestCaptureSupplierLegacyWhitespaceNameReuse(t *testing.T) {
	db, account, _ := captureFixture(t)
	captureProduct(t, db, account, "100", "", "cn")
	res, e := db.Exec("INSERT INTO suppliers(account_id,platform,name) VALUES(?,'1688',?)", account, "\t同名厂\u3000")
	if e != nil {
		t.Fatal(e)
	}
	id, _ := res.LastInsertId()
	if e = AssignSupplier(db, account, "1688", "", "100", id); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec("UPDATE source_suppliers SET updated_at='before'"); e != nil {
		t.Fatal(e)
	}
	r, e := CaptureSupplier(db, account, "100", "同名厂")
	if e != nil {
		t.Fatal(e)
	}
	if r["supplierId"] != id || r["created"] != false || r["retained"] != 1 || r["assigned"] != int64(1) || r["updated"] != int64(0) {
		t.Fatal(r)
	}
	var at, rawName string
	db.QueryRow("SELECT updated_at FROM source_suppliers WHERE site=''").Scan(&at)
	db.QueryRow("SELECT name FROM suppliers WHERE id=?", id).Scan(&rawName)
	if at != "before" || rawName != "\t同名厂\u3000" || captureCount(t, db, "suppliers") != 1 {
		t.Fatal("same trimmed name was mutated")
	}
}
func TestCaptureSupplierAccountIsolationAndValidation(t *testing.T) {
	db, account, other := captureFixture(t)
	captureProduct(t, db, account, "100", "")
	captureProduct(t, db, other, "200", "")
	for _, tc := range []struct{ account, product, name string }{
		{other, "100", "厂家"}, {account, "200", "厂家"}, {account, "999", "厂家"},
		{account, "https://detail.1688.com/offer/100.html", "厂家"}, {account, "100", "   "},
	} {
		if _, e := CaptureSupplier(db, tc.account, tc.product, tc.name); e == nil {
			t.Fatalf("unexpected acceptance %+v", tc)
		}
	}
	if captureCount(t, db, "suppliers") != 0 {
		t.Fatal("invalid capture created supplier")
	}
	a, e := CaptureSupplier(db, account, "100", "同名厂家")
	if e != nil {
		t.Fatal(e)
	}
	b, e := CaptureSupplier(db, other, "200", "同名厂家")
	if e != nil {
		t.Fatal(e)
	}
	if a["supplierId"] == b["supplierId"] {
		t.Fatal("suppliers leaked across accounts")
	}
}
func TestCaptureSupplierConcurrentReportsAndRollback(t *testing.T) {
	db, account, _ := captureFixture(t)
	captureProduct(t, db, account, "100", "")
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := CaptureSupplier(db, account, "100", "并发厂家")
			if e != nil {
				errors <- e
			}
		}()
	}
	wg.Wait()
	close(errors)
	for e := range errors {
		t.Fatal(e)
	}
	if captureCount(t, db, "suppliers") != 1 || captureCount(t, db, "source_suppliers") != 1 {
		t.Fatal("concurrent reports duplicated records")
	}
	captureProduct(t, db, account, "200", "")
	_, e := db.Exec(`CREATE TRIGGER fail_capture BEFORE INSERT ON source_suppliers WHEN NEW.product_id='200' BEGIN SELECT RAISE(ABORT,'fixture failure'); END;`)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = CaptureSupplier(db, account, "200", "应当回滚的厂家"); e == nil {
		t.Fatal("expected assignment failure")
	}
	if n := captureCount(t, db, "suppliers"); n != 1 {
		t.Fatal(fmt.Sprintf("failed capture left %d suppliers", n))
	}
}

func TestCaptureSupplierUpdateFailureRollsBackAllSitesAndNewSupplier(t *testing.T) {
	db, account, _ := captureFixture(t)
	captureProduct(t, db, account, "100", "")
	original, e := CaptureSupplier(db, account, "100", "旧厂家")
	if e != nil {
		t.Fatal(e)
	}
	captureProduct(t, db, account, "100", "", "cn")
	if _, e = db.Exec("UPDATE source_suppliers SET updated_at='before'"); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`CREATE TRIGGER fail_capture_update BEFORE UPDATE ON source_suppliers WHEN NEW.product_id='100' BEGIN SELECT RAISE(ABORT,'fixture update failure'); END;`); e != nil {
		t.Fatal(e)
	}
	if _, e = CaptureSupplier(db, account, "100", "应当回滚的新厂家"); e == nil {
		t.Fatal("expected reassignment failure")
	}
	if captureCount(t, db, "suppliers") != 1 || captureCount(t, db, "source_suppliers") != 1 {
		t.Fatal("partially committed transaction")
	}
	var id int64
	var at string
	if e = db.QueryRow("SELECT supplier_id,updated_at FROM source_suppliers").Scan(&id, &at); e != nil {
		t.Fatal(e)
	}
	if id != original["supplierId"] || at != "before" {
		t.Fatal("original assignment changed after rollback")
	}
}
