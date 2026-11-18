package orders

import "fmt"

// Merge applies source-independent supplemental data and refund freshness rules.
func Merge(old, p Fields, oldAt string, policy ImportPolicy) (Fields, int, int, error) {
	if !policy.PreserveMissingFields {
		return p, 0, 0, nil
	}
	if policy.RequireWholeOrderPurchase && (Text(old["appPurchaseStatus"]) == "1" || Text(old["isManualStockout"]) == "true" || Text(old["isManualStockout"]) == "1") {
		return nil, 0, 0, fmt.Errorf("存在部分采购或人工出库状态，需要补充包裹级采购信息")
	}
	merged := Fields{}
	for k, v := range old {
		merged[k] = v
	}
	for k, v := range p {
		merged[k] = v
	}
	info, _ := p["orderInfo"].(map[string]any)
	merged["platformOrderSn"] = info["platformOrderSn"]
	oldInfo, _ := old["orderInfo"].(map[string]any)
	refundChanged := Text(info["gmtRefund"]) != "" && Text(info["gmtRefund"]) != Text(oldInfo["gmtRefund"])
	incoming := Objects(p["items"])
	byPackageItem := map[string]Fields{}
	byOrderItem := map[string]Fields{}
	counts := map[string]int{}
	for _, i := range Objects(old["items"]) {
		if k := Text(i["opOrderPackageItemId"]); k != "" {
			byPackageItem[k] = i
		}
		k := Text(i["opOrderItemId"])
		counts[k]++
		byOrderItem[k] = i
	}
	items := []Fields{}
	seen := map[string]bool{}
	missing, supplemented := 0, 0
	for _, i := range incoming {
		iid := Text(i["opOrderItemId"])
		if iid == "" || seen[iid] {
			return nil, 0, 0, fmt.Errorf("商品明细标识缺失或重复")
		}
		seen[iid] = true
		previous := byPackageItem[Text(i["opOrderPackageItemId"])]
		if previous == nil && counts[iid] == 1 {
			candidate := byOrderItem[iid]
			if Text(candidate["opOrderPackageItemId"]) == "" || Text(i["opOrderPackageItemId"]) == "" {
				previous = candidate
			}
		}
		item := Fields{}
		for k, v := range previous {
			item[k] = v
		}
		for k, v := range i {
			item[k] = v
		}
		if _, has := i["refundQuantity"]; has && i["refundQuantity"] != nil {
			delete(item, "refundQuantityUnknown")
			delete(item, "refundSupplementCapturedAt")
		} else if !refundChanged && previous != nil && previous["refundQuantity"] != nil && previous["refundQuantityUnknown"] != true {
			item["refundQuantity"] = previous["refundQuantity"]
			delete(item, "refundQuantityUnknown")
			item["refundSupplementCapturedAt"] = previous["refundSupplementCapturedAt"]
			if Text(item["refundSupplementCapturedAt"]) == "" {
				item["refundSupplementCapturedAt"] = oldAt
			}
			supplemented++
		} else {
			delete(item, "refundQuantity")
			item["refundQuantityUnknown"] = true
			missing++
		}
		items = append(items, item)
	}
	merged["items"] = items
	return merged, missing, supplemented, nil
}
