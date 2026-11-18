package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"order_processor/internal/store"
	"path/filepath"
	"strings"
	"testing"
)

func authRequest(h http.Handler, c *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1"+path, strings.NewReader(body))
	if c != nil {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func adminSession(t *testing.T, h http.Handler) *http.Cookie {
	t.Helper()
	w := authRequest(h, nil, "POST", "/api/login", `{"username":"admin","password":"123456"}`)
	if w.Code != 200 || len(w.Result().Cookies()) != 1 {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	return w.Result().Cookies()[0]
}

// Existing route tests exercise the same authenticated middleware as the UI.
func authenticatedTestServer(t *testing.T, db *sql.DB, dir string) http.Handler {
	t.Helper()
	h := New(db, dir, 1, "test")
	c := adminSession(t, h)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { r.AddCookie(c); h.ServeHTTP(w, r) })
}

func TestAccountAccessFollowsCurrentGrants(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	first, err := store.Create(db, "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Create(db, "second")
	if err != nil {
		t.Fatal(err)
	}
	h := New(db, dir, 1, "test")
	for _, path := range []string{"/api/accounts", "/api/users", "/desktop/1/vnc.html", "/api/accounts/1/orders"} {
		if w := authRequest(h, nil, "GET", path, ""); w.Code != 401 {
			t.Fatal(path, w.Code)
		}
	}
	if w := authRequest(h, nil, "POST", "/api/login", `{"username":"admin","password":"wrong"}`); w.Code != 401 {
		t.Fatal(w.Code)
	}
	root := adminSession(t, h)
	w := authRequest(h, root, "POST", "/api/users", `{"username":"staff","password":"p123","accounts":["`+second+`"]}`)
	if w.Code < 200 || w.Code >= 300 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = authRequest(h, nil, "POST", "/api/login", `{"username":"staff","password":"p123"}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	c := w.Result().Cookies()[0]
	w = authRequest(h, c, "GET", "/api/accounts", "")
	var accounts []store.Account
	if err = json.Unmarshal(w.Body.Bytes(), &accounts); err != nil || len(accounts) != 1 || accounts[0].ID != second {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, path := range []string{"/api/users", "/api/accounts/" + first + "/orders", "/api/accounts/" + second + "/api-credentials", "/desktop/" + second + "/vnc.html", "/api/accounts/" + second + "/archives"} {
		if w := authRequest(h, c, "GET", path, ""); w.Code != 403 {
			t.Fatal(path, w.Code)
		}
	}
	if w := authRequest(h, c, "POST", "/api/accounts", `{"name":"unapproved"}`); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := authRequest(h, c, "GET", "/api/accounts/"+second+"/orders", ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err = db.Exec("DELETE FROM user_accounts"); err != nil {
		t.Fatal(err)
	}
	if w := authRequest(h, c, "GET", "/api/accounts/"+second+"/orders", ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	authRequest(h, c, "POST", "/api/logout", "")
	if w := authRequest(h, c, "GET", "/api/session", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
}
