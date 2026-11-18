package miaoshouopenapi

import (
	"context"
	"fmt"
	"order_processor/internal/domain/orders"
)

// Source adapts the vendor API to the application's channel-neutral order port.
type Source struct{ Client *Client }

func (s Source) Fetch(ctx context.Context, progress func(string)) (orders.Batch, error) {
	snapshot, err := s.Client.FetchPending(ctx, progress)
	if err != nil {
		return orders.Batch{}, err
	}
	return PendingBatch(snapshot)
}

// PendingBatch translates provider fields and relation semantics at the edge.
func PendingBatch(snapshot PendingSnapshot) (orders.Batch, error) {
	groups, err := Classify(snapshot)
	if err != nil {
		return orders.Batch{}, err
	}
	batch := orders.Batch{Source: "openapi", Policy: orders.ImportPolicy{PreserveMissingFields: true, RequireUnchangedAccount: true, RequireWholeOrderPurchase: true}}
	scopes := map[string][]string{"waitPurchase": {}, "hasPurchase": {}}
	for _, raw := range snapshot.Packages {
		p, err := decodeObject(raw)
		if err != nil {
			return batch, err
		}
		id := value(p["opOrderPackageId"])
		if len(orders.Objects(p["giftItems"])) > 0 {
			return batch, fmt.Errorf("包裹含独立赠品明细，请使用网页采集")
		}
		items, ok := p["items"].([]any)
		if !ok {
			return batch, fmt.Errorf("官方包裹缺少商品明细")
		}
		for _, v := range items {
			item, ok := v.(map[string]any)
			if !ok {
				return batch, fmt.Errorf("官方商品明细格式错误")
			}
			if gift, ok := item["isGift"]; ok {
				item["isTiktokGift"] = "0"
				if value(gift) == "true" || value(gift) == "1" {
					item["isTiktokGift"] = "1"
				}
			}
		}
		info := p["orderInfo"].(map[string]any)
		p["platformOrderSn"] = info["platformOrderSn"]
		if note, exists := info["appNote"]; exists {
			p["appNote"] = note
		}
		p["appPurchaseStatus"] = "0"
		if groups[id] == "hasPurchase" {
			p["appPurchaseStatus"] = "2"
		}
		p["purchaseGroupRule"] = "normal-purchase-order-relation-v1"
		batch.Records = append(batch.Records, orders.Record{ID: id, Fields: p})
		scopes[groups[id]] = append(scopes[groups[id]], id)
	}
	for _, name := range []string{"waitPurchase", "hasPurchase"} {
		batch.Scopes = append(batch.Scopes, orders.Scope{Name: name, PackageIDs: scopes[name]})
	}
	return batch, nil
}
