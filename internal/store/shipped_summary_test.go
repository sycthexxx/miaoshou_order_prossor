package store

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestShippedSummaryGroupsPurchaseAndWaybillAndFlagsMissing(t *testing.T) {
	makePackage := func(id string, items []object, purchases ...firstMilePurchase) object {
		raw, err := json.Marshal(purchases)
		if err != nil {
			t.Fatal(err)
		}
		return object{"opOrderPackageId": id, "platformOrderSn": "order-" + id, "items": items,
			"firstMilePurchases": json.RawMessage(raw)}
	}
	sku := object{"opOrderItemId": "sku-a", "picUrlThumb": "https://example.com/a.png", "quantity": 2,
		"sourceList": []object{{"source": "1688", "sourceItemId": "100"}}}
	other := object{"opOrderItemId": "sku-b", "quantity": 1,
		"sourceList": []object{{"source": "1688", "sourceItemId": "200"}}}
	p1 := makePackage("1", []object{sku, other},
		firstMilePurchase{ID: "10", Number: "168801", Platform: "1688", ItemIDs: []string{"sku-a"}},
		firstMilePurchase{ID: "11", Number: "168802", Platform: "1688", ItemIDs: []string{"sku-b"},
			Shipments: []firstMileShipment{{Number: "168802"}}})
	p2 := makePackage("2", []object{sku}, firstMilePurchase{ID: "20", Number: "168801", Platform: "1688",
		Shipments: []firstMileShipment{{Number: "YT100"}}})
	p3 := makePackage("3", []object{sku}, firstMilePurchase{ID: "30", Number: "internal-miaoshou",
		Shipments: []firstMileShipment{{Number: "YT100", TraceID: "trace-30"}}})
	factories, err := groupShippedOrders([]object{p1, p2, p3}, map[[2]string]string{
		{"1", "10"}: "历史卖家", {"1", "11"}: "厂家乙", {"2", "20"}: "历史卖家", {"3", "30"}: "历史卖家",
	}, map[string]object{`source:["1688","","100"]`: {"name": "厂家甲"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(factories) != 2 || factories[0].Name != "厂家乙" || factories[0].Missing != 1 ||
		len(factories[0].Groups[0].Shipments) != 0 {
		t.Fatalf("missing-waybill factory was not prioritized: %+v", factories)
	}
	a := factories[1]
	if a.Name != "厂家甲" || a.Missing != 0 || len(a.Groups) != 1 || len(a.Groups[0].Orders) != 3 ||
		!reflect.DeepEqual(a.Groups[0].Numbers, []string{"168801"}) {
		t.Fatalf("shared purchase and waybill were not grouped: %+v", a)
	}
	if !reflect.DeepEqual(a.Groups[0].OfferIDs, []string{"100"}) ||
		!reflect.DeepEqual(factories[0].Groups[0].OfferIDs, []string{"200"}) {
		t.Fatal("expected only the distinct sources for each group's linked SKUs")
	}
	if shipment := a.Groups[0].Shipments[0]; shipment.PackageID != "3" || shipment.TraceID != "trace-30" {
		t.Fatalf("wrong package selected for shared waybill: %+v", shipment)
	}
	items := a.Groups[0].Orders[0].Items
	if len(items) != 1 || items[0]["picUrlThumb"] != sku["picUrlThumb"] || items[0]["opOrderItemId"] != "sku-a" {
		t.Fatalf("purchase includes unrelated SKU: %v", items)
	}
}
