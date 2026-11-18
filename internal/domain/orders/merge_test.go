package orders

import "testing"

func TestMergeNeverCopiesRefundAcrossDifferentPackageItem(t *testing.T) {
	old := Fields{"items": []Fields{{"opOrderItemId": "same", "opOrderPackageItemId": "old", "refundQuantity": 2}}}
	var p Fields
	Decode([]byte(`{"orderInfo":{},"items":[{"opOrderItemId":"same","opOrderPackageItemId":"new","quantity":2}]}`), &p)
	_, missing, supplemented, err := Merge(old, p, "old", ImportPolicy{PreserveMissingFields: true})
	if err != nil || missing != 1 || supplemented != 0 {
		t.Fatal(missing, supplemented, err)
	}
}

func TestChangedRefundTimeNeedsFreshWebData(t *testing.T) {
	old := Fields{"orderInfo": map[string]any{"gmtRefund": "old"}, "items": []Fields{{"opOrderItemId": "same", "refundQuantity": 0}}}
	var p Fields
	Decode([]byte(`{"orderInfo":{},"items":[{"opOrderItemId":"same","quantity":2}]}`), &p)
	p["orderInfo"].(map[string]any)["gmtRefund"] = "new"
	_, missing, supplemented, err := Merge(old, p, "old", ImportPolicy{PreserveMissingFields: true})
	if err != nil || missing != 1 || supplemented != 0 {
		t.Fatal(missing, supplemented, err)
	}
}
