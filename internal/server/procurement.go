package server

import (
	"database/sql"
	"net/http"
	"order_processor/internal/store"
	"time"
)

func procurementRoutes(mux *http.ServeMux, db *sql.DB) *procurementCache {
	mux.HandleFunc("GET /api/accounts/{id}/shipment-stats", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.Exists(db, id) {
			send(w, 404, map[string]string{"error": "账户不存在"})
			return
		}
		q := r.URL.Query()
		start, end := q.Get("start"), q.Get("end")
		_, startErr := time.Parse("2006-01-02", start)
		_, endErr := time.Parse("2006-01-02", end)
		if (start != "" || end != "") && (startErr != nil || endErr != nil || start > end) {
			send(w, 400, map[string]string{"error": "请选择有效的日期范围和统计范围"})
			return
		}
		stats, err := store.ShipmentStats(db, id, start, end)
		if err != nil {
			send(w, 500, map[string]string{"error": "销售数据读取失败"})
			return
		}
		send(w, 200, stats)
	})
	procurementMappingRoutes(mux, db)
	cache := &procurementCache{load: func(account string, gifts bool) (map[string]any, error) { return store.Procurement(db, account, gifts) }}
	mux.HandleFunc("GET /api/accounts/{id}/procurement", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.Exists(db, id) {
			send(w, 404, map[string]string{"error": "账户不存在"})
			return
		}
		d, e := cache.get(id, r.URL.Query().Get("gifts") == "1", r.URL.Query().Get("refresh") == "1")
		if e != nil {
			send(w, 500, map[string]string{"error": "采购汇总读取失败"})
			return
		}
		send(w, 200, d)
	})
	mux.HandleFunc("GET /api/accounts/{id}/procurement/offers/{offerId}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.Exists(db, id) {
			send(w, 404, map[string]string{"error": "账户不存在"})
			return
		}
		offer := r.PathValue("offerId")
		if !offerNumber.MatchString(offer) {
			send(w, 400, map[string]string{"error": "1688 商品编号无效"})
			return
		}
		data, err := cache.get(id, r.URL.Query().Get("gifts") == "1", false)
		if err != nil {
			send(w, 500, map[string]string{"error": "采购汇总读取失败"})
			return
		}
		snapshot, found := summaryOfferSnapshot(data, id, offer)
		if !found {
			send(w, 200, map[string]any{"found": false, "createdAt": data["createdAt"]})
			return
		}
		send(w, 200, map[string]any{"found": true, "snapshot": snapshot})
	})
	return cache
}
