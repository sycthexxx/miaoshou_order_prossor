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

func TestPluginQuerySettingsSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"A", "B"} {
		if _, err := store.Create(db, name); err != nil {
			t.Fatal(err)
		}
	}
	h := New(db, dir, 1, "test")
	request := func(method, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, "http://127.0.0.1/api/plugin-query-settings", strings.NewReader(body)))
		return w
	}
	if w := request("GET", ""); !strings.Contains(w.Body.String(), `"account":"1"`) {
		t.Fatal(w.Body.String())
	}
	const settings = `{"account":"2","gifts":true}`
	if w := request("PUT", settings); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := request("PUT", `{"account":"999"}`); w.Code != 400 {
		t.Fatal(w.Code)
	}
	db.Close()
	db, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h = New(db, dir, 1, "test")
	if w := request("GET", ""); strings.TrimSpace(w.Body.String()) != settings {
		t.Fatal(w.Body.String())
	}
}
