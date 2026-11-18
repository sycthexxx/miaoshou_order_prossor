package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"order_processor/internal/adapters/miaoshou/web/browser"
	"order_processor/internal/domain/packing"
)

type PackingSubmitter struct {
	Runtime    browser.Runtime
	Client     *http.Client
	StorageKey func(string) string
}

// PackingPayload mirrors manualRelateOrderPackagePurchaseOrders, not the open API.
func PackingPayload(in packing.Request) map[string]any {
	items := map[string]any{}
	for _, item := range in.Items {
		v := map[string]any{"forwarderId": "0", "forwarderInfo": map[string]string{"purchaseUrl": ""}, "appNote": item.Note, "itemPurchaseAmount": item.Amount, "isAutoRsyncPurchasePayment": "0", "forwarderNote": "", "isAlibbOfficialService": "0"}
		if len(item.Waybills) > 0 {
			waybills := []any{}
			for _, w := range item.Waybills {
				waybills = append(waybills, map[string]string{"purchaseOrderLogisticsName": w.Company, "purchaseOrderWaybillCode": w.Number})
			}
			v["purchaseOrderPackages"] = waybills
		}
		if p := item.Purchase; p != nil {
			if item.AutoSyncAmount {
				v["isAutoRsyncPurchasePayment"] = "1"
			}
			for k, value := range map[string]string{"purchasePlatform": p.Platform, "purchaseOrderSn": p.OrderNumber, "purchaseOrderBuyer": p.Buyer, "purchaseOrderSeller": p.Seller, "purchaseOrderPayment": p.Payment, "purchaseOrderFullname": p.Recipient, "purchaseOrderMobile": p.Mobile, "purchaseOrderFullAddress": p.Address, "purchaseOrderStartTime": p.CreatedAt, "purchaseOrderRefundMoney": p.Refund, "purchaseOrderStatus": p.Status, "purchaseOrderSendTime": p.ShippedAt, "logisticsIsAccept": "0", "wsyOrderDetailId": "", "wsyOrderType": "", "purchaseShopId": ""} {
				v[k] = value
			}
			v["isChanged"] = true
			items[item.OrderItemID] = []any{v}
		} else {
			v["purchasePlatform"] = "other"
			items[item.OrderItemID] = v
		}
	}
	return map[string]any{"purchaseOrderInfoList": []any{map[string]any{"opOrderPackageId": in.PackageID, "forwarderId": "0", "opOrderItemIdAndPurchaseOrderInfoMap": items}}}
}
func (s PackingSubmitter) Submit(ctx context.Context, account string, in packing.Request) (packing.Result, error) {
	raw, err := s.request(ctx, account, "packing", PackingPayload(in))
	if err != nil {
		return packing.Result{}, err
	}
	if raw.Error != "" {
		return packing.Result{State: raw.State, Message: raw.Error, Archive: raw.Archive}, nil
	}
	return packingResult(raw.Status, raw.Body, raw.Archive), nil
}

type browserResponse struct {
	Status  int    `json:"status"`
	Body    string `json:"body"`
	Archive string `json:"archive"`
	Error   string `json:"error"`
	State   string `json:"state"`
}

func (s PackingSubmitter) request(ctx context.Context, account, action string, payload any) (browserResponse, error) {
	key := s.StorageKey(account)
	target, err := s.Runtime.Resolve(ctx, key, false)
	if err != nil {
		return browserResponse{}, fmt.Errorf("请先在账户管理打开并登录妙手浏览器：%w", err)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return browserResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.API.String()+"/accounts/"+key+"/"+action, bytes.NewReader(body))
	if err != nil {
		return browserResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+target.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.Client.Do(req)
	if err != nil {
		return browserResponse{State: "unknown", Error: "未收到浏览器服务结果。请先在妙手核对，避免重复提交。"}, nil
	}
	defer resp.Body.Close()
	var raw browserResponse
	if err = json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return browserResponse{State: "unknown", Error: "浏览器返回结果无法读取，请在妙手核对后再操作。"}, nil
	}
	if raw.Error != "" {
		state := raw.State
		if state == "" {
			state = "not_sent"
			if resp.StatusCode >= 500 {
				state = "unknown"
			}
		}
		raw.State = state
		return raw, nil
	}
	return raw, nil
}
func packingResult(status int, body, archive string) packing.Result {
	r := packing.Result{State: "unknown", Message: "妙手返回内容无法确认提交结果，请打开妙手核对。", Archive: archive}
	var data struct {
		Result       string `json:"result"`
		Message      string `json:"message"`
		Msg          string `json:"msg"`
		Reason       string `json:"reason"`
		Errors       []any  `json:"errors"`
		SuccessCount *int   `json:"successCount"`
		FailCount    *int   `json:"failCount"`
	}
	if json.Unmarshal([]byte(body), &data) != nil {
		return r
	}
	if data.SuccessCount != nil {
		r.SuccessCount = *data.SuccessCount
	}
	if data.FailCount != nil {
		r.FailCount = *data.FailCount
	}
	r.Details = data.Errors
	if r.SuccessCount > 0 && (r.FailCount > 0 || len(data.Errors) > 0) {
		r.State = "partial"
		r.Message = "妙手返回部分成功，请按错误详情核对商品后再操作。"
		return r
	}
	if status >= 200 && status < 300 && data.Result == "success" && data.SuccessCount != nil && data.FailCount != nil && r.SuccessCount > 0 && r.FailCount == 0 && len(data.Errors) == 0 {
		r.State = "completed"
		r.Message = "已提交代打包。请同步待处理订单，按妙手最新状态更新分组和采购汇总。"
		return r
	}
	if r.FailCount > 0 || len(data.Errors) > 0 || (data.Result != "" && data.Result != "success") {
		r.State = "failed"
		r.Message = data.Message
		if r.Message == "" {
			r.Message = data.Msg
		}
		if r.Message == "" {
			r.Message = data.Reason
		}
		if r.Message == "" {
			r.Message = "妙手未完成提交，请查看错误详情。"
		}
	}
	return r
}
