package web

import "testing"

func TestPurchaseAccountsRequiresMatchingOrder(t *testing.T) {
	const number = "5127929209710117142"
	const body = `{"result":"success","purchaseOrderInfo":{"purchaseOrderSn":"5127929209710117142","purchaseOrderBuyer":"buyer","purchaseOrderSeller":"seller"}}`
	info, err := purchaseAccounts(browserResponse{Status: 200, Body: body, Archive: "raw"}, number)
	if err != nil || info.OrderNumber != number || info.Buyer != "buyer" || info.Seller != "seller" || info.Archive != "raw" {
		t.Fatalf("%+v %v", info, err)
	}
	for _, raw := range []browserResponse{{Status: 200, Body: body}, {Status: 200, Body: `{"result":"success","purchaseOrderInfo":{}}`}, {Status: 401, Body: body}, {Status: 200, Body: `{"result":"fail"}`}} {
		if _, err := purchaseAccounts(raw, "5127867576719035123"); err == nil {
			t.Fatalf("accepted mismatched/missing accounts: %+v", raw)
		}
	}
}
