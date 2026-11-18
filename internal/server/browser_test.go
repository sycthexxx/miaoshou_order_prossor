package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"order_processor/internal/adapters/miaoshou/web/browser"
	"order_processor/internal/store"
	"path/filepath"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type fixedRuntime struct{ target browser.Target }

func (f fixedRuntime) Remote() bool { return true }
func (f fixedRuntime) Resolve(context.Context, string, bool) (browser.Target, error) {
	return f.target, nil
}
func TestLANRequestHeaders(t *testing.T) {
	h := requestBoundary(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), "http://192.168.1.10:17381")
	for _, tc := range []struct {
		host, origin, site, path string
		want                     int
	}{
		{"192.168.1.10:17381", "http://192.168.1.10:17381", "same-origin", "/api/accounts", 204},
		{"192.168.1.10:17381", "https://evil.example", "cross-site", "/desktop/1/websockify", 403},
		{"192.168.1.10:17381", "http://localhost:5173", "same-site", "/api/accounts", 403},
		{"evil.example", "http://192.168.1.10:17381", "same-origin", "/api/accounts", 403},
		{"192.168.1.10:17381", "", "cross-site", "/api/accounts", 403},
	} {
		r := httptest.NewRequest("GET", "http://"+tc.host+tc.path, nil)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Sec-Fetch-Site", tc.site)
		r.Header.Set("X-Forwarded-Host", "192.168.1.10:17381")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%+v: %d", tc, w.Code)
		}
	}
}
func TestWorkerCredentialsAndDesktopExclusion(t *testing.T) {
	db, e := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	id, e := store.Create(db, "test")
	if e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse("http://127.0.0.1:18080")
	calls := 0
	g := &browserGateway{db: db, runtime: fixedRuntime{browser.Target{API: u, Desktop: u, Token: "worker-secret"}}, client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "Bearer worker-secret" || r.Header.Get("Cookie") != "" || r.Header.Get("X-Forwarded-Host") != "" {
			t.Fatal("forwarded untrusted headers")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"state":"verified"}`)), Header: make(http.Header)}, nil
	})}}
	call := func(action string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://localhost/api/accounts/"+id+"/"+action, nil)
		r.SetPathValue("id", id)
		r.Header.Set("Cookie", "employee-secret")
		r.Header.Set("Authorization", "Bearer employee-secret")
		r.Header.Set("X-Forwarded-Host", "evil.example")
		w := httptest.NewRecorder()
		g.action(action)(w, r)
		return w
	}
	g.gate(id).desktop = true
	if w := call("sync"); w.Code != 409 || calls != 0 {
		t.Fatal("sync not excluded", w.Code, calls)
	}
	g.gate(id).desktop = false
	w := call("sync")
	if w.Code != 200 || calls != 1 || strings.Contains(w.Body.String(), "secret") {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestDefaultLANAddresses(t *testing.T) {
	h := requestBoundary(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), "")
	for _, tc := range []struct {
		host, origin string
		want         int
	}{
		{"192.168.1.104:17381", "http://192.168.1.104:17381", 204},
		{"192.168.20.5:17381", "http://192.168.20.5:17381", 204},
		{"127.0.0.1:17381", "http://127.0.0.1:5173", 204},
		{"[::1]:17381", "http://[::1]:17381", 204},
		{"192.168.1.104:17381", "http://192.168.1.105:17381", 403},
		{"192.168.1.104:17381", "http://localhost:5173", 403},
		{"192.168.1.104.evil.example:17381", "", 403},
		{"10.0.0.5:17381", "", 403},
	} {
		r := httptest.NewRequest("POST", "http://"+tc.host+"/api/accounts", nil)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%+v: %d", tc, w.Code)
		}
	}
}

type failedRuntime struct{ err error }

func (f failedRuntime) Remote() bool { return true }
func (f failedRuntime) Resolve(context.Context, string, bool) (browser.Target, error) {
	return browser.Target{}, f.err
}

func TestRuntimeFailureShowsActionableReason(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id, _ := store.Create(db, "test")
	root := t.TempDir()
	old := &browser.Docker{DataDir: root, Prefix: "op-browser-", Run: func(_ context.Context, args ...string) ([]byte, error) {
		if args[0] != "inspect" {
			t.Fatalf("unexpected mutation %v", args)
		}
		return []byte(`[{"Config":{"Labels":{"order-processor.account":"` + id + `","order-processor.data":"` + root + `","order-processor.profile-layout":"container-v1"}}}]`), nil
	}}
	unavailable := &browser.Docker{Run: func(context.Context, ...string) ([]byte, error) { return nil, errors.New("secret raw daemon failure") }}
	for _, tc := range []struct {
		name, action, code, fragment string
		runtime                      browser.Runtime
		status                       int
	}{
		{"old-layout", "status", "container_rebuild_required", "保留全部数据", old, 503},
		{"docker", "login", "docker_unavailable", "Docker 不可用", unavailable, 503},
		{"closed", "status", "", "closed", failedRuntime{browser.ErrNotStarted}, 200},
		{"start", "sync", "browser_not_started", "打开登录窗口", failedRuntime{browser.ErrNotStarted}, 503},
		{"raw", "status", "runtime_unavailable", "运行环境检查失败", failedRuntime{errors.New("secret Bearer token")}, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &browserGateway{db: db, runtime: tc.runtime}
			r := httptest.NewRequest("GET", "http://localhost/", nil)
			r.SetPathValue("id", id)
			w := httptest.NewRecorder()
			g.action(tc.action)(w, r)
			body := w.Body.String()
			if w.Code != tc.status || !strings.Contains(body, tc.fragment) || !strings.Contains(body, `"browserMode":"docker"`) || strings.Contains(body, "secret") || (tc.code != "" && !strings.Contains(body, tc.code)) {
				t.Fatal(w.Code, body)
			}
		})
	}
}
