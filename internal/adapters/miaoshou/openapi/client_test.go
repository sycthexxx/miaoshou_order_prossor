package miaoshouopenapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func fakeClient(t *testing.T, f transport) *Client {
	t.Helper()
	c, e := New(Credentials{AppKey: "fake-app", AppSecret: "fake-secret", SubAccountID: "fake-sub"}, Options{Transport: f})
	if e != nil {
		t.Fatal(e)
	}
	c.now = func() time.Time { return time.Unix(1700000000, 0) }
	return c
}
func TestListSignsExactJSONAndPreservesIDs(t *testing.T) {
	c := fakeClient(t, func(r *http.Request) (*http.Response, error) {
		body, e := io.ReadAll(r.Body)
		if e != nil {
			t.Fatal(e)
		}
		expected := `{"page":1,"pageSize":200,"platform":"tiktok","shopIds":[9007199254740993],"platformOrderSns":"订单一,订单二"}`
		if string(body) != expected || r.URL.String() != BaseURL+listPath || r.Method != "POST" {
			t.Fatal("request contract mismatch", string(body))
		}
		if r.Header.Get("Content-Type") != "application/json" || r.Header.Get("x-app-key") != "fake-app" || r.Header.Get("x-timestamp") != "1700000000" || r.Header.Get("x-sub-account-id") != "fake-sub" {
			t.Fatal("header contract mismatch")
		}
		mac := hmac.New(sha256.New, []byte("fake-secret"))
		mac.Write([]byte("fake-secret" + listPath + "1700000000fake-app" + expected + "fake-secret"))
		if r.Header.Get("x-sign") != hex.EncodeToString(mac.Sum(nil)) {
			t.Fatal("signature mismatch")
		}
		return response(200, `{"result":"success","code":"success","data":{"orderPackageList":[{"platformOrderId":9007199254740993,"items":[{"skuId":9223372036854775807}]}],"total":1,"page":1,"pageSize":200}}`), nil
	})
	out, e := c.ListPackages(context.Background(), ListRequest{Page: 1, PageSize: 200, Platform: "tiktok", ShopIDs: []int64{9007199254740993}, PlatformOrderSns: "订单一,订单二"})
	if e != nil {
		t.Fatal(e)
	}
	if out.Total != 1 || len(out.OrderPackageList) != 1 || !strings.Contains(string(out.OrderPackageList[0]), "9007199254740993") || !strings.Contains(string(out.OrderPackageList[0]), "9223372036854775807") {
		t.Fatal(out)
	}
}
func TestDetailOptionalCodeAndSubAccount(t *testing.T) {
	c, e := New(Credentials{AppKey: "fake-app", AppSecret: "fake-secret"}, Options{Transport: transport(func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		if string(b) != `{"opOrderPackageId":9007199254740993}` || r.URL.Path != detailPath || r.Header.Get("x-sub-account-id") != "" {
			t.Fatal("wrong detail request")
		}
		return response(200, `{"result":"success","data":{"orderPackageInfo":{"opOrderPackageId":9007199254740993,"skuId":"9223372036854775808"}}}`), nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, e := c.GetPackage(context.Background(), 9007199254740993)
	if e != nil || !json.Valid(p) || !strings.Contains(string(p), "9007199254740993") {
		t.Fatal(string(p), e)
	}
}
func TestValidationDoesNotSend(t *testing.T) {
	c := fakeClient(t, func(*http.Request) (*http.Response, error) { t.Fatal("invalid request was sent"); return nil, nil })
	for _, r := range []ListRequest{{Page: 0, PageSize: 10}, {Page: 1, PageSize: 9}, {Page: 1, PageSize: 201}, {Page: 1, PageSize: 10, ShopIDs: []int64{0}}} {
		if _, e := c.ListPackages(context.Background(), r); e == nil {
			t.Fatal(r)
		}
	}
	if _, e := c.GetPackage(context.Background(), 0); e == nil {
		t.Fatal("accepted zero ID")
	}
	if _, e := New(Credentials{}, Options{}); e == nil {
		t.Fatal("accepted empty credentials")
	}
}
func TestRejectRedirectWithoutSendingCredentialsToTarget(t *testing.T) {
	calls := 0
	c := fakeClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "openapi-erp.91miaoshou.com" {
			t.Fatal("credential request left fixed host")
		}
		v := response(307, "")
		v.Header.Set("Location", "https://other.invalid/collect")
		return v, nil
	})
	_, e := c.GetPackage(context.Background(), 1)
	if e == nil || calls != 1 {
		t.Fatal(calls, e)
	}
}
func TestErrorsDoNotEchoRemoteContentOrCredentials(t *testing.T) {
	for _, body := range []string{`{"result":"fail","code":"signInvalid","message":"fake-secret private buyer"}`, `{"result":"success","code":"failure","data":{}}`, `{"code":"success","data":{}}`, `{"result":"success","data":null}`, `not json fake-secret`} {
		c := fakeClient(t, func(*http.Request) (*http.Response, error) { return response(200, body), nil })
		_, e := c.GetPackage(context.Background(), 1)
		if e == nil || strings.Contains(e.Error(), "fake-secret") || strings.Contains(e.Error(), "buyer") {
			t.Fatal(e)
		}
	}
	c := fakeClient(t, func(*http.Request) (*http.Response, error) { return nil, errors.New("fake-secret transport internal") })
	_, e := c.GetPackage(context.Background(), 1)
	if e == nil || strings.Contains(e.Error(), "fake-secret") {
		t.Fatal(e)
	}
}
func TestMissingListAndDetailPayloadRejected(t *testing.T) {
	c := fakeClient(t, func(*http.Request) (*http.Response, error) {
		return response(200, `{"result":"success","code":"success","data":{}}`), nil
	})
	if _, e := c.ListPackages(context.Background(), ListRequest{Page: 1, PageSize: 10}); e == nil {
		t.Fatal("accepted missing list")
	}
	if _, e := c.GetPackage(context.Background(), 1); e == nil {
		t.Fatal("accepted missing detail")
	}
}

func TestObserverReceivesFullEnvelopeAndQuery(t *testing.T) {
	observed := false
	c, err := New(Credentials{AppKey: "fake", AppSecret: "fake"}, Options{
		Transport: transport(func(r *http.Request) (*http.Response, error) {
			return response(200, `{"result":"success","extra":{"state":"ready"},"data":{"orderPackageInfo":{"opOrderPackageId":7},"more":{"purchaseStatus":2}}}`), nil
		}),
		ObserveResponse: func(path string, request, raw json.RawMessage) error {
			observed = true
			if path != detailPath || string(request) != `{"opOrderPackageId":7}` || !strings.Contains(string(raw), `"purchaseStatus":2`) || !strings.Contains(string(raw), `"extra"`) {
				t.Fatal("incomplete observation")
			}
			raw[0] = '!'
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.GetPackage(context.Background(), 7)
	if err != nil || !observed || !json.Valid(result) {
		t.Fatal("observer changed caller behavior", err)
	}
}

func TestReadOnlyGatewayRetryPreservesSignedBody(t *testing.T) {
	calls := 0
	var firstBody, firstSignature string
	c := fakeClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if calls == 1 {
			firstBody = string(body)
			firstSignature = r.Header.Get("x-sign")
			return response(504, "gateway timeout"), nil
		}
		if string(body) != firstBody || r.Header.Get("x-sign") != firstSignature {
			t.Fatal("retry changed signed request")
		}
		return response(200, `{"result":"success","data":{"orderPackageList":[],"total":0,"page":1,"pageSize":200}}`), nil
	})
	if _, err := c.ListPackages(context.Background(), ListRequest{Page: 1, PageSize: 200}); err != nil || calls != 2 {
		t.Fatal(calls, err)
	}
}
