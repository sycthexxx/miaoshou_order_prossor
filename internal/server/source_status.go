package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"order_processor/internal/store"
)

func sourceStatusRoutes(mux *http.ServeMux, db *sql.DB, cache *procurementCache) {
	failure := func(w http.ResponseWriter, err error) {
		if errors.Is(err, store.ErrSourceStatusInput) || errors.Is(err, store.ErrProcurementMappingInput) || errors.Is(err, store.ErrProcurementMappingSource) {
			send(w, 400, map[string]string{"error": err.Error()})
		} else {
			send(w, 500, map[string]string{"error": "货源状态保存或读取失败"})
		}
	}
	mux.HandleFunc("GET /api/accounts/{id}/source-status/{offerId}", func(w http.ResponseWriter, r *http.Request) {
		if !store.Exists(db, r.PathValue("id")) {
			send(w, 404, map[string]string{"error": "账户不存在"})
			return
		}
		status, err := store.GetSourceStatus(db, r.PathValue("id"), r.PathValue("offerId"))
		if err != nil {
			failure(w, err)
			return
		}
		send(w, 200, map[string]any{"sourceStatus": status})
	})
	mux.HandleFunc("PUT /api/accounts/{id}/source-status/{offerId}", func(w http.ResponseWriter, r *http.Request) {
		account := r.PathValue("id")
		if !store.Exists(db, account) {
			send(w, 404, map[string]string{"error": "账户不存在"})
			return
		}
		var in store.SourceStatusInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&in) != nil || decoder.Decode(&struct{}{}) != io.EOF {
			send(w, 400, map[string]string{"error": "货源状态格式不正确"})
			return
		}
		result, err := store.SetSourceStatus(db, account, r.PathValue("offerId"), in)
		if err != nil {
			failure(w, err)
			return
		}
		response := map[string]any{"sourceStatus": result.Status, "changed": result.Changed, "retainedManual": result.RetainedManual}
		if result.Changed {
			response["cacheRefreshed"] = true
			if err := cache.refreshAccount(account); err != nil {
				response["cacheRefreshed"] = false
				response["cacheRefreshError"] = "货源状态已保存，但采购汇总更新失败，请手动刷新采购汇总"
			}
		}
		send(w, 200, response)
	})
}
