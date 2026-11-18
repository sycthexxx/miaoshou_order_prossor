package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"order_processor/internal/adapters/miaoshou/web/browser"
	app "order_processor/internal/application/packing"
	"order_processor/internal/domain/packing"
	"order_processor/internal/store"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPackingJobHTTPSourceSelectionAndSinglePackageWrites(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, _ := store.Create(db, "test")
	product := `{"platform":"tiktok","shopId":"shop","platformItemId":"product","sourceList":[{"source":"1688","sourceItemId":"100","sourceItemUrl":"https://detail.1688.com/offer/100.html"}]}`
	if _, err = db.Exec(`INSERT INTO shop_products(account_id,platform,shop_id,platform_item_id,payload,captured_at,source_file) VALUES(?,'tiktok','shop','product',?,'2026','test')`, account, product); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"1", "2"} {
		payload := fmt.Sprintf(`{"opOrderPackageId":"%s","appPackageNo":"MS%s","platform":"tiktok","shopId":"shop","appPackageStatus":"wait_seller_send","appPackageOperateStatus":"wait_process","appPurchaseStatus":"0","items":[{"opOrderItemId":"%s1","platformItemId":"product","platformSkuId":"sku","skuSubName":"黑色,L","quantity":1,"refundQuantity":0},{"opOrderItemId":"gift","isGift":"1","quantity":1},{"opOrderItemId":"other","quantity":1}]}`, id, id, id)
		if _, err = db.Exec(`INSERT INTO packages(account_id,package_id,payload,captured_at,source_file) VALUES(?,?,?,'2026','test')`, account, id, payload); err != nil {
			t.Fatal(err)
		}
	}
	calls := []string{}
	shippingCalls := []string{}
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/waybill") {
			var in struct {
				Action    string `json:"action"`
				PackageID string `json:"packageId"`
			}
			json.NewDecoder(r.Body).Decode(&in)
			shippingCalls = append(shippingCalls, in.Action+":"+in.PackageID)
			body := `{"result":"success","successCount":1,"failCount":0,"errors":[]}`
			if in.Action == "inspect" {
				body = fmt.Sprintf(`{"result":"success","orderPackageDetailInfo":{"opOrderPackageId":"%s","logisticsNo":null,"gmtApplyLogisticsNo":null,"isApplyTrackingNoFail":"0"}}`, in.PackageID)
			}
			json.NewEncoder(w).Encode(map[string]any{"status": 200, "body": body})
			return
		}
		var body struct {
			List []struct {
				ID    string                     `json:"opOrderPackageId"`
				Items map[string]json.RawMessage `json:"opOrderItemIdAndPurchaseOrderInfoMap"`
			} `json:"purchaseOrderInfoList"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body.List) != 1 {
			t.Error("more than one package")
		}
		values := body.List[0].Items
		if len(values) != 2 || string(values["gift"]) != string(values[body.List[0].ID+"1"]) {
			t.Errorf("gift missing or submission info differs: %s", values)
		}
		label, err := (store.PackingRepository{DB: db}).ShippingLabel(context.Background(), account, body.List[0].ID)
		if err != nil || label.State != "completed" {
			t.Error("packing started before durable label success", label, err)
		}
		shippingCalls = append(shippingCalls, "packing:"+body.List[0].ID)
		calls = append(calls, body.List[0].ID)
		json.NewEncoder(w).Encode(map[string]any{"status": 200, "body": `{"result":"success","successCount":1,"failCount":0,"errors":[]}`})
	}))
	defer worker.Close()
	u, _ := url.Parse(worker.URL)
	g := &browserGateway{db: db, runtime: fixedRuntime{browser.Target{API: u, Token: "fixture"}}}
	mux := http.NewServeMux()
	packingJobRoutes(nil, mux, g)
	registerSupplierLocks(mux, db)
	actor := loginUser{ID: 1, Username: "管理员"}
	req := httptest.NewRequest("POST", "/api/accounts/"+account+"/supplier-locks/0", nil)
	grant := httptest.NewRecorder()
	mux.ServeHTTP(grant, req.WithContext(context.WithValue(req.Context(), userContextKey{}, actor)))
	var lease store.SupplierLease
	if grant.Code != 200 || json.Unmarshal(grant.Body.Bytes(), &lease) != nil || lease.ID == 0 {
		t.Fatal(grant.Code, grant.Body.String())
	}
	body := `{"name":"batch","sourceKey":"source:[\"1688\",\"\",\"100\"]","intervalSeconds":5,"selections":[{"packageId":"1","itemId":"11"},{"packageId":"2","itemId":"21"}],"info":{"purchase":{"platform":"1688","orderNumber":"001","buyer":"buyer","seller":"seller"},"autoSyncAmount":true}}`
	create := func(user loginUser) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/accounts/"+account+"/packing-jobs", strings.NewReader(body))
		req.Header.Set("X-Supplier-Lease", fmt.Sprint(lease.ID))
		req = req.WithContext(context.WithValue(req.Context(), userContextKey{}, user))
		mux.ServeHTTP(w, req)
		return w
	}
	if refused := create(loginUser{ID: 2, Username: "另一用户"}); refused.Code != 409 {
		t.Fatal("another user submitted with a copied lease", refused.Code)
	}
	w := create(actor)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if len(calls) != 0 {
		t.Fatal("creation sent synchronous writes")
	}
	if w = create(actor); w.Code != 409 {
		t.Fatal("duplicate queued selections accepted", w.Code)
	}
	repo := store.PackingJobs{DB: db}
	q := app.Queue{Repository: repo, Submitter: g}
	now := time.Now()
	for n := 0; n < 2; n++ {
		if claimed, err := q.Step(context.Background(), now.Add(time.Duration(n)*time.Minute)); err != nil || !claimed {
			t.Fatal(claimed, err)
		}
	}
	jobs, err := repo.List(context.Background(), account)
	if err != nil || len(jobs) != 1 || jobs[0].State != "completed" || jobs[0].Counts["completed"] != 2 {
		t.Fatal(jobs, err)
	}
	detail, e := repo.Detail(context.Background(), account, jobs[0].ID)
	if e != nil || len(detail.Items[0].Lines) != 2 || !detail.Items[0].Lines[1].Gift {
		t.Fatal("gift missing from persisted task", detail, e)
	}
	if len(calls) != 2 || calls[0] != "1" || calls[1] != "2" {
		t.Fatal(calls)
	}
	var unchanged string
	db.QueryRow("SELECT payload FROM packages WHERE package_id='1'").Scan(&unchanged)
	if !strings.Contains(unchanged, `"appPurchaseStatus":"0"`) {
		t.Fatal("local group changed without sync")
	}
	// Completed task editing uses saved scope and only queues a new task.
	previewResponse := httptest.NewRecorder()
	mux.ServeHTTP(previewResponse, httptest.NewRequest("GET", fmt.Sprintf("/api/accounts/%s/packing-jobs/%d/rerun-preview", account, jobs[0].ID), nil))
	if previewResponse.Code != 200 {
		t.Fatal(previewResponse.Code, previewResponse.Body.String())
	}
	var preview packing.Job
	if err := json.Unmarshal(previewResponse.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	in := store.PackingRerunInput{IntervalSeconds: 5}
	for _, item := range preview.Items {
		in.Requests = append(in.Requests, item.Request)
	}
	data, _ := json.Marshal(in)
	retryResponse := httptest.NewRecorder()
	mux.ServeHTTP(retryResponse, httptest.NewRequest("POST", fmt.Sprintf("/api/accounts/%s/packing-jobs/%d/rerun", account, jobs[0].ID), strings.NewReader(string(data))))
	if retryResponse.Code != 201 || len(calls) != 2 {
		t.Fatal("rerun route failed or sent synchronously", retryResponse.Code, retryResponse.Body.String(), calls)
	}
	want := "inspect:1,check:1,apply:1,packing:1,inspect:2,check:2,apply:2,packing:2"
	if strings.Join(shippingCalls, ",") != want {
		t.Fatal(shippingCalls)
	}
	for n := 0; n < 2; n++ {
		if claimed, e := q.Step(context.Background(), now.Add(time.Duration(n+2)*time.Minute)); e != nil || !claimed {
			t.Fatal(claimed, e)
		}
	}
	if strings.Join(shippingCalls, ",") != want+",packing:1,packing:2" {
		t.Fatal("rerun reapplied label", shippingCalls)
	}
	// Direct submissions also share the global gate across accounts.
	g.packingMu.Lock()
	_, err = g.Submit(context.Background(), account, packing.Request{})
	g.packingMu.Unlock()
	if err != errPackingBusy {
		t.Fatal("global submission gate bypassed", err)
	}
}
