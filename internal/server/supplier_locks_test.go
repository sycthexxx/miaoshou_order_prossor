package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"order_processor/internal/store"
	"path/filepath"
	"strconv"
	"testing"
)

func TestSupplierLockBelongsToUserAcrossSessions(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, err := store.Create(db, "test")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerSupplierLocks(mux, db)
	h := (&authentication{db: db, sessions: map[string]loginSession{}}).protect(mux)
	owner := adminSession(t, h)
	_, err = db.Exec("INSERT INTO app_users(username,password,is_admin) VALUES('observer','pw',1)")
	if err != nil {
		t.Fatal(err)
	}
	login := authRequest(h, nil, "POST", "/api/login", `{"username":"observer","password":"pw"}`)
	if login.Code != 200 {
		t.Fatal(login.Code, login.Body.String())
	}
	observer := login.Result().Cookies()[0]
	path := "/api/accounts/" + account + "/supplier-locks"
	lockPath := path + "/0"
	if w := authRequest(h, observer, "POST", "/api/accounts/"+account+".0/supplier-locks/0", ""); w.Code != 404 {
		t.Fatalf("noncanonical account path: %d", w.Code)
	}
	w := authRequest(h, owner, "POST", lockPath, "")
	var acquired store.SupplierLease
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &acquired) != nil || acquired.ID <= 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = authRequest(h, owner, "POST", "/api/logout", ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	owner = adminSession(t, h)
	list := func(cookie *http.Cookie) []store.SupplierLease {
		t.Helper()
		res := authRequest(h, cookie, "GET", path, "")
		var rows []store.SupplierLease
		if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &rows) != nil {
			t.Fatal(res.Code, res.Body.String())
		}
		return rows
	}
	if rows := list(owner); len(rows) != 1 || rows[0].ID != acquired.ID {
		t.Fatalf("owner could not resume after login: %+v", rows)
	}
	if rows := list(observer); len(rows) != 1 || rows[0].ID != 0 || rows[0].Username != "admin" {
		t.Fatalf("observer should see the active owner: %+v", rows)
	}
	if w = authRequest(h, observer, "POST", lockPath, ""); w.Code != 409 {
		t.Fatalf("conflicting acquisition: %d", w.Code)
	}
	release := func(cookie *http.Cookie) int {
		req := httptest.NewRequest("DELETE", lockPath, nil)
		req.AddCookie(cookie)
		req.Header.Set("X-Supplier-Lease", strconv.FormatInt(acquired.ID, 10))
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		return res.Code
	}
	if code := release(observer); code != 409 {
		t.Fatalf("foreign release: %d", code)
	}
	if code := release(owner); code != 200 || len(list(observer)) != 0 {
		t.Fatalf("owner release: %d", code)
	}
	if w = authRequest(h, observer, "POST", lockPath, ""); w.Code != 200 {
		t.Fatalf("acquisition after handoff: %d %s", w.Code, w.Body.String())
	}
}
