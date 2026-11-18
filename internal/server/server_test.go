package server

import (
	"net/http/httptest"
	"order_processor/internal/store"
	"path/filepath"
	"strings"
	"testing"
)

func TestAccountsPersistenceAndOrigin(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.db")
	db, e := store.Open(p)
	if e != nil {
		t.Fatal(e)
	}
	h := New(db, dir, 1, "test")
	request := func(origin, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://127.0.0.1:17381/api/accounts", strings.NewReader(body))
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := request("https://evil.example", `{"name":"bad"}`); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := request("http://127.0.0.1:17381", `{"name":"公司主账户"}`); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("http://127.0.0.1:17381", `{"name":"公司主账户"}`); w.Code != 409 {
		t.Fatal(w.Code)
	}
	db.Close()
	db, e = store.Open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	a, e := store.Accounts(db)
	if e != nil || len(a) != 1 || a[0].Name != "公司主账户" {
		t.Fatalf("persistence: %v %v", a, e)
	}
	if store.Exists(db, "../../other") {
		t.Fatal("unexpected account")
	}
}
