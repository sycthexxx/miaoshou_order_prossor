package web

import (
	"context"
	"encoding/json"
	"fmt"
)

type PurchaseAccounts struct {
	OrderNumber string `json:"orderNumber"`
	Buyer       string `json:"buyer"`
	Seller      string `json:"seller"`
	Archive     string `json:"archive,omitempty"`
}

func (s PackingSubmitter) LookupPurchase(ctx context.Context, account, number string) (PurchaseAccounts, error) {
	raw, err := s.request(ctx, account, "purchase-lookup", map[string]string{"orderNumber": number})
	if err != nil {
		return PurchaseAccounts{}, err
	}
	return purchaseAccounts(raw, number)
}
func purchaseAccounts(raw browserResponse, number string) (PurchaseAccounts, error) {
	var data struct {
		Result string `json:"result"`
		Info   struct {
			Number string `json:"purchaseOrderSn"`
			Buyer  string `json:"purchaseOrderBuyer"`
			Seller string `json:"purchaseOrderSeller"`
		} `json:"purchaseOrderInfo"`
	}
	if raw.Error != "" {
		return PurchaseAccounts{}, fmt.Errorf("%s", raw.Error)
	}
	if raw.Status != 200 || json.Unmarshal([]byte(raw.Body), &data) != nil || data.Result != "success" || data.Info.Number != number || data.Info.Buyer == "" || data.Info.Seller == "" {
		return PurchaseAccounts{}, fmt.Errorf("未查到该采购单的完整账号信息，请确认采购单号及妙手 1688 授权；也可手动填写")
	}
	return PurchaseAccounts{OrderNumber: data.Info.Number, Buyer: data.Info.Buyer, Seller: data.Info.Seller, Archive: raw.Archive}, nil
}
