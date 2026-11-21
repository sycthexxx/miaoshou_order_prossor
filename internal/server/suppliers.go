package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"order_processor/internal/store"
)

func supplierRoutes(mux *http.ServeMux, db *sql.DB, caches ...*procurementCache) {
	mux.HandleFunc("POST /api/accounts/{id}/source-supplier/capture", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.Exists(db, id) {
			send(w, 404, map[string]string{"error": "账户不存在"})
			return
		}
		var in struct {
			ProductID string `json:"productId"`
			Name      string `json:"name"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil {
			send(w, 400, map[string]string{"error": "请求格式错误"})
			return
		}
		result, e := store.CaptureSupplier(db, currentUser(r).ID, id, in.ProductID, in.Name)
		if e != nil {
			send(w, http.StatusBadRequest, map[string]string{"error": e.Error()})
			return
		}
		if result["assigned"].(int64)+result["updated"].(int64) > 0 && len(caches) > 0 {
			result["cacheRefreshed"] = true
			if err := caches[0].refreshAccount(id); err != nil {
				// Association committed successfully; a retry would not fix a
				// failed summary query, so preserve the successful result.
				result["cacheRefreshed"] = false
				result["cacheRefreshError"] = "厂家关联已保存，但采购汇总更新失败，请手动刷新采购汇总"
			}
		}
		send(w, 200, result)
	})
	mux.HandleFunc("GET /api/accounts/{id}/suppliers", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.Exists(db, id) {
			send(w, 404, map[string]string{"error": "账户不存在"})
			return
		}
		d, e := store.Suppliers(db, id)
		if e != nil {
			send(w, 500, map[string]string{"error": "卖家读取失败"})
			return
		}
		send(w, 200, d)
	})
	mux.HandleFunc("PUT /api/accounts/{id}/source-supplier", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.Exists(db, id) {
			send(w, 404, map[string]string{"error": "账户不存在"})
			return
		}
		var in struct {
			Source   string `json:"source"`
			Site     string `json:"site"`
			Product  string `json:"productId"`
			Supplier int64  `json:"supplierId"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil {
			send(w, 400, map[string]string{"error": "请求格式错误"})
			return
		}
		if e := store.AssignSupplier(db, currentUser(r).ID, id, in.Source, in.Site, in.Product, in.Supplier); e != nil {
			status := http.StatusBadRequest
			if errors.Is(e, store.ErrSupplierBusy) {
				status = http.StatusConflict
			}
			send(w, status, map[string]string{"error": e.Error()})
			return
		}
		send(w, 200, map[string]bool{"ok": true})
	})
}
