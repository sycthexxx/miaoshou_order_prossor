package miaoshouopenapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

const purchasePath = "/open/v1/order/purchase_order/purchase_record/search_list"

type PurchaseResponse struct {
	List  []json.RawMessage
	Total int
}

func (c *Client) ListPurchases(ctx context.Context, page, size int) (PurchaseResponse, error) {
	var result PurchaseResponse
	if page < 1 || size < 10 || size > 500 {
		return result, fmt.Errorf("miaoshou: invalid purchase pagination")
	}
	raw, err := c.post(ctx, purchasePath, struct {
		Page     int `json:"page"`
		PageSize int `json:"pageSize"`
	}{page, size})
	if err != nil {
		return result, err
	}
	var data struct {
		List  *[]json.RawMessage `json:"list"`
		Total json.RawMessage    `json:"total"`
	}
	if json.Unmarshal(raw, &data) != nil || data.List == nil {
		return result, fmt.Errorf("miaoshou: invalid purchase list")
	}
	n, err := strconv.Atoi(string(bytes.Trim(data.Total, `"`)))
	if err != nil || n < 0 {
		return result, fmt.Errorf("miaoshou: invalid purchase total")
	}
	return PurchaseResponse{*data.List, n}, nil
}

type PendingSnapshot struct {
	Packages  []json.RawMessage `json:"packages"`
	Purchases []json.RawMessage `json:"purchases"`
}

