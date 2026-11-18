package server

import (
	"net/http"
	"net/http/httptest"
	"order_processor/internal/store"
	"path/filepath"
	"strings"
	"testing"
)

func TestCredentialManagement(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id, _ := store.Create(db, "test")
	mux := http.NewServeMux()
	apiCredentialRoutes(mux, db)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(method, "/api/accounts/"+id+"/api-credentials"+path, strings.NewReader(body)))
		return w
	}
	w := request("POST", "", `{"name":"主账号","apiId":"ak-example","key":"secret-only-on-server","current":true}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request("GET", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"current":true`) || strings.Contains(w.Body.String(), "secret-only") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request("GET", "/1", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"key":"secret-only-on-server"`) {
		t.Fatal("credential detail must include saved key")
	}
	if w = request("GET", "/999", ""); w.Code != 404 {
		t.Fatal("missing credential detail accepted")
	}
	w = request("PUT", "/1", `{"name":"主账号修改","apiId":"ak-example"}`)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	c, err := store.CurrentAPICredential(db, id)
	if err != nil || c.Key != "secret-only-on-server" {
		t.Fatal("blank edit replaced secret")
	}
	if w = request("POST", "", `{"name":"new","apiId":"id"}`); w.Code != 400 {
		t.Fatal("missing key accepted")
	}
	if w = request("POST", "/999/activate", ""); w.Code != 400 {
		t.Fatal("missing credential accepted")
	}
	if w = request("DELETE", "/1", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = request("GET", "", ""); w.Body.String() != "[]\n" {
		t.Fatal(w.Body.String())
	}
}
