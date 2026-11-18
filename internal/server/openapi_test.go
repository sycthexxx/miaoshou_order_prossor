package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"order_processor/internal/application/ordersync"
	"order_processor/internal/store"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type apiTestTransport func(*http.Request) (*http.Response, error)

func (f apiTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestOpenAPIRoutesCommitArchiveAndInvalidate(t *testing.T) {
	original := http.DefaultTransport
	http.DefaultTransport = apiTestTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("x-app-key") != "fake-app" {
			t.Error("sync did not use selected saved credential")
		}
		data := `{"orderPackageList":[],"total":0,"page":1,"pageSize":200}`
		if strings.Contains(r.URL.Path, "purchase_record") {
			data = `{"list":[],"total":"0"}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"result":"success","data":` + data + `}`))}, nil
	})
	defer func() { http.DefaultTransport = original }()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, err := store.Create(db, "test")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	loads := 0
	cache := &procurementCache{load: func(string, bool) (map[string]any, error) { loads++; return map[string]any{}, nil }}
	cache.get(account, false, false)
	openAPIRoutes(mux, db, dir, cache)
	apiCredentialRoutes(mux, db)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(method, "/api/accounts/"+account+path, strings.NewReader(body)))
		return w
	}
	if w := call("POST", "/sync-openapi", `{}`); w.Code != 400 {
		t.Fatal(w.Code)
	}
	saved := call("POST", "/api-credentials", `{"name":"主账号","apiId":"fake-app","key":"fake-secret","current":true}`)
	if saved.Code != 200 {
		t.Fatal(saved.Code, saved.Body.String())
	}
	body := ""
	w := call("POST", "/sync-openapi", body)
	if w.Code != 202 || strings.Contains(w.Body.String(), "fake-secret") {
		t.Fatal(w.Code)
	}
	if w := call("POST", "/sync-openapi", body); w.Code != 409 {
		t.Fatal(w.Code)
	}
	var job ordersync.Job
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		w = call("GET", "/openapi-status", "")
		if err = json.Unmarshal(w.Body.Bytes(), &job); err != nil {
			t.Fatal(err)
		}
		if job.State != "running" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if job.State != "completed" || job.Result == nil {
		t.Fatal(job.State, job.Error)
	}
	cache.get(account, false, false)
	if loads != 2 {
		t.Fatal("procurement cache was not invalidated")
	}
	files, err := filepath.Glob(filepath.Join(dir, job.Archive, "*.json"))
	if err != nil || len(files) != 5 {
		t.Fatal("missing full responses", len(files), err)
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "fake-secret") || strings.Contains(string(raw), "fake-app") {
			t.Fatal("credential in archive")
		}
		info, _ := os.Stat(file)
		if info.Mode().Perm() != 0600 {
			t.Fatal("archive permissions")
		}
	}
	// Cancellation must not report success or alter a previous complete snapshot.
	w = call("POST", "/sync-openapi", body)
	if w.Code != 202 {
		t.Fatal(w.Code)
	}
	call("POST", "/stop-openapi", "")
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		w = call("GET", "/openapi-status", "")
		json.Unmarshal(w.Body.Bytes(), &job)
		if job.State != "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if job.State != "cancelled" {
		t.Fatal(job.State)
	}
}
