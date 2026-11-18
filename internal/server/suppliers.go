package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"order_processor/internal/store"
)

func supplierRoutes(mux *http.ServeMux, db *sql.DB) {
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
		result, e := store.CaptureSupplier(db, id, in.ProductID, in.Name)
		if e != nil {
			send(w, 400, map[string]string{"error": e.Error()})
			return
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
		if e := store.AssignSupplier(db, id, in.Source, in.Site, in.Product, in.Supplier); e != nil {
			send(w, 400, map[string]string{"error": e.Error()})
			return
		}
		send(w, 200, map[string]bool{"ok": true})
	})
}
