package packing

import (
	"errors"
	"fmt"
	"strings"
)

// Request describes the business action; ERP form names belong in the adapter.
// This first workflow uses no freight forwarder.
type Request struct {
	PackageID string `json:"packageId"`
	Items     []Item `json:"items"`
}
type Item struct {
	OrderItemID    string    `json:"orderItemId"`
	Purchase       *Purchase `json:"purchase,omitempty"`
	Waybills       []Waybill `json:"waybills,omitempty"`
	Amount         string    `json:"amount"`
	AutoSyncAmount bool      `json:"autoSyncAmount"`
	Note           string    `json:"note"`
}
type Purchase struct {
	Platform    string `json:"platform"`
	OrderNumber string `json:"orderNumber"`
	Buyer       string `json:"buyer"`
	Seller      string `json:"seller"`
	Payment     string `json:"payment,omitempty"`
	Recipient   string `json:"recipient,omitempty"`
	Mobile      string `json:"mobile,omitempty"`
	Address     string `json:"address,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
	Refund      string `json:"refund,omitempty"`
	Status      string `json:"status,omitempty"`
	ShippedAt   string `json:"shippedAt,omitempty"`
}
type Waybill struct {
	Company string `json:"company"`
	Number  string `json:"number"`
}
type Result struct {
	TaskID       int64  `json:"taskId,omitempty"`
	State        string `json:"state"` // completed, partial, failed, unknown, not_sent
	Message      string `json:"message"`
	Archive      string `json:"archive,omitempty"`
	SuccessCount int    `json:"successCount"`
	FailCount    int    `json:"failCount"`
	Details      any    `json:"details,omitempty"`
}

func (r Request) Validate() error {
	if strings.TrimSpace(r.PackageID) == "" || len(r.Items) == 0 {
		return errors.New("请选择包裹和要提交的商品")
	}
	seen := map[string]bool{}
	for _, i := range r.Items {
		if strings.TrimSpace(i.OrderItemID) == "" || seen[i.OrderItemID] {
			return errors.New("商品行 ID 缺失或重复")
		}
		seen[i.OrderItemID] = true
		if i.Purchase != nil {
			p := i.Purchase
			if p.Platform != "1688" {
				return errors.New("当前采购信息录入支持 1688")
			}
			if strings.TrimSpace(p.OrderNumber) == "" || strings.TrimSpace(p.Buyer) == "" || strings.TrimSpace(p.Seller) == "" {
				return errors.New("请填写采购单号、采购账号和卖家账号")
			}
		} else if len(i.Waybills) == 0 {
			return fmt.Errorf("商品 %s 请填写采购信息或快递单号", i.OrderItemID)
		}
		for _, w := range i.Waybills {
			if strings.TrimSpace(w.Company) == "" || strings.TrimSpace(w.Number) == "" {
				return errors.New("快递单号和物流公司必须同时填写")
			}
		}
	}
	return nil
}
