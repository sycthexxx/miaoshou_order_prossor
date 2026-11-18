package server

import (
	"net/http"
	"order_processor/internal/adapters/miaoshou/web"
	"order_processor/internal/domain/orders"
	"order_processor/internal/store"
)

func (g *browserGateway) amountDetail(w http.ResponseWriter, r *http.Request) {
	account, packageID := r.PathValue("id"), r.PathValue("package")
	var payload string
	if err := g.db.QueryRowContext(r.Context(), "SELECT payload FROM packages WHERE account_id=? AND package_id=?", account, packageID).Scan(&payload); err != nil {
		send(w, 404, map[string]string{"error": "当前账户中没有该包裹，请先同步订单"})
		return
	}
	var p orders.Fields
	if orders.Decode([]byte(payload), &p) != nil {
		send(w, 422, map[string]string{"error": "无法读取包裹订单信息"})
		return
	}
	orderID := orders.Text(p["opOrderId"])
	if orderID == "" {
		info, _ := p["orderInfo"].(map[string]any)
		orderID = orders.Text(info["opOrderId"])
	}
	if orderID == "" {
		send(w, 422, map[string]string{"error": "包裹缺少妙手订单 ID，请先同步订单"})
		return
	}
	gate := g.gate(account)
	if !gate.TryLock() {
		send(w, 409, map[string]string{"error": "账户浏览器正在忙，请稍后重试"})
		return
	}
	defer gate.Unlock()
	adapter := web.PackingSubmitter{Runtime: g.runtime, Client: g.client, StorageKey: func(id string) string { return store.StorageKey(g.db, id) }}
	data, err := adapter.AmountDetail(r.Context(), account, packageID, orderID)
	if err != nil {
		send(w, 422, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	send(w, 200, data)
}
