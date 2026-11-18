package server

import (
	"encoding/json"
	"net/http/httptest"
	"order_processor/internal/store"
	"path/filepath"
	"strings"
	"testing"
)

func TestSupplierCaptureLANRoute(t *testing.T) {
	dir := t.TempDir()
	db, e := store.Open(filepath.Join(dir, "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	account, e := store.Create(db, "account")
	if e != nil {
		t.Fatal(e)
	}
	other, e := store.Create(db, "other")
	if e != nil {
		t.Fatal(e)
	}
	payload := `{"sourceList":[{"source":"1688","sourceItemId":"100"}]}`
	if _, e = db.Exec(`INSERT INTO shop_products(account_id,platform,shop_id,platform_item_id,payload,captured_at,source_file) VALUES(?,'tiktok','shop','product',?,'2026','fixture')`, account, payload); e != nil {
		t.Fatal(e)
	}
	h := New(db, dir, 1, "unused")
	request := func(id, origin, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://192.168.1.104:17381/api/accounts/"+id+"/source-supplier/capture", strings.NewReader(body))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	body := `{"productId":"100","name":"页面厂家"}`
	if w := request(account, "http://192.168.1.104:17381", body); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w := request(account, "http://192.168.1.104:17381", `{"productId":"100","name":"新的页面厂家"}`)
	var result map[string]any
	if e = json.Unmarshal(w.Body.Bytes(), &result); e != nil {
		t.Fatal(e)
	}
	if w.Code != 200 || result["assigned"] != float64(0) || result["retained"] != float64(0) || result["updated"] != float64(1) || result["supplierName"] != "新的页面厂家" {
		t.Fatal(w.Code, result)
	}
	if w := request(other, "http://192.168.1.104:17381", body); w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("999999", "http://192.168.1.104:17381", body); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := request(account, "https://detail.1688.com", body); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := request(account, "http://192.168.1.104:17381", "bad json"); w.Code != 400 {
		t.Fatal(w.Code)
	}
	var count int
	db.QueryRow("SELECT count(*) FROM suppliers").Scan(&count)
	if count != 2 {
		t.Fatal("unexpected writes from rejected requests", count)
	}
}
