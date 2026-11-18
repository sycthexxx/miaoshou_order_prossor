// Package miaoshouopenapi adapts the official Miaoshou ERP API.
// Credentials are supplied by the caller and kept in memory.
package miaoshouopenapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const BaseURL = "https://openapi-erp.91miaoshou.com"
const listPath = "/open/v1/order/package/fetch/search_package_list"
const detailPath = "/open/v1/order/package/fetch/get_package_info"

// Credentials are supplied by the caller. The API client does not persist or log them.
type Credentials struct{ AppKey, AppSecret, SubAccountID string }

// Options permits transport injection for offline tests. Production uses the
// default HTTP transport, a bounded timeout, and always rejects redirects.
type Options struct {
	Transport http.RoundTripper
	Timeout   time.Duration
	// ObserveResponse receives a copy of the validated response for diagnostics.
	// It includes the envelope and may contain private order data.
	ObserveResponse func(path string, request, response json.RawMessage) error
}
type Client struct {
	credentials     Credentials
	http            *http.Client
	now             func() time.Time
	observeResponse func(string, json.RawMessage, json.RawMessage) error
}

func New(credentials Credentials, options Options) (*Client, error) {
	if strings.TrimSpace(credentials.AppKey) == "" || strings.TrimSpace(credentials.AppSecret) == "" {
		return nil, errors.New("miaoshou: application credentials are required")
	}
	if strings.ContainsAny(credentials.AppKey+credentials.SubAccountID, "\r\n") {
		return nil, errors.New("miaoshou: invalid credential header")
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{credentials: credentials, now: time.Now, observeResponse: options.ObserveResponse, http: &http.Client{Transport: options.Transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

type ListRequest struct {
	Page             int     `json:"page"`
	PageSize         int     `json:"pageSize"`
	Platform         string  `json:"platform,omitempty"`
	ShopIDs          []int64 `json:"shopIds,omitempty"`
	AppPackageStatus string  `json:"appPackageStatus,omitempty"`
	PlatformOrderSns string  `json:"platformOrderSns,omitempty"`
	GmtCreateFrom    string  `json:"gmtCreateFrom,omitempty"`
	GmtCreateTo      string  `json:"gmtCreateTo,omitempty"`
	GmtModifiedFrom  string  `json:"gmtModifiedFrom,omitempty"`
	GmtModifiedTo    string  `json:"gmtModifiedTo,omitempty"`
	AppPackageTab    string  `json:"appPackageTab,omitempty"`
}

// Individual packages are retained as original JSON, including large numeric
// platform IDs. Callers should use json.Decoder.UseNumber when decoding them.
type ListResponse struct {
	OrderPackageList []json.RawMessage `json:"orderPackageList"`
	Total            int64             `json:"total"`
	Page             int               `json:"page"`
	PageSize         int               `json:"pageSize"`
}

func (c *Client) ListPackages(ctx context.Context, request ListRequest) (ListResponse, error) {
	var result ListResponse
	if request.Page < 1 || request.PageSize < 10 || request.PageSize > 200 {
		return result, errors.New("miaoshou: page must be positive and pageSize must be 10..200")
	}
	for _, id := range request.ShopIDs {
		if id <= 0 {
			return result, errors.New("miaoshou: shop IDs must be positive")
		}
	}
	raw, err := c.post(ctx, listPath, request)
	if err != nil {
		return result, err
	}
	var data struct {
		OrderPackageList *[]json.RawMessage `json:"orderPackageList"`
		Total            *int64             `json:"total"`
		Page             *int               `json:"page"`
		PageSize         *int               `json:"pageSize"`
	}
	if json.Unmarshal(raw, &data) != nil || data.OrderPackageList == nil || data.Total == nil || data.Page == nil || data.PageSize == nil || *data.Total < 0 || *data.Page < 1 || *data.PageSize < 10 || *data.PageSize > 200 {
		return result, errors.New("miaoshou: invalid package list response")
	}
	result = ListResponse{OrderPackageList: *data.OrderPackageList, Total: *data.Total, Page: *data.Page, PageSize: *data.PageSize}
	return result, nil
}
func (c *Client) GetPackage(ctx context.Context, packageID int64) (json.RawMessage, error) {
	if packageID <= 0 {
		return nil, errors.New("miaoshou: package ID must be positive")
	}
	raw, err := c.post(ctx, detailPath, struct {
		ID int64 `json:"opOrderPackageId"`
	}{packageID})
	if err != nil {
		return nil, err
	}
	var data struct {
		Info json.RawMessage `json:"orderPackageInfo"`
	}
	if json.Unmarshal(raw, &data) != nil || len(data.Info) == 0 || data.Info[0] != '{' {
		return nil, errors.New("miaoshou: invalid package detail response")
	}
	return data.Info, nil
}
func (c *Client) post(ctx context.Context, path string, payload any) (json.RawMessage, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.New("miaoshou: request encoding failed")
	}
	requestBody := append(json.RawMessage(nil), body...)
	request, err := c.signedRequest(ctx, path, body)
	if err != nil {
		return nil, err
	}
	response, err := c.do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("miaoshou: transport failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, errors.New("miaoshou: HTTP status " + strconv.Itoa(response.StatusCode))
	}
	const maxBody = 32 << 20
	body, err = io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		return nil, errors.New("miaoshou: response could not be read")
	}
	var envelope struct {
		Result string          `json:"result"`
		Code   string          `json:"code"`
		Data   json.RawMessage `json:"data"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return nil, errors.New("miaoshou: invalid response JSON")
	}
	if envelope.Result != "success" || (envelope.Code != "" && envelope.Code != "success") {
		return nil, errors.New("miaoshou: API did not report success")
	}
	if len(envelope.Data) == 0 || envelope.Data[0] != '{' {
		return nil, errors.New("miaoshou: missing response data")
	}
	if c.observeResponse != nil {
		if err := c.observeResponse(path, append(json.RawMessage(nil), requestBody...), append(json.RawMessage(nil), body...)); err != nil {
			return nil, err
		}
	}
	return envelope.Data, nil
}

// signedRequest signs the exact bytes supplied by both read and write operations.
func (c *Client) signedRequest(ctx context.Context, path string, body []byte) (*http.Request, error) {
	timestamp := strconv.FormatInt(c.now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(c.credentials.AppSecret))
	// Sign precisely the JSON bytes transmitted below, without re-encoding.
	mac.Write([]byte(c.credentials.AppSecret + path + timestamp + c.credentials.AppKey))
	mac.Write(body)
	mac.Write([]byte(c.credentials.AppSecret))
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("miaoshou: request creation failed")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-app-key", c.credentials.AppKey)
	request.Header.Set("x-timestamp", timestamp)
	request.Header.Set("x-sign", hex.EncodeToString(mac.Sum(nil)))
	if c.credentials.SubAccountID != "" {
		request.Header.Set("x-sub-account-id", c.credentials.SubAccountID)
	}
	return request, nil
}

// Read operations retry transient gateway/rate-limit failures with a bounded
// delay. Write operations call http.Do directly and are never retried here.
func (c *Client) do(request *http.Request) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		response, err := c.http.Do(request)
		if err != nil {
			return nil, err
		}
		status := response.StatusCode
		if attempt >= 2 || (status != 429 && status != 502 && status != 503 && status != 504) {
			return response, nil
		}
		delay := time.Duration(attempt+1) * time.Second
		if seconds, err := strconv.Atoi(response.Header.Get("Retry-After")); err == nil && seconds > 0 {
			delay = time.Duration(min(seconds, 30)) * time.Second
		}
		response.Body.Close()
		timer := time.NewTimer(delay)
		select {
		case <-request.Context().Done():
			timer.Stop()
			return nil, request.Context().Err()
		case <-timer.C:
		}
		clone := request.Clone(request.Context())
		clone.Body, err = request.GetBody()
		if err != nil {
			return nil, err
		}
		request = clone
	}
}
