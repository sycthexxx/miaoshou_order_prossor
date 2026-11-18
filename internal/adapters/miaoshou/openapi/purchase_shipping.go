package miaoshouopenapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const UpdatePurchaseShippingPath = "/open/v1/ewm/goods_purchase_order/goods_purchase_order/operate/update_goods_purchase_order_outer_shipping"

// GoodsPurchaseOrderID is the Miaoshou warehouse purchase order ID, not a
// package ID or a supplier's order number. Optional fields stay omitted.
type UpdatePurchaseShippingRequest struct {
	GoodsPurchaseOrderID string             `json:"goodsPurchaseOrderId"`
	PurchaseOrderInfo    *PurchaseOrderInfo `json:"purchaseOrderInfo,omitempty"`
	ShippingID           string             `json:"shippingId,omitempty"`
}

type PurchaseOrderInfo struct {
	Platform string            `json:"purchasePlatform,omitempty"`
	Number   string            `json:"purchaseOrderSn,omitempty"`
	Buyer    string            `json:"purchaseOrderBuyer,omitempty"`
	Seller   string            `json:"purchaseOrderSeller,omitempty"`
	Packages []PurchaseWaybill `json:"purchaseOrderPackages,omitempty"`
}

type PurchaseWaybill struct {
	Company string `json:"purchaseOrderLogisticsName"`
	Number  string `json:"purchaseOrderWaybillCode"`
}

func (r UpdatePurchaseShippingRequest) Validate() error {
	if strings.TrimSpace(r.GoodsPurchaseOrderID) == "" {
		return errors.New("请填写 goodsPurchaseOrderId（妙手采购单 ID）")
	}
	return nil
}

// ShippingUpdateResponse keeps the complete body, including API errors and
// non-JSON HTTP failures, so the standalone diagnostic can save the evidence.
type ShippingUpdateResponse struct {
	Status int
	Body   []byte
}

func (c *Client) UpdatePurchaseShipping(ctx context.Context, input UpdatePurchaseShippingRequest) (ShippingUpdateResponse, error) {
	var out ShippingUpdateResponse
	if err := input.Validate(); err != nil {
		return out, err
	}
	body, err := json.Marshal(input)
	if err != nil {
		return out, err
	}
	request, err := c.signedRequest(ctx, UpdatePurchaseShippingPath, body)
	if err != nil {
		return out, err
	}
	// Do not retry writes: a failed response may follow a successful mutation.
	response, err := c.http.Do(request)
	if err != nil {
		return out, errors.New("提交未收到有效响应，结果未知；请先核对远端采购单，不要直接重试")
	}
	defer response.Body.Close()
	out.Status = response.StatusCode
	out.Body, err = io.ReadAll(response.Body)
	if err != nil {
		return out, errors.New("响应未读取完整，提交结果未知；请先核对远端采购单")
	}
	if out.Status < 200 || out.Status >= 300 {
		return out, fmt.Errorf("HTTP %d，已保留响应，请核对远端结果后再操作", out.Status)
	}
	var envelope struct {
		Result string `json:"result"`
		Code   string `json:"code"`
	}
	if json.Unmarshal(out.Body, &envelope) != nil {
		return out, errors.New("响应不是有效 JSON，提交结果未知")
	}
	// This endpoint's documented success response has no data member.
	if envelope.Result != "success" || (envelope.Code != "" && envelope.Code != "success") {
		return out, errors.New("接口未明确返回成功，请查看完整响应并核对远端采购单")
	}
	return out, nil
}
