package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"order_processor/internal/store"
)

func procurementMappingRoutes(mux *http.ServeMux, db *sql.DB) {
	respond := func(w http.ResponseWriter, result store.ProcurementMappingRecord, e error) {
		switch {
		case e == nil:
			send(w, http.StatusOK, result)
		case errors.Is(e, store.ErrSupplierBusy):
			send(w, http.StatusConflict, map[string]string{"error": e.Error()})
		case errors.Is(e, store.ErrProcurementMappingConflict):
			send(w, http.StatusConflict, map[string]any{"error": e.Error(), "revision": result.Revision, "mapping": result.Mapping})
		case errors.Is(e, store.ErrProcurementMappingInput), errors.Is(e, store.ErrProcurementMappingSource):
			send(w, http.StatusBadRequest, map[string]string{"error": e.Error()})
		default:
			send(w, http.StatusInternalServerError, map[string]string{"error": "采购映射保存或读取失败"})
		}
	}
	mux.HandleFunc("GET /api/accounts/{id}/procurement-mappings/{offerId}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.Exists(db, id) {
			send(w, http.StatusNotFound, map[string]string{"error": "账户不存在"})
			return
		}
		result, e := store.GetProcurementMapping(db, id, r.PathValue("offerId"))
		respond(w, result, e)
	})
	mux.HandleFunc("PUT /api/accounts/{id}/procurement-mappings/{offerId}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.Exists(db, id) {
			send(w, http.StatusNotFound, map[string]string{"error": "账户不存在"})
			return
		}
		var in struct {
			Revision *int64                   `json:"revision"`
			Mapping  store.ProcurementMapping `json:"mapping"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&in) != nil || in.Revision == nil || decoder.Decode(&struct{}{}) != io.EOF {
			send(w, http.StatusBadRequest, map[string]string{"error": "采购映射格式错误，请提供revision和mapping；不接受DOM选择器等未知字段"})
			return
		}
		result, e := store.SaveProcurementMapping(db, currentUser(r).ID, id, r.PathValue("offerId"), *in.Revision, in.Mapping)
		respond(w, result, e)
	})
}
