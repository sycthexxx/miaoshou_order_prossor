package web

import "testing"

func TestInspectPlatformLabel(t *testing.T) {
	for _, tc := range []struct{ body, state string }{
		{`{"result":"success","orderPackageDetailInfo":{"opOrderPackageId":"7","logisticsNo":"DCS123","gmtApplyLogisticsNo":"2026-11-17","isApplyTrackingNoFail":"0"}}`, "completed"},
		{`{"result":"success","orderPackageDetailInfo":{"opOrderPackageId":"7","logisticsNo":null,"gmtApplyLogisticsNo":null,"isApplyTrackingNoFail":"0"}}`, "ready"},
		{`{"result":"success","orderPackageDetailInfo":{"opOrderPackageId":"7","gmtApplyLogisticsNo":"2026-11-17","isApplyTrackingNoFail":"0"}}`, "pending"},
		{`{"result":"success","orderPackageDetailInfo":{"opOrderPackageId":"7","gmtApplyLogisticsNo":"2026-11-17","isApplyTrackingNoFail":"1"}}`, "ready"},
		{`{"result":"success","orderPackageDetailInfo":{"opOrderPackageId":"8","logisticsNo":"DCS123"}}`, "not_sent"},
		{`{"result":"success"}`, "not_sent"},
	} {
		if r := inspectLabel(browserResponse{Status: 200, Body: tc.body}, "7"); r.State != tc.state {
			t.Fatal(r, tc)
		}
	}
}
