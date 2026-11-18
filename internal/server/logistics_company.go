package server

import (
	"net/http"
	"order_processor/internal/adapters/miaoshou/web"
	"order_processor/internal/store"
	"strings"
)

func (g *browserGateway) logisticsCompany(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !store.Exists(g.db, id) {
		send(w, 404, map[string]string{"error": "账户不存在"})
		return
	}
	number := strings.TrimSpace(r.URL.Query().Get("logisticsNo"))
	if number == "" || len(number) > 128 {
		send(w, 400, map[string]string{"error": "请输入有效的国内快递单号"})
		return
	}
	adapter := web.PackingSubmitter{Runtime: g.runtime, Client: g.client, StorageKey: func(account string) string { return store.StorageKey(g.db, account) }}
	company, err := adapter.LogisticsCompany(r.Context(), id, number)
	if err != nil {
		send(w, 422, map[string]string{"error": err.Error()})
		return
	}
	send(w, 200, map[string]string{"company": company})
}
