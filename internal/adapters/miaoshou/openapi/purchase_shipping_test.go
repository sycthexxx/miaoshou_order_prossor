package miaoshouopenapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"testing"
)

func TestPurchaseShippingSignsWriteAndAcceptsSuccessWithoutData(t *testing.T) {
	calls := 0
	c := fakeClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(r.Body)
		want := `{"goodsPurchaseOrderId":"9007199254740993","purchaseOrderInfo":{"purchasePlatform":"1688","purchaseOrderSn":"9007199254740993999","purchaseOrderBuyer":"buyer","purchaseOrderSeller":"seller","purchaseOrderPackages":[{"purchaseOrderLogisticsName":"圆通速递","purchaseOrderWaybillCode":"YT123"}]}}`
		if string(body) != want || r.URL.String() != BaseURL+UpdatePurchaseShippingPath || r.Method != "POST" {
			t.Fatal("write contract mismatch", string(body), r.URL)
		}
		mac := hmac.New(sha256.New, []byte("fake-secret"))
		mac.Write([]byte("fake-secret" + UpdatePurchaseShippingPath + "1700000000fake-app" + want + "fake-secret"))
		if r.Header.Get("x-sign") != hex.EncodeToString(mac.Sum(nil)) || r.Header.Get("x-sub-account-id") != "fake-sub" {
			t.Fatal("write signature mismatch")
		}
		return response(200, `{"result":"success","code":"success"}`), nil
	})
	out, err := c.UpdatePurchaseShipping(context.Background(), UpdatePurchaseShippingRequest{
		GoodsPurchaseOrderID: "9007199254740993",
		PurchaseOrderInfo:    &PurchaseOrderInfo{Platform: "1688", Number: "9007199254740993999", Buyer: "buyer", Seller: "seller", Packages: []PurchaseWaybill{{Company: "圆通速递", Number: "YT123"}}},
	})
	if err != nil || out.Status != 200 || calls != 1 || string(out.Body) != `{"result":"success","code":"success"}` {
		t.Fatal(out, calls, err)
	}
}

func TestPurchaseShippingDoesNotRetryAndKeepsFailureBody(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{
		{429, `{"message":"rate limited"}`},
		{502, "<html>gateway failure</html>"},
		{503, "unavailable"},
		{504, "timeout"},
		{200, `{"result":"fail","code":"notFound","message":"采购单不存在"}`},
		{200, `{"result":"success","code":"unexpectedCode"}`},
		{200, "not JSON"},
	} {
		calls := 0
		c := fakeClient(t, func(*http.Request) (*http.Response, error) {
			calls++
			return response(tc.status, tc.body), nil
		})
		out, err := c.UpdatePurchaseShipping(context.Background(), UpdatePurchaseShippingRequest{GoodsPurchaseOrderID: "123", ShippingID: "456"})
		if err == nil || calls != 1 || out.Status != tc.status || string(out.Body) != tc.body {
			t.Fatal(tc, calls, out, err)
		}
	}
	calls := 0
	c := fakeClient(t, func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("connection lost") })
	if _, err := c.UpdatePurchaseShipping(context.Background(), UpdatePurchaseShippingRequest{}); err == nil || calls != 0 {
		t.Fatal("empty purchase ID sent")
	}
	if _, err := c.UpdatePurchaseShipping(context.Background(), UpdatePurchaseShippingRequest{GoodsPurchaseOrderID: "123"}); err == nil || calls != 1 {
		t.Fatal("transport failure retried or hidden")
	}
}
