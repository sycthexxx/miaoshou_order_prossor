package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"order_processor/internal/domain/connections"
	"order_processor/internal/store"
	"strconv"
)

func apiCredentialRoutes(mux *http.ServeMux, db *sql.DB) {
	exists := func(w http.ResponseWriter, r *http.Request) bool {
		if store.Exists(db, r.PathValue("id")) {
			return true
		}
		send(w, 404, map[string]string{"error": "账户不存在"})
		return false
	}
	mux.HandleFunc("GET /api/accounts/{id}/api-credentials", func(w http.ResponseWriter, r *http.Request) {
		if !exists(w, r) {
			return
		}
		list, err := store.APICredentials(db, r.PathValue("id"))
		if err != nil {
			send(w, 500, map[string]string{"error": "读取 API 凭证失败"})
			return
		}
		send(w, 200, list)
	})
	mux.HandleFunc("GET /api/accounts/{id}/api-credentials/{credential}", func(w http.ResponseWriter, r *http.Request) {
		if !exists(w, r) {
			return
		}
		id, err := strconv.ParseInt(r.PathValue("credential"), 10, 64)
		if err != nil || id <= 0 {
			send(w, 400, map[string]string{"error": "API 凭证编号无效"})
			return
		}
		credential, err := store.APICredential(db, r.PathValue("id"), id)
		if err == sql.ErrNoRows {
			send(w, 404, map[string]string{"error": "API 凭证不存在"})
			return
		}
		if err != nil {
			send(w, 500, map[string]string{"error": "读取 API 凭证失败"})
			return
		}
		send(w, 200, struct {
			connections.Credential
			Key string `json:"key"`
		}{credential, credential.Key})
	})
	save := func(w http.ResponseWriter, r *http.Request) {
		if !exists(w, r) {
			return
		}
		var in connections.Input
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			send(w, 400, map[string]string{"error": "请求格式错误"})
			return
		}
		var id int64
		if r.PathValue("credential") != "" {
			var err error
			id, err = strconv.ParseInt(r.PathValue("credential"), 10, 64)
			if err != nil || id <= 0 {
				send(w, 400, map[string]string{"error": "API 凭证编号无效"})
				return
			}
		}
		id, err := store.SaveAPICredential(db, r.PathValue("id"), id, in)
		if err != nil {
			send(w, 400, map[string]string{"error": err.Error()})
			return
		}
		send(w, 200, map[string]int64{"id": id})
	}
	mux.HandleFunc("POST /api/accounts/{id}/api-credentials", save)
	mux.HandleFunc("PUT /api/accounts/{id}/api-credentials/{credential}", save)
	change := func(remove bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !exists(w, r) {
				return
			}
			id, err := strconv.ParseInt(r.PathValue("credential"), 10, 64)
			if err != nil || id <= 0 {
				send(w, 400, map[string]string{"error": "API 凭证编号无效"})
				return
			}
			if remove {
				err = store.DeleteAPICredential(db, r.PathValue("id"), id)
			} else {
				err = store.SetCurrentAPICredential(db, r.PathValue("id"), id)
			}
			if err != nil {
				send(w, 400, map[string]string{"error": err.Error()})
				return
			}
			send(w, 200, map[string]bool{"ok": true})
		}
	}
	mux.HandleFunc("POST /api/accounts/{id}/api-credentials/{credential}/activate", change(false))
	mux.HandleFunc("DELETE /api/accounts/{id}/api-credentials/{credential}", change(true))
}
