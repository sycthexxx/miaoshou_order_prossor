package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"order_processor/internal/store"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestProcurementCacheReuseRefreshAndIsolation(t *testing.T) {
	var calls atomic.Int32
	c := procurementCache{load: func(account string, gifts bool) (map[string]any, error) {
		return map[string]any{"generation": calls.Add(1), "account": account, "gifts": gifts}, nil
	}}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.get("a", false, false); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
	first, _ := c.get("a", false, false)
	second, _ := c.get("a", false, true)
	if first["generation"] == second["generation"] {
		t.Fatal("refresh reused generation")
	}
	same, _ := c.get("a", false, false)
	if same["generation"] != second["generation"] || same["createdAt"] != second["createdAt"] {
		t.Fatal(same)
	}
	c.get("a", true, false)
	c.get("b", false, false)
	if calls.Load() != 4 {
		t.Fatal(calls.Load())
	}
	// Failure leaves the last successful cache intact.
	c.load = func(string, bool) (map[string]any, error) { return nil, errors.New("failed") }
	if _, err := c.get("a", false, true); err == nil {
		t.Fatal("expected failure")
	}
	same, _ = c.get("a", false, false)
	if same["generation"] != second["generation"] {
		t.Fatal(same)
	}
}

func TestProcurementOfferAggregationIsNarrow(t *testing.T) {
	group := func(url, id, spec string, n int64) map[string]any {
		return map[string]any{"url": url, "source": map[string]any{"source": "1688", "sourceItemId": id}, "title": "商品", "supplier": map[string]any{"name": "private seller"}, "image": map[string]any{"buyerName": "secret buyer"}, "variants": []map[string]any{{"spec": spec, "quantity": n, "images": []string{"https://img.example/1.jpg", "javascript:bad"}, "lines": []map[string]any{{"orderNo": "secret order"}}}}}
	}
	d := map[string]any{"createdAt": int64(123), "groups": []map[string]any{group("https://detail.1688.com/offer/42.html?x=1", "999", "红,M", 2), group("", "42", "红,M", 3), group("https://detail.1688.com/offer/55.html", "42", "红,M", 100)}}
	s, ok := summaryOfferSnapshot(d, "a", "42")
	if !ok {
		t.Fatal("missing")
	}
	v := s["variants"].([]map[string]any)
	if len(v) != 1 || v[0]["quantity"] != int64(5) || len(v[0]["images"].([]string)) != 1 {
		t.Fatal(v)
	}
	b, _ := json.Marshal(s)
	for _, forbidden := range []string{"secret", "buyer", "orderNo", "lines", "supplier", "999"} {
		if strings.Contains(string(b), forbidden) {
			t.Fatal(string(b))
		}
	}
	if s["createdAt"] != int64(123) || s["account"] != "a" {
		t.Fatal(s)
	}
	if _, ok := summaryOfferSnapshot(d, "a", "404"); ok {
		t.Fatal("unexpected hit")
	}
}

func TestProcurementRoutesCacheUntilExplicitRefresh(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, _ := store.Create(db, "a")
	b, _ := store.Create(db, "b")
	source := `{"sourceList":[{"source":"1688","sourceItemId":"42","sourceSite":"cn","sourceItemUrl":"https://detail.1688.com/offer/42.html"}]}`
	db.Exec(`INSERT INTO shop_products(account_id,platform,shop_id,platform_item_id,payload,captured_at,source_file) VALUES(?,'tiktok','shop','p',?,'2026','fixture')`, a, source)
	set := func(n int) {
		t.Helper()
		p := fmt.Sprintf(`{"opOrderPackageId":"1","appPackageStatus":"wait_seller_send","appPackageOperateStatus":"wait_audit","appPurchaseStatus":"0","items":[{"opOrderItemId":"i","platform":"tiktok","shopId":"shop","platformItemId":"p","quantity":"%d","skuSubName":"红,M","title":"商品"}]}`, n)
		if _, err := db.Exec(`INSERT INTO packages(account_id,package_id,payload,captured_at,source_file) VALUES(?,'1',?,'2026','fixture') ON CONFLICT(account_id,package_id) DO UPDATE SET payload=excluded.payload`, a, p); err != nil {
			t.Fatal(err)
		}
	}
	set(2)
	h := authenticatedTestServer(t, db, dir)
	get := func(path string) map[string]any {
		t.Helper()
		r := httptest.NewRequest("GET", "http://localhost"+path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var x map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &x); err != nil {
			t.Fatal(err)
		}
		return x
	}
	base := "/api/accounts/" + a + "/procurement"
	first := get(base)
	if first["quantity"] != float64(2) {
		t.Fatal(first)
	}
	set(5)
	cached := get(base)
	if cached["quantity"] != float64(2) || cached["createdAt"] != first["createdAt"] {
		t.Fatal(cached)
	}
	offer := get(base + "/offers/42")
	if offer["found"] != true {
		t.Fatal(offer)
	}
	snap := offer["snapshot"].(map[string]any)
	if snap["variants"].([]any)[0].(map[string]any)["quantity"] != float64(2) {
		t.Fatal(snap)
	}
	get(base + "?refresh=1")
	offer = get(base + "/offers/42")
	if offer["snapshot"].(map[string]any)["variants"].([]any)[0].(map[string]any)["quantity"] != float64(5) {
		t.Fatal(offer)
	}
	if get(base + "/offers/404")["found"] != false || get("/api/accounts/" + b + "/procurement/offers/42")["found"] != false {
		t.Fatal("cross account or absent offer matched")
	}
}

func TestConcurrentRefreshCannotRestoreOlderGeneration(t *testing.T) {
	var calls atomic.Int32
	c := procurementCache{load: func(string, bool) (map[string]any, error) { return map[string]any{"generation": calls.Add(1)}, nil }}
	if _, err := c.get("a", false, false); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.get("a", false, true); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	last, err := c.get("a", false, false)
	if err != nil || last["generation"] != calls.Load() || calls.Load() != 13 {
		t.Fatal(last, calls.Load(), err)
	}
}