// FetchPending validates a complete pagination pass. A second full pass checks
// package membership and purchase links, including changes outside page one.
func (c *Client) FetchPending(ctx context.Context, progress func(string)) (PendingSnapshot, error) {
	var result PendingSnapshot
	var previous string
	for pass := 1; pass <= 2; pass++ {
		result = PendingSnapshot{}
		for page, total := 1, -1; ; page++ {
			if err := pause(ctx); err != nil {
				return result, err
			}
			progress(fmt.Sprintf("核对第 %d 轮：待处理包裹第 %d 页", pass, page))
			r, err := c.ListPackages(ctx, ListRequest{Page: page, PageSize: 200, AppPackageTab: "waitProcess"})
			if err != nil {
				return result, err
			}
			if total < 0 {
				total = int(r.Total)
			}
			expected := min(200, total-(page-1)*200)
			if r.Page != page || r.PageSize != 200 || int(r.Total) != total || expected < 0 || len(r.OrderPackageList) != expected {
				return result, fmt.Errorf("订单分页发生变化，请重新同步")
			}
			result.Packages = append(result.Packages, r.OrderPackageList...)
			if len(result.Packages) == total {
				break
			}
		}
		for page, total := 1, -1; ; page++ {
			if err := pause(ctx); err != nil {
				return result, err
			}
			progress(fmt.Sprintf("核对第 %d 轮：采购记录第 %d 页", pass, page))
			r, err := c.ListPurchases(ctx, page, 500)
			if err != nil {
				return result, err
			}
			if total < 0 {
				total = r.Total
			}
			expected := min(500, total-(page-1)*500)
			if r.Total != total || expected < 0 || len(r.List) != expected {
				return result, fmt.Errorf("采购记录分页发生变化，请重新同步")
			}
			result.Purchases = append(result.Purchases, r.List...)
			if len(result.Purchases) == total {
				break
			}
		}
		groups, err := Classify(result)
		if err != nil {
			return result, err
		}
		// Compare business fields; image delivery URLs and general modification
		// timestamps can change without affecting procurement or membership.
		canonical := map[string]json.RawMessage{}
		for _, raw := range result.Packages {
			p, _ := decodeObject(raw)
			canonical[value(p["opOrderPackageId"])] = businessPackage(p)
		}
		signature, _ := json.Marshal(struct {
			Packages map[string]json.RawMessage
			Groups   map[string]string
		}{canonical, groups})
		if pass == 2 && previous != string(signature) {
			return result, fmt.Errorf("同步期间订单或采购关联发生变化，未更新本地数据，请重试")
		}
		previous = string(signature)
	}
	return result, nil
}
func pause(ctx context.Context) error {
	timer := time.NewTimer(600 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func decodeObject(raw json.RawMessage) (map[string]any, error) {
	var p map[string]any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	err := d.Decode(&p)
	if err == nil && p == nil {
		err = fmt.Errorf("missing object")
	}
	return p, err
}
func value(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
func orderKey(p map[string]any) string {
	platform, number := value(p["platform"]), value(p["platformOrderSn"])
	if platform == "" || number == "" {
		return ""
	}
	b, _ := json.Marshal([]string{platform, number})
	return string(b)
}

// Classify uses order relations, never supplier shipping status. Ambiguous
// split orders and manual stock-out are rejected until verified against web data.
func Classify(snapshot PendingSnapshot) (map[string]string, error) {
	linked := map[string]bool{}
	purchaseIDs := map[string]bool{}
	for _, raw := range snapshot.Purchases {
		p, err := decodeObject(raw)
		if err != nil {
			return nil, fmt.Errorf("采购记录格式错误")
		}
		number, source := value(p["purchaseOrderSn"]), value(p["purchasePlatformName"])
		identity, _ := json.Marshal([]string{source, number})
		if number == "" || purchaseIDs[string(identity)] {
			return nil, fmt.Errorf("采购记录标识缺失或分页重复")
		}
		purchaseIDs[string(identity)] = true
		relations, ok := p["opOrderPackageList"].([]any)
		if !ok {
			return nil, fmt.Errorf("采购记录缺少订单关联列表")
		}
		for _, v := range relations {
			r, ok := v.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("采购关联格式错误")
			}
			if value(r["relateStatus"]) == "normal" {
				key := orderKey(r)
				if key == "" {
					return nil, fmt.Errorf("有效采购关联缺少订单标识")
				}
				linked[key] = true
			} else if value(r["relateStatus"]) != "cancel" {
				return nil, fmt.Errorf("采购关联状态无法识别，请使用网页采集")
			}
		}
	}
	out := map[string]string{}
	keys := map[string]bool{}
	for _, raw := range snapshot.Packages {
		p, err := decodeObject(raw)
		if err != nil {
			return nil, fmt.Errorf("包裹格式错误")
		}
		id := value(p["opOrderPackageId"])
		if id == "" || out[id] != "" {
			return nil, fmt.Errorf("包裹标识缺失或分页重复")
		}
		info, ok := p["orderInfo"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("包裹缺少订单信息")
		}
		key := orderKey(map[string]any{"platform": p["platform"], "platformOrderSn": info["platformOrderSn"]})
		if key == "" || keys[key] {
			return nil, fmt.Errorf("包裹 %s 的订单关联不唯一，请使用网页采集", id)
		}
		keys[key] = true
		if v := value(info["isManualStockout"]); v == "true" || v == "1" {
			return nil, fmt.Errorf("包裹 %s 为人工出库，请使用网页采集", id)
		}
		out[id] = "waitPurchase"
		if linked[key] {
			out[id] = "hasPurchase"
		}
	}
	return out, nil
}

func businessPackage(p map[string]any) json.RawMessage {
	out := map[string]any{}
	for _, key := range []string{"opOrderPackageId", "platform", "shopId", "appPackageStatus", "platformPackageStatus", "giftItems"} {
		out[key] = p[key]
	}
	info, _ := p["orderInfo"].(map[string]any)
	order := map[string]any{}
	for _, key := range []string{"platformOrderSn", "appOrderStatus", "platformOrderStatus", "gmtRefund", "isManualStockout"} {
		order[key] = info[key]
	}
	out["orderInfo"] = order
	items := map[string]any{}
	rows, _ := p["items"].([]any)
	for _, v := range rows {
		row, ok := v.(map[string]any)
		if !ok {
			continue
		}
		i := map[string]any{}
		for _, key := range []string{"opOrderItemId", "opOrderPackageItemId", "platformItemId", "platformSkuId", "skuSubName", "quantity", "isGift", "refundQuantity", "isRefund"} {
			i[key] = row[key]
		}
		items[value(row["opOrderPackageItemId"])+":"+value(row["opOrderItemId"])] = i
	}
	out["items"] = items
	raw, _ := json.Marshal(out)
	return raw
}
