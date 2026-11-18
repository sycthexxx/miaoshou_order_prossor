package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"order_processor/internal/adapters/miaoshou/web/browser"
	"order_processor/internal/store"
	"path/filepath"
	"testing"
)

func TestAmountDetailIsTransientAndAccountScoped(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, _ := store.Create(db, "test")
	payload := `{"opOrderId":2209871569,"opOrderPackageId":"2178796421"}`
	db.Exec(`INSERT INTO packages(account_id,package_id,payload,captured_at,source_file) VALUES(?,'2178796421',?,'2026','test')`, account, payload)
	calls := 0
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var in map[string]string
		json.NewDecoder(r.Body).Decode(&in)
		if in["orderId"] != "2209871569" || in["packageId"] != "2178796421" {
			t.Error(in)
		}
		json.NewEncoder(w).Encode(map[string]any{"status": 200, "body": `{"amount":{"orderProfit":"36.14"},"order":{"orderNo":"587229498025411885"}}`})
	}))
	defer worker.Close()
	u, _ := url.Parse(worker.URL)
	g := &browserGateway{db: db, runtime: fixedRuntime{browser.Target{API: u, Token: "test"}}, client: worker.Client()}
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/accounts/{id}/packages/{package}/amount-detail", g.amountDetail)
	for n := 0; n < 2; n++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/accounts/"+account+"/packages/2178796421/amount-detail", nil))
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if calls != 2 {
		t.Fatal("lookup cached", calls)
	}
	var after string
	db.QueryRow(`SELECT payload FROM packages WHERE account_id=?`, account).Scan(&after)
	if after != payload {
		t.Fatal("read modified stored order")
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/accounts/999/packages/2178796421/amount-detail", nil))
	if w.Code != 404 || calls != 2 {
		t.Fatal("account scope bypassed")
	}
}
