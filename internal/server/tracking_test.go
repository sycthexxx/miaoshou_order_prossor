package server

import (
	"testing"

	"order_processor/internal/domain/orders"
)

func TestTrackingUsesOnlyWaybillsLinkedToThePackage(t *testing.T) {
	p := orders.Fields{"platform": "tiktok", "logisticsNo": "DCS123"}
	purchases := `[{"number":"5123","shipments":[{"number":"5123","traceId":"44"},{"number":"YT998","company":"圆通","traceId":"55"}]}]`
	for _, number := range []string{"", "5123", "unrelated"} {
		if data := trackingPayload("domestic", number, p, purchases); data != nil {
			t.Fatalf("invalid domestic waybill %q accepted", number)
		}
	}
	if data := trackingPayload("domestic", "YT998", p, purchases); data["traceId"] != "55" || data["company"] != "圆通" {
		t.Fatalf("domestic payload: %v", data)
	}
	if data := trackingPayload("platform", "unrelated", p, purchases); data["number"] != "DCS123" {
		t.Fatalf("platform payload: %v", data)
	}
	p["platform"] = "shopee"
	if trackingPayload("platform", "", p, purchases) != nil {
		t.Fatal("non-TikTok platform waybill accepted")
	}
}
