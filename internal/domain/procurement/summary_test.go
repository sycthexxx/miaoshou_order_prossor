package procurement

import (
	"order_processor/internal/domain/orders"
	"testing"
)

type object = orders.Fields

var text = orders.Text

func TestProcurementBySourceAndSpec(t *testing.T) {
	source := object{"source": "1688", "sourceItemId": "123", "sourceItemUrl": "https://detail.1688.com/offer/123.html"}
	makeOrder := func(id, shop, spec, qty string) object {
		return object{"opOrderPackageId": id, "appPackageNo": id, "shopId": shop, "platform": "tiktok", "appPackageStatus": "wait_seller_send", "appPackageOperateStatus": "wait_audit", "appPurchaseStatus": "0", "items": []object{{"opOrderItemId": id, "platformItemId": "product-" + shop, "platformSkuId": "sku-" + spec, "skuSubCnName": spec, "quantity": qty, "sourceList": []any{source}, "sourceState": "available"}}}
	}
	a := makeOrder("1", "a", "红色,S", "5")
	b := makeOrder("2", "b", "红色,S", "2")
	b["items"].([]object)[0]["refundQuantity"] = "1"
	c := makeOrder("3", "a", "红色,L", "10")
	paid := makeOrder("4", "a", "红色,L", "99")
	paid["appPurchaseStatus"] = "2"
	cancelled := makeOrder("5", "a", "红色,L", "99")
	cancelled["appPackageStatus"] = "cancelled"
	gift := makeOrder("6", "a", "赠品", "1")
	gift["items"].([]object)[0]["isGift"] = "1"
	noSource := makeOrder("7", "a", "黑色,M", "3")
	noSource["items"].([]object)[0]["sourceList"] = []any{}
	multi := makeOrder("8", "a", "黑色,XL", "4")
	multi["items"].([]object)[0]["sourceList"] = []any{source, object{"source": "1688", "sourceItemId": "456"}}
	orders := []object{a, b, c, paid, cancelled, gift, noSource, multi}
	d := Summarize(orders, false)
	if d["quantity"] != int64(23) || d["unresolvedQuantity"] != int64(7) || d["packages"] != 5 {
		t.Fatal(d)
	}
	groups := d["groups"].([]object)
	if len(groups) != 3 {
		t.Fatal(groups)
	}
	if groups[0]["quantity"] != int64(16) || len(groups[0]["products"].([]string)) != 2 {
		t.Fatal(groups[0])
	}
	sums := map[string]int64{}
	for _, v := range groups[0]["variants"].([]object) {
		sums[text(v["spec"])] = v["quantity"].(int64)
	}
	if sums["红色,S"] != 6 || sums["红色,L"] != 10 {
		t.Fatal(sums)
	}
	if Summarize(orders, true)["quantity"] != int64(24) {
		t.Fatal("gift inclusion")
	}
}

func TestProcurementImagesStayWithSpecification(t *testing.T) {
	source := object{"source": "1688", "sourceItemId": "123", "sourceItemUrl": "https://detail.1688.com/offer/123.html"}
	item := func(spec, image string) object {
		return object{"skuSubCnName": spec, "quantity": "1", "sourceList": []object{source}, "picUrlThumb": image, "picUrl": image}
	}
	order := object{"opOrderPackageId": "1", "appPackageStatus": "wait_seller_send", "appPackageOperateStatus": "wait_audit", "appPurchaseStatus": "0", "items": []object{item("红色,S", "https://images.example/red.jpg"), item("黑色,M", "https://images.example/black.jpg"), item("蓝色,L", "javascript:alert(1)")}}
	groups := Summarize([]object{order}, false)["groups"].([]object)
	for _, v := range groups[0]["variants"].([]object) {
		images, _ := v["images"].([]string)
		switch v["spec"] {
		case "红色,S":
			if len(images) != 1 || images[0] != "https://images.example/red.jpg" {
				t.Fatal(v)
			}
		case "黑色,M":
			if len(images) != 1 || images[0] != "https://images.example/black.jpg" {
				t.Fatal(v)
			}
		case "蓝色,L":
			if len(images) != 0 {
				t.Fatal(v)
			}
		}
	}
}

func TestMissingRefundQuantityIncludesNormalOrders(t *testing.T) {
	for _, tt := range []struct {
		name     string
		item     object
		info     object
		quantity int64
		pending  int
	}{
		{"new normal order", object{"refundQuantityUnknown": true}, object{}, 3, 0},
		{"explicit no refund", object{"refundQuantityUnknown": true, "isRefund": false}, object{"gmtRefund": nil}, 3, 0},
		{"partial refund", object{"refundQuantity": 1}, object{}, 2, 0},
		{"full refund", object{"refundQuantity": 3}, object{}, 0, 0},
		{"refund without quantity", object{"refundQuantityUnknown": true, "isRefund": true}, object{}, 0, 1},
		{"refund timestamp without quantity", object{"refundQuantityUnknown": true}, object{"gmtRefund": "2026-11-17"}, 0, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.item["quantity"] = 3
			tt.item["opOrderItemId"] = "item"
			tt.item["skuSubName"] = "红色,L"
			tt.item["sourceList"] = []object{{"source": "1688", "sourceItemId": "123"}}
			p := object{"opOrderPackageId": "1", "procurementEligible": true, "orderInfo": tt.info, "items": []object{tt.item}}
			report := Summarize([]object{p}, false)
			if report["quantity"] != tt.quantity || len(report["pendingRefundLines"].([]object)) != tt.pending {
				t.Fatal(report)
			}
			if tt.quantity > 0 {
				g := report["groups"].([]object)[0]
				if g["quantity"] != tt.quantity || len(g["variants"].([]object)) != 1 {
					t.Fatal(g)
				}
			}
		})
	}
}

func TestProcurementLocalNotes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fields object
		want   string
	}{
		{"package note", object{"appNote": "采购", "orderInfo": map[string]any{"sellerNote": "商家留言"}}, "采购"},
		{"nested note", object{"orderInfo": map[string]any{"appNote": "核对尺码"}}, "核对尺码"},
		{"cleared note", object{"appNote": nil, "orderInfo": map[string]any{"appNote": "旧备注"}}, ""},
		{"seller note is different", object{"orderInfo": map[string]any{"sellerNote": "商家留言"}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.fields
			p["procurementEligible"] = true
			p["opOrderPackageId"] = "1"
			p["items"] = []object{{"opOrderItemId": "i", "quantity": 2, "skuSubName": "L"}}
			report := Summarize([]object{p}, false)
			line := report["groups"].([]object)[0]["variants"].([]object)[0]["lines"].([]object)[0]
			if line["localNote"] != tc.want || report["quantity"] != int64(2) {
				t.Fatal(report)
			}
		})
	}
}
