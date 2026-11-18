package server

import (
	"net/http"
	"order_processor/internal/adapters/miaoshou/web"
	"order_processor/internal/store"
	"regexp"
)

var purchaseOrderNumber = regexp.MustCompile(`^\d{10,30}$`)

func (g *browserGateway) purchaseInfo(w http.ResponseWriter, r *http.Request) {
	id, number := r.PathValue("id"), r.URL.Query().Get("orderNumber")
	if !store.Exists(g.db, id) {
		send(w, 404, map[string]string{"error": "账户不存在"})
		return
	}
	if !purchaseOrderNumber.MatchString(number) {
		send(w, 400, map[string]string{"error": "请输入有效的 1688 采购单号"})
		return
	}
	gate := g.gate(id)
	if !gate.TryLock() {
		send(w, 409, map[string]string{"error": "账户浏览器正在忙，请稍后重新查询"})
		return
	}
	defer gate.Unlock()
	adapter := web.PackingSubmitter{Runtime: g.runtime, Client: g.client, StorageKey: func(account string) string { return store.StorageKey(g.db, account) }}
	info, err := adapter.LookupPurchase(r.Context(), id, number)
	if err != nil {
		send(w, 422, map[string]string{"error": err.Error()})
		return
	}
	send(w, 200, info)
}
