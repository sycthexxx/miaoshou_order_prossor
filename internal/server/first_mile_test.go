package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"order_processor/internal/adapters/miaoshou/web/browser"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"order_processor/internal/store"
)

func TestFirstMileSettingsAndHistory(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id, err := store.Create(db, "purchase logistics")
	if err != nil {
		t.Fatal(err)
	}
	accountDir := filepath.Join(dir, "runtime", store.StorageKey(db, id))
	h := authenticatedTestServer(t, db, dir)
	path := "/api/accounts/" + id + "/first-mile"
	for _, check := range []struct {
		method, suffix, body, want string
		status                     int
	}{
		{"GET", "/settings", "", `"intervalMinutes":60`, 200},
		{"PUT", "/settings", `{"enabled":true,"intervalMinutes":0}`, "", 400},
		{"PUT", "/settings", `{"enabled":false,"intervalMinutes":15}`, `"enabled":false`, 200},
		{"GET", "/settings", "", `"intervalMinutes":15`, 200},
		{"GET", "/runs", "", `"runs":[]`, 200},
	} {
		w := authRequest(h, nil, check.method, path+check.suffix, check.body)
		if w.Code != check.status || !strings.Contains(w.Body.String(), check.want) {
			t.Fatalf("%s %s: %d %s", check.method, check.suffix, w.Code, w.Body.String())
		}
	}
	runs := filepath.Join(accountDir, "first-mile-runs")
	if err := os.MkdirAll(runs, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runs, "1763811000000.json"), []byte(`{"id":"1763811000000","state":"completed","items":[{"number":"5127948757295409109"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	list := authRequest(h, nil, http.MethodGet, path+"/runs", "")
	detail := authRequest(h, nil, http.MethodGet, path+"/runs/1763811000000", "")
	if list.Code != 200 || !strings.Contains(list.Body.String(), "1763811000000") ||
		strings.Contains(list.Body.String(), "items") || detail.Code != 200 ||
		!strings.Contains(detail.Body.String(), "5127948757295409109") {
		t.Fatalf("list: %d %s, detail: %d %s", list.Code, list.Body.String(), detail.Code, detail.Body.String())
	}
}

func TestFirstMileStopTargetsOnlyRunningRound(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id, err := store.Create(db, "stop test")
	if err != nil {
		t.Fatal(err)
	}
	runs := filepath.Join(dir, "runtime", store.StorageKey(db, id), "first-mile-runs")
	if err := os.MkdirAll(runs, 0700); err != nil {
		t.Fatal(err)
	}
	for runID, state := range map[string]string{"1763811000000": "running", "1763810000000": "completed"} {
		if err := os.WriteFile(filepath.Join(runs, runID+".json"), []byte(`{"state":"`+state+`","canStop":true}`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	h := authenticatedTestServer(t, db, dir)
	base := "/api/accounts/" + id + "/first-mile/runs"
	for runID, status := range map[string]int{"1763811000000": 202, "1763810000000": 409, "1763820000000": 404} {
		w := authRequest(h, nil, http.MethodPost, base+"/"+runID+"/stop", "")
		if w.Code != status {
			t.Fatalf("%s: %d %s", runID, w.Code, w.Body.String())
		}
	}
	if _, err := os.Stat(filepath.Join(runs, "1763811000000.stop")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(runs, "1763810000000.stop")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	list := authRequest(h, nil, http.MethodGet, base, "")
	if !strings.Contains(list.Body.String(), `"state":"stopping"`) {
		t.Fatal(list.Body.String())
	}
}

func TestFirstMileManualStartForwardsAccountAndBusyStatus(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id, err := store.Create(db, "manual")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/accounts/"+id+"/first-mile-start" ||
			r.Header.Get("Authorization") != "Bearer worker-test" {
			t.Error("unexpected worker request")
			w.WriteHeader(400)
			return
		}
		calls++
		if calls == 1 {
			send(w, 202, map[string]string{"id": "1763811000000", "state": "running"})
		} else {
			send(w, 409, map[string]string{"error": "已有一轮头程物流同步正在运行"})
		}
	}))
	defer worker.Close()
	target, _ := url.Parse(worker.URL)
	h := NewWithRuntimeContext(nil, db, dir, fixedRuntime{browser.Target{API: target, Token: "worker-test"}}, "")
	cookie := adminSession(t, h)
	route := "/api/accounts/" + id + "/first-mile/runs"
	for _, status := range []int{http.StatusAccepted, http.StatusConflict} {
		w := authRequest(h, cookie, http.MethodPost, route, "")
		if w.Code != status {
			t.Fatalf("manual start: %d %s", w.Code, w.Body.String())
		}
	}
}
