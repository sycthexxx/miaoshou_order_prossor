package procurement

import (
	"encoding/json"
	"net/url"
	"order_processor/internal/domain/orders"
	"sort"
	"strconv"
	"strings"
)

func Summarize(packages []orders.Fields, gifts bool) orders.Fields {
	groups := map[string]orders.Fields{}
	variants := map[string]map[string]orders.Fields{}
	packageIDs := map[string]bool{}
	var total, unresolved int64
	pendingRefund := []orders.Fields{}
	for _, p := range packages {
		orderInfo, _ := p["orderInfo"].(map[string]any)
		orderedAt := orders.Text(orderInfo["gmtOrderStart"])
		paidAt := orders.Text(orderInfo["gmtPay"])
		op := orders.Text(p["appPackageOperateStatus"])
		eligible, known := p["procurementEligible"].(bool)
		if (!known && (orders.Text(p["appPackageStatus"]) != "wait_seller_send" || orders.Text(p["appPurchaseStatus"]) != "0" || (op != "wait_audit" && op != "wait_process"))) || (known && !eligible) {
			continue
		}
		for _, i := range orders.Objects(p["items"]) {
			gift := orders.Text(i["isTiktokGift"]) == "1" || orders.Text(i["isGift"]) == "1"
			if gift && !gifts {
				continue
			}
			q, e := strconv.ParseInt(orders.Text(i["quantity"]), 10, 64)
			if e != nil || q <= 0 {
				continue
			}
			if i["refundQuantityUnknown"] == true && hasRefundRecord(p, i) {
				pendingRefund = append(pendingRefund, orders.Fields{"packageNo": p["appPackageNo"], "packageId": p["opOrderPackageId"], "itemId": i["opOrderItemId"], "title": i["title"], "quantity": q, "orderedAt": orderedAt, "paidAt": paidAt})
				continue
			}
			refund, _ := strconv.ParseInt(orders.Text(i["refundQuantity"]), 10, 64)
			if refund < 0 {
				refund = 0
			}
			q -= refund
			if q <= 0 {
				continue
			}
			shop := orders.Text(i["shopId"])
			if shop == "" {
				shop = orders.Text(p["shopId"])
			}
			platform := orders.Text(i["platform"])
			if platform == "" {
				platform = orders.Text(p["platform"])
			}
			identity := []string{platform, shop, orders.Text(i["platformItemId"])}
			if identity[2] == "" {
				identity = append(identity, orders.Text(p["opOrderPackageId"]), orders.Text(i["opOrderItemId"]))
			}
			sources := orders.Objects(i["sourceList"])
			unique := map[string]orders.Fields{}
			for _, s := range sources {
				if orders.Text(s["sourceItemId"]) != "" {
					key, _ := json.Marshal([]string{orders.Text(s["source"]), orders.Text(s["sourceSite"]), orders.Text(s["sourceItemId"])})
					unique[string(key)] = s
				}
			}
			reason := ""
			key := ""
			source := orders.Fields{}
			if len(unique) == 1 {
				for k, s := range unique {
					key = "source:" + k
					source = s
				}
			} else {
				if len(unique) > 1 {
					reason = "多个候选货源，待确认"
				} else {
					switch orders.Text(i["sourceState"]) {
					case "empty":
						reason = "产品未填写货源"
					case "not_synced":
						reason = "产品尚未同步"
					default:
						reason = "货源信息不完整"
					}
				}
				b, _ := json.Marshal(identity)
				key = "unresolved:" + reason + ":" + string(b)
				unresolved += q
			}
			g, ok := groups[key]
			if !ok {
				g = orders.Fields{"key": key, "source": source, "reason": reason, "title": i["cnTitle"], "image": i, "quantity": int64(0), "products": []string{}, "variants": []orders.Fields{}}
				if orders.Text(g["title"]) == "" {
					g["title"] = i["title"]
				}
				groups[key] = g
				variants[key] = map[string]orders.Fields{}
			}
			productLabel := shop + " / " + orders.Text(i["platformItemId"])
			found := false
			for _, v := range g["products"].([]string) {
				if v == productLabel {
					found = true
				}
			}
			if !found {
				g["products"] = append(g["products"].([]string), productLabel)
			}
			spec := strings.TrimSpace(orders.Text(i["skuSubCnName"]))
			if spec == "" {
				spec = strings.TrimSpace(orders.Text(i["skuSubName"]))
			}
			specKey := spec
			if spec == "" {
				spec = "规格缺失，请核对"
				specKey = shop + "/" + orders.Text(i["platformSkuId"]) + "/" + orders.Text(i["opOrderItemId"])
			}
			v, ok := variants[key][specKey]
			if !ok {
				v = orders.Fields{"spec": spec, "quantity": int64(0), "lines": []orders.Fields{}}
				variants[key][specKey] = v
			}
			// Images belong to this specification, never to the group's first SKU.
			images, _ := v["images"].([]string)
			lineImage := orders.Fields{}
			for _, field := range []string{"picUrlThumb", "picUrl", "originalPicUrl"} {
				image := orders.Text(i[field])
				u, err := url.Parse(image)
				if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
					continue
				}
				lineImage[field] = image
				found := false
				for _, existing := range images {
					if existing == image {
						found = true
						break
					}
				}
				if !found && len(images) < 6 {
					images = append(images, image)
				}
			}
			if len(images) > 0 {
				v["images"] = images
			}
			v["quantity"] = v["quantity"].(int64) + q
			v["lines"] = append(v["lines"].([]orders.Fields), orders.Fields{
				"packageId": p["opOrderPackageId"], "itemId": i["opOrderItemId"], "packageNo": p["appPackageNo"],
				"orderNo": p["platformOrderSn"], "localNote": localNote(p),
				"orderedAt": orderedAt, "paidAt": paidAt, "paymentMethod": orders.Text(orderInfo["paymentMethod"]),
				"shopId": shop, "productId": i["platformItemId"], "skuId": i["platformSkuId"], "originalSpec": i["skuSubName"], "image": lineImage, "quantity": q, "gift": gift,
			})
			g["quantity"] = g["quantity"].(int64) + q
			total += q
			packageIDs[orders.Text(p["opOrderPackageId"])] = true
		}
	}
	result := []orders.Fields{}
	for key, g := range groups {
		for _, v := range variants[key] {
			g["variants"] = append(g["variants"].([]orders.Fields), v)
		}
		sort.Slice(g["variants"].([]orders.Fields), func(a, b int) bool {
			return orders.Text(g["variants"].([]orders.Fields)[a]["spec"]) < orders.Text(g["variants"].([]orders.Fields)[b]["spec"])
		})
		sort.Strings(g["products"].([]string))
		// Only expose navigable HTTP(S) source links.
		source := g["source"].(orders.Fields)
		u, e := url.Parse(orders.Text(source["sourceItemUrl"]))
		if e == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
			g["url"] = u.String()
		}
		result = append(result, g)
	}
	sort.Slice(result, func(a, b int) bool {
		ra, rb := orders.Text(result[a]["reason"]), orders.Text(result[b]["reason"])
		if (ra == "") != (rb == "") {
			return ra == ""
		}
		return orders.Text(result[a]["key"]) < orders.Text(result[b]["key"])
	})
	return orders.Fields{"pendingRefundLines": pendingRefund, "groups": result, "quantity": total, "unresolvedQuantity": unresolved, "packages": len(packageIDs), "sources": len(groups)}
}

// Missing API quantities alone do not indicate a refund. Keep only actual
// refund records with an unknown quantity out of the purchasing demand.
func hasRefundRecord(p, item orders.Fields) bool {
	info, _ := p["orderInfo"].(map[string]any)
	flag := orders.Text(item["isRefund"])
	return (flag != "" && flag != "0" && flag != "false") || orders.Text(info["gmtRefund"]) != ""
}

// appNote is Miaoshou's local order note, distinct from the seller's note.
func localNote(p orders.Fields) string {
	if value, exists := p["appNote"]; exists {
		return orders.Text(value)
	}
	info, _ := p["orderInfo"].(map[string]any)
	return orders.Text(info["appNote"])
}
