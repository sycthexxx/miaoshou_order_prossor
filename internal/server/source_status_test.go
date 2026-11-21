package server

import (
	"net/http"
	"net/http/httptest"
	"order_processor/internal/store"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceStatusUpdatesWholeCachedSummaryAndPreservesTotal(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, _ := store.Create(db, "a")
	other, _ := store.Create(db, "b")
	_, err = db.Exec(`INSERT INTO shop_products(account_id,platform,shop_id,platform_item_id,payload,captured_at,source_file) VALUES(?,'tiktok','s','p','{"sourceList":[{"source":"1688","sourceItemId":"100"}]}','2026','fixture')`, a)
	if err != nil {
		t.Fatal(err)
	}
	loads := 0
	cache := &procurementCache{load: func(account string, gifts bool) (map[string]any, error) {
		loads++
		status, err := store.GetSourceStatus(db, account, "100")
		return map[string]any{"groups": []map[string]any{{"sourceStatus": status, "url": "https://detail.1688.com/offer/100.html", "variants": []map[string]any{{"spec": "M", "quantity": int64(8)}}}}, "quantity": 8, "packages": 3}, err
	}}
	ch, stop, err := cache.subscribe(a, false)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	initial := takeSnapshot(t, ch)
	gift, stopGift, err := cache.subscribe(a, true)
	if err != nil {
		t.Fatal(err)
	}
	defer stopGift()
	takeSnapshot(t, gift)
	mux := http.NewServeMux()
	sourceStatusRoutes(mux, db, cache)
	put := func(account, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("PUT", "/api/accounts/"+account+"/source-status/100", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	body := `{"state":"invalid","source":"plugin","reason":"商品已下架","evidence":"offer_removed"}`
	w := put(a, body)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"changed":true`) {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, updates := range []<-chan map[string]any{ch, gift} {
		d := takeSnapshot(t, updates)
		if d["quantity"] != 8 || d["packages"] != 3 || d["revision"].(int64) <= initial["revision"].(int64) || d["groups"].([]map[string]any)[0]["sourceStatus"].(store.SourceStatus).State != "invalid" {
			t.Fatal(d)
		}
	}
	cached, _ := cache.get(a, false, false)
	snapshot, found := summaryOfferSnapshot(cached, a, "100")
	if !found || snapshot["sourceStatus"].(store.SourceStatus).State != "invalid" {
		t.Fatal("direct-open snapshot lost source status", snapshot)
	}
	before := loads
	w = put(a, body)
	if w.Code != 200 || loads != before {
		t.Fatal("same status rebuilt summary", loads, before)
	}
	select {
	case <-ch:
		t.Fatal("same status pushed duplicate")
	default:
	}
	if w = put(other, body); w.Code != 400 {
		t.Fatal("foreign source accepted", w.Code)
	}
	if w = put(a, `{"state":"invalid","source":"plugin","reason":"login failed"}`); w.Code != 400 {
		t.Fatal("inconclusive failure accepted", w.Code)
	}
	w = put(a, `{"state":"invalid","source":"manual","reason":"人工确认"}`)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	takeSnapshot(t, ch)
	takeSnapshot(t, gift)
	before = loads
	w = put(a, `{"state":"valid","source":"plugin"}`)
	if w.Code != 200 || loads != before || !strings.Contains(w.Body.String(), `"retainedManual":true`) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = put(a, `{"state":"valid","source":"manual"}`)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	d := takeSnapshot(t, ch)
	if d["groups"].([]map[string]any)[0]["sourceStatus"].(store.SourceStatus).State != "valid" {
		t.Fatal(d)
	}
}
