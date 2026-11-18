package server

import (
	"database/sql"
	"net/http"
	miaoshouopenapi "order_processor/internal/adapters/miaoshou/openapi"
	"order_processor/internal/application/ordersync"
	"order_processor/internal/store"
)

func openAPIRoutes(mux *http.ServeMux, db *sql.DB, dir string, cache *procurementCache, gateways ...*browserGateway) {
	service := ordersync.New(store.OrderRepository{DB: db}, cache.invalidate)
	var gateway *browserGateway
	if len(gateways) > 0 {
		gateway = gateways[0]
		gateway.syncingOrders = func(id string) bool { return service.Status(id).State == "running" }
	}
	exists := func(w http.ResponseWriter, r *http.Request) bool {
		if store.Exists(db, r.PathValue("id")) {
			return true
		}
		send(w, 404, map[string]string{"error": "账户不存在"})
		return false
	}
	mux.HandleFunc("GET /api/accounts/{id}/openapi-status", func(w http.ResponseWriter, r *http.Request) {
		if !exists(w, r) {
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		send(w, 200, service.Status(r.PathValue("id")))
	})
	mux.HandleFunc("POST /api/accounts/{id}/stop-openapi", func(w http.ResponseWriter, r *http.Request) {
		if !exists(w, r) {
			return
		}
		service.Stop(r.PathValue("id"))
		send(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/accounts/{id}/sync-openapi", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if gateway != nil {
			g := gateway.gate(id)
			g.Lock()
			defer g.Unlock()
		}
		if !exists(w, r) {
			return
		}
		saved, err := store.CurrentAPICredential(db, id)
		if err == sql.ErrNoRows {
			send(w, 400, map[string]string{"error": "请先在 API 管理中添加凭证并设为当前使用"})
			return
		}
		if err != nil {
			send(w, 500, map[string]string{"error": "读取当前 API 凭证失败"})
			return
		}
		credentials := miaoshouopenapi.Credentials{AppKey: saved.APIID, AppSecret: saved.Key, SubAccountID: saved.SubAccountID}
		if service.Status(id).State == "running" {
			send(w, 409, map[string]string{"error": ordersync.ErrRunning.Error()})
			return
		}
		capture, err := miaoshouopenapi.NewCapture(credentials, dir, store.StorageKey(db, id))
		if err != nil {
			send(w, 400, map[string]string{"error": err.Error()})
			return
		}
		job, err := service.Start(id, ordersync.Run{Source: capture.Source, StartedAt: capture.StartedAt, Archive: capture.Archive, SaveResult: capture.SaveResult})
		if err != nil {
			send(w, 409, map[string]string{"error": err.Error()})
			return
		}
		send(w, 202, job)
	})
}
