package miaoshouopenapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestPurchaseTotalAndRelationContract(t *testing.T) {
	for _, total := range []string{`"1"`, `1`} {
		c := fakeClient(t, func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != purchasePath {
				t.Fatal(r.URL.Path)
			}
			return response(200, `{"result":"success","data":{"list":[{"purchaseOrderSn":"purchase","purchasePlatformName":"1688","opOrderPackageList":[]}],"total":`+total+`}}`), nil
		})
		got, err := c.ListPurchases(context.Background(), 1, 500)
		if err != nil || got.Total != 1 || len(got.List) != 1 {
			t.Fatal(got, err)
		}
	}
}
func TestClassifyNormalRelationRegardlessOfSupplierShipping(t *testing.T) {
	s := PendingSnapshot{Packages: []json.RawMessage{
		json.RawMessage(`{"opOrderPackageId":1,"platform":"tiktok","orderInfo":{"platformOrderSn":9007199254740993}}`),
		json.RawMessage(`{"opOrderPackageId":2,"platform":"tiktok","orderInfo":{"platformOrderSn":"second"}}`),
		json.RawMessage(`{"opOrderPackageId":3,"platform":"shopee","orderInfo":{"platformOrderSn":9007199254740993}}`),
	}, Purchases: []json.RawMessage{json.RawMessage(`{"purchaseOrderSn":"purchase","purchasePlatformName":null,"purchaseOrderStatus":"wait_send","opOrderPackageList":[{"platform":"tiktok","platformOrderSn":9007199254740993,"relateStatus":"normal"},{"platform":"tiktok","platformOrderSn":"second","relateStatus":"cancel"}]}`)}}
	got, err := Classify(s)
	if err != nil || got["1"] != "hasPurchase" || got["2"] != "waitPurchase" || got["3"] != "waitPurchase" {
		t.Fatal(got, err)
	}
	s.Packages = append(s.Packages, json.RawMessage(`{"opOrderPackageId":4,"platform":"tiktok","orderInfo":{"platformOrderSn":9007199254740993}}`))
	if _, err = Classify(s); err == nil {
		t.Fatal("accepted split order ambiguity")
	}
	s.Packages = s.Packages[:3]
	s.Purchases = []json.RawMessage{json.RawMessage(`{}`)}
	if _, err = Classify(s); err == nil {
		t.Fatal("accepted missing relation list")
	}
}
func TestFetchRejectsIncompletePaginationAndCancellation(t *testing.T) {
	c := fakeClient(t, func(r *http.Request) (*http.Response, error) {
		return response(200, `{"result":"success","data":{"orderPackageList":[],"total":1,"page":1,"pageSize":200}}`), nil
	})
	if _, err := c.FetchPending(context.Background(), func(string) {}); err == nil {
		t.Fatal("accepted partial list")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.FetchPending(ctx, func(string) {}); err != context.Canceled {
		t.Fatal(err)
	}
}
func TestFetchRejectsChangedRelationBetweenPasses(t *testing.T) {
	count := 0
	c := fakeClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == listPath {
			return response(200, `{"result":"success","data":{"orderPackageList":[{"opOrderPackageId":1,"platform":"tiktok","orderInfo":{"platformOrderSn":"order"}}],"total":1,"page":1,"pageSize":200}}`), nil
		}
		count++
		status := "normal"
		if count == 2 {
			status = "cancel"
		}
		return response(200, fmt.Sprintf(`{"result":"success","data":{"list":[{"purchaseOrderSn":"purchase","purchasePlatformName":"1688","opOrderPackageList":[{"platform":"tiktok","platformOrderSn":"order","relateStatus":%q}]}],"total":"1"}}`, status)), nil
	})
	if _, err := c.FetchPending(context.Background(), func(string) {}); err == nil {
		t.Fatal("accepted changed procurement membership")
	}
}

func TestBusinessSignatureIgnoresImageAndModifiedTimeOnly(t *testing.T) {
	a, _ := decodeObject(json.RawMessage(`{"opOrderPackageId":1,"items":[{"opOrderItemId":2,"quantity":3,"originalPicUrl":"https://image/a"}],"orderInfo":{"gmtOrderModified":"old"}}`))
	b, _ := decodeObject(json.RawMessage(`{"opOrderPackageId":1,"items":[{"opOrderItemId":2,"quantity":3,"originalPicUrl":"https://image/b"}],"orderInfo":{"gmtOrderModified":"new"}}`))
	if string(businessPackage(a)) != string(businessPackage(b)) {
		t.Fatal("volatile fields affect signature")
	}
	b["items"].([]any)[0].(map[string]any)["quantity"] = 4
	if string(businessPackage(a)) == string(businessPackage(b)) {
		t.Fatal("missed quantity change")
	}
}
