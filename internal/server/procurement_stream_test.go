package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"order_processor/internal/store"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func takeSnapshot(t *testing.T, ch <-chan map[string]any) map[string]any {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(time.Second):
		t.Fatal("missing snapshot")
		return nil
	}
}
func TestCaptureRebuildsWholeSnapshotsOnlyOnChange(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, _ := store.Create(db, "a")
	other, _ := store.Create(db, "b")
	_, err = db.Exec(`INSERT INTO shop_products(account_id,platform,shop_id,platform_item_id,payload,captured_at,source_file) VALUES(?,'tiktok','s','p','{"sourceList":[{"source":"1688","sourceItemId":"100"}]}','2026','fixture')`, account)
	if err != nil {
		t.Fatal(err)
	}
	loads := 0
	fail := false
	cache := &procurementCache{load: func(a string, g bool) (map[string]any, error) {
		loads++
		if fail {
			return nil, errors.New("private SQL failure")
		}
		var name string
		db.QueryRow(`SELECT s.name FROM source_suppliers m JOIN suppliers s ON s.id=m.supplier_id WHERE m.account_id=?`, a).Scan(&name)
		return map[string]any{"groups": []map[string]any{{"supplierName": name}}, "quantity": int64(7), "packages": 2, "gifts": g}, nil
	}}
	plain, stop, _ := cache.subscribe(account, false)
	defer stop()
	gift, stopGift, _ := cache.subscribe(account, true)
	defer stopGift()
	foreign, stopForeign, _ := cache.subscribe(other, false)
	defer stopForeign()
	old := takeSnapshot(t, plain)
	takeSnapshot(t, gift)
	takeSnapshot(t, foreign)
	mux := http.NewServeMux()
	supplierRoutes(mux, db, cache)
	capture := func(name string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/accounts/"+account+"/source-supplier/capture", strings.NewReader(`{"productId":"100","name":"`+name+`"}`))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	w := capture("厂家A")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	for _, ch := range []<-chan map[string]any{plain, gift} {
		d := takeSnapshot(t, ch)
		if d["quantity"] != int64(7) || d["packages"] != 2 || d["groups"].([]map[string]any)[0]["supplierName"] != "厂家A" || d["revision"].(int64) <= old["revision"].(int64) {
			t.Fatal("not a full rebuilt snapshot", d)
		}
	}
	select {
	case <-foreign:
		t.Fatal("cross-account event")
	default:
	}
	before := loads
	w = capture(" 厂家A ")
	if w.Code != 200 || loads != before {
		t.Fatal("retained-only rebuilt", loads, before)
	}
	select {
	case <-plain:
		t.Fatal("unchanged event")
	default:
	}
	w = capture("厂家B")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	takeSnapshot(t, plain)
	takeSnapshot(t, gift)
	fail = true
	w = capture("厂家C")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"cacheRefreshed":false`) || strings.Contains(w.Body.String(), "private SQL") {
		t.Fatal(w.Code, w.Body.String())
	}
	var name string
	db.QueryRow(`SELECT name FROM suppliers s JOIN source_suppliers m ON s.id=m.supplier_id WHERE m.account_id=?`, account).Scan(&name)
	if name != "厂家C" {
		t.Fatal("association was not committed", name)
	}
}
func TestSnapshotQueuesCoalesceAndRevisionsIncrease(t *testing.T) {
	cache := &procurementCache{load: func(a string, g bool) (map[string]any, error) {
		return map[string]any{"groups": []any{}, "gifts": g}, nil
	}}
	ch, stop, err := cache.subscribe("1", false)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	first := takeSnapshot(t, ch)
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := cache.get("1", false, true); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	last := takeSnapshot(t, ch)
	current, _ := cache.get("1", false, false)
	if last["revision"] != current["revision"] || last["revision"].(int64) <= first["revision"].(int64) {
		t.Fatal(last, current)
	}
	select {
	case <-ch:
		t.Fatal("stale events queued")
	default:
	}
}
func TestProcurementWebSocketAuthenticationAndCompleteSnapshot(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, _ := store.Create(db, "a")
	cache := &procurementCache{load: func(string, bool) (map[string]any, error) {
		return map[string]any{"groups": []any{}, "quantity": 9, "packages": 3}, nil
	}}
	auth := &authentication{db: db, sessions: map[string]loginSession{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/accounts/{id}/procurement/ws", cache.stream(auth))
	server := httptest.NewServer(requestBoundary(auth.protect(mux), ""))
	defer server.Close()
	endpoint := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/accounts/" + account + "/procurement/ws?gifts=1"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if conn, resp, err := websocket.Dial(ctx, endpoint, nil); err == nil {
		conn.CloseNow()
		t.Fatal("anonymous allowed")
	} else if resp.StatusCode != 401 {
		t.Fatal(resp.StatusCode)
	}
	login, err := http.Post(server.URL+"/api/login", "application/json", strings.NewReader(`{"username":"admin","password":"123456"}`))
	if err != nil {
		t.Fatal(err)
	}
	cookie := login.Cookies()[0]
	login.Body.Close()
	if _, err = db.Exec(`INSERT INTO app_users(username,password) VALUES('restricted','test-only')`); err != nil {
		t.Fatal(err)
	}
	staffLogin, err := http.Post(server.URL+"/api/login", "application/json", strings.NewReader(`{"username":"restricted","password":"test-only"}`))
	if err != nil {
		t.Fatal(err)
	}
	staffCookie := staffLogin.Cookies()[0]
	staffLogin.Body.Close()
	if conn, resp, err := websocket.Dial(ctx, endpoint, &websocket.DialOptions{HTTPHeader: http.Header{"Cookie": []string{staffCookie.String()}, "Origin": []string{server.URL}}}); err == nil {
		conn.CloseNow()
		t.Fatal("ungranted account allowed")
	} else if resp.StatusCode != 403 {
		t.Fatal(resp.StatusCode)
	}
	headers := http.Header{"Cookie": []string{cookie.String()}, "Origin": []string{server.URL}}
	conn, _, err := websocket.Dial(ctx, endpoint, &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	_, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var event struct {
		Type    string         `json:"type"`
		Account string         `json:"account"`
		Gifts   bool           `json:"gifts"`
		Summary map[string]any `json:"summary"`
	}
	if err = json.Unmarshal(payload, &event); err != nil || event.Type != "procurement_snapshot" || event.Account != account || !event.Gifts || event.Summary["quantity"] != float64(9) || event.Summary["packages"] != float64(3) || event.Summary["revision"] == nil {
		t.Fatal(event, err)
	}
	// Expired/revoked sessions cannot continue receiving business updates.
	auth.mu.Lock()
	auth.sessions = map[string]loginSession{}
	auth.mu.Unlock()
	cache.get(account, true, true)
	if _, _, err = conn.Read(ctx); err == nil {
		t.Fatal("revoked session received snapshot")
	}
}
