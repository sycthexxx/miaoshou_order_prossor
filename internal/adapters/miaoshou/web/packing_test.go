package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"order_processor/internal/adapters/miaoshou/web/browser"
	"order_processor/internal/domain/packing"
	"testing"
)

func TestPackingFormModes(t *testing.T) {
	p := PackingPayload(packing.Request{PackageID: "2177432687", Items: []packing.Item{
		{OrderItemID: "2616118132", AutoSyncAmount: true, Purchase: &packing.Purchase{Platform: "1688", OrderNumber: "5127601970660025941", Buyer: "buyer", Seller: "seller", Payment: "33.9", Status: "wait_send"}},
		{OrderItemID: "2616118129", Amount: "1.5", Waybills: []packing.Waybill{{Company: "中通快递", Number: "001234"}}},
	}})
	group := p["purchaseOrderInfoList"].([]any)[0].(map[string]any)
	if group["opOrderPackageId"] != "2177432687" || group["forwarderId"] != "0" {
		t.Fatal(group)
	}
	items := group["opOrderItemIdAndPurchaseOrderInfoMap"].(map[string]any)
	purchase := items["2616118132"].([]any)[0].(map[string]any)
	if purchase["purchaseOrderSn"] != "5127601970660025941" || purchase["isAutoRsyncPurchasePayment"] != "1" || purchase["purchaseOrderPayment"] != "33.9" {
		t.Fatal(purchase)
	}
	waybill := items["2616118129"].(map[string]any)
	if waybill["purchasePlatform"] != "other" || waybill["isAutoRsyncPurchasePayment"] != "0" || waybill["itemPurchaseAmount"] != "1.5" {
		t.Fatal(waybill)
	}
	w := waybill["purchaseOrderPackages"].([]any)[0].(map[string]string)
	if w["purchaseOrderWaybillCode"] != "001234" || w["purchaseOrderLogisticsName"] != "中通快递" {
		t.Fatal(w)
	}
}
func TestPackingResultRequiresBusinessSuccess(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{`{"result":"success","errors":[],"successCount":1,"failCount":0}`, "completed"},
		{`{"result":"success","errors":[{"message":"商品不存在"}],"successCount":0,"failCount":1}`, "failed"},
		{`{"result":"success","errors":["一条失败"],"successCount":1,"failCount":1}`, "partial"},
		{`{"result":"fail","message":"登录失效"}`, "failed"},
		{`{"result":"success"}`, "unknown"},
		{`<html>登录</html>`, "unknown"},
	} {
		t.Run(tc.want+tc.body, func(t *testing.T) {
			r := packingResult(200, tc.body, "receipt")
			if r.State != tc.want || r.Archive != "receipt" {
				t.Fatal(r)
			}
		})
	}
}

type packingRuntime struct{ target browser.Target }

func (r packingRuntime) Resolve(context.Context, string, bool) (browser.Target, error) {
	return r.target, nil
}
func (r packingRuntime) Remote() bool { return true }
func TestPackingWorkerFailureNeverRetries(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/accounts/old-key/packing" || r.Header.Get("Authorization") != "Bearer worker" {
			t.Error("wrong worker route")
		}
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		if in["purchaseOrderInfoList"] == nil {
			t.Error("not web payload")
		}
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"connection lost","state":"unknown","archive":"runtime/old-key/packing/1"}`))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	s := PackingSubmitter{Runtime: packingRuntime{browser.Target{API: u, Token: "worker"}}, Client: srv.Client(), StorageKey: func(string) string { return "old-key" }}
	r, err := s.Submit(context.Background(), "1", packing.Request{PackageID: "1"})
	if err != nil || calls != 1 || r.State != "unknown" || r.Archive == "" {
		t.Fatal(r, err, calls)
	}
}
