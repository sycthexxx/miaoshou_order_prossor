package server

import (
	"encoding/json"
	"net/http/httptest"
	"order_processor/internal/store"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcurementMappingRoutesStrictSchemaAndConflict(t *testing.T) {
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
	if _, e = db.Exec(`INSERT INTO shop_products(account_id,platform,shop_id,platform_item_id,payload,captured_at,source_file) VALUES(?,'tiktok','shop','p','{"sourceList":[{"source":"1688","sourceItemId":"100"}]}','2026','fixture')`, account); e != nil {
		t.Fatal(e)
	}
	h := New(db, dir, 1, "unused")
	request := func(method, id, offer, origin, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://192.168.1.104:17381/api/accounts/"+id+"/procurement-mappings/"+offer, strings.NewReader(body))
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	origin := "http://192.168.1.104:17381"
	w := request("GET", account, "100", origin, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"revision":0`) {
		t.Fatal(w.Code, w.Body.String())
	}
	body := `{"revision":0,"mapping":{"version":2,"colors":{"海军蓝":"藏青色"},"skus":{"海军蓝,XL":{"tail":["1XL"],"exception":"藏青色,XL","legacyAlias":"蓝色,XL"}}}}`
	w = request("PUT", account, "100", origin, body)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request("PUT", account, "100", origin, body)
	var conflict map[string]any
	if e = json.Unmarshal(w.Body.Bytes(), &conflict); e != nil {
		t.Fatal(e)
	}
	if w.Code != 409 || conflict["revision"] != float64(1) || conflict["mapping"] == nil {
		t.Fatal(w.Code, conflict)
	}
	for _, bad := range []string{
		`{"revision":1,"mapping":{"version":2,"colors":{},"skus":{"海军蓝,XL":{"selector":"#quantity"}}}}`,
		`{"revision":1,"mapping":{"version":2,"selector":"#quantity","colors":{},"skus":{}}}`,
		`{"revision":1,"mapping":{"version":2,"colors":{"蓝":4},"skus":{}}}`,
		`{"revision":1,"mapping":{"version":2,"colors":{},"skus":{"蓝,XL":{"tail":[1]}}}}`,
		`{"mapping":{"version":2,"colors":{},"skus":{}}}`,
		`{"revision":1,"mapping":{"version":1}}`,
		body + `{}`,
	} {
		w = request("PUT", account, "100", origin, bad)
		if w.Code != 400 {
			t.Fatal(w.Code, bad, w.Body.String())
		}
	}
	if w = request("PUT", account, "999", origin, body); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w = request("GET", other, "100", origin, ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w = request("GET", "99999", "100", origin, ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w = request("PUT", account, "100", "https://detail.1688.com", body); w.Code != 403 {
		t.Fatal(w.Code)
	}
	w = request("GET", account, "100", origin, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"revision":1`) {
		t.Fatal("rejected requests changed mapping", w.Code, w.Body.String())
	}
}
