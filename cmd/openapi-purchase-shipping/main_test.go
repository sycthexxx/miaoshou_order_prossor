package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	api "order_processor/internal/adapters/miaoshou/openapi"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPreviewNeedsNeitherDatabaseNorCredentials(t *testing.T) {
	dir := t.TempDir()
	request := filepath.Join(dir, "request.json")
	db := filepath.Join(dir, "missing.db")
	os.WriteFile(request, []byte(`{"goodsPurchaseOrderId":"test-only","purchaseOrderInfo":{"purchaseOrderSn":"123"}}`), 0600)
	var out bytes.Buffer
	if err := run(context.Background(), []string{"--request", request, "--db", db}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "仅预览") {
		t.Fatal(out.String())
	}
	if _, err := os.Stat(db); !os.IsNotExist(err) {
		t.Fatal("preview touched database")
	}
	for _, invalid := range []string{`{"goodsPurchaseOrderId":""}`, `{"goodsPurchaseOrderId":"1","opOrderPackageId":"2"}`, `{"goodsPurchaseOrderId":"1"} {}`} {
		os.WriteFile(request, []byte(invalid), 0600)
		if err := run(context.Background(), []string{"--request", request}, io.Discard); err == nil {
			t.Fatal("invalid input accepted", invalid)
		}
	}
}

func TestSubmitArchivesExactRequestAndCompleteResponse(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		file   string
	}{
		{200, `{"result":"success","code":"success"}`, "response.json"},
		{200, `{"result":"fail","code":"badRequest","message":"完整错误","id":9007199254740993}`, "response.json"},
		{502, "<html>upstream error</html>", "response.txt"},
	} {
		dir := t.TempDir()
		calls := 0
		var sent []byte
		client, err := api.New(api.Credentials{AppKey: "fake-app", AppSecret: "fake-secret"}, api.Options{Transport: transport(func(r *http.Request) (*http.Response, error) {
			calls++
			sent, _ = io.ReadAll(r.Body)
			return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
		})})
		if err != nil {
			t.Fatal(err)
		}
		err = submitAndSave(context.Background(), client, api.UpdatePurchaseShippingRequest{GoodsPurchaseOrderID: "123"}, dir, io.Discard)
		if (err == nil) != strings.Contains(tc.body, `"result":"success"`) || calls != 1 {
			t.Fatal(calls, err)
		}
		request, _ := os.ReadFile(filepath.Join(dir, "request.json"))
		response, _ := os.ReadFile(filepath.Join(dir, tc.file))
		if !bytes.Equal(request, sent) || string(response) != tc.body {
			t.Fatal("archive differs from transmitted request or received response")
		}
		files, _ := os.ReadDir(dir)
		for _, file := range files {
			content, _ := os.ReadFile(filepath.Join(dir, file.Name()))
			if strings.Contains(string(content), "fake-secret") || strings.Contains(string(content), "x-sign") {
				t.Fatal("credential leaked into archive")
			}
		}
		// If request evidence cannot be written, no mutation should be attempted.
		if err = submitAndSave(context.Background(), client, api.UpdatePurchaseShippingRequest{GoodsPurchaseOrderID: "123"}, filepath.Join(dir, "absent"), io.Discard); err == nil || calls != 1 {
			t.Fatal("sent without saving request")
		}
	}
}
