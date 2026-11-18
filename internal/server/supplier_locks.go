package server

import (
	"database/sql"
	"errors"
	"net/http"
	"order_processor/internal/store"
	"strconv"
)

func registerSupplierLocks(mux *http.ServeMux, db *sql.DB) {
	const path = "/api/accounts/{id}/supplier-locks"
	mux.HandleFunc("GET "+path, func(w http.ResponseWriter, r *http.Request) {
		leases, err := store.SupplierLeases(r.Context(), db, r.PathValue("id"))
		if err != nil {
			send(w, 500, map[string]string{"error": "厂家状态读取失败"})
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		viewer := currentUser(r).ID
		for i := range leases {
			if leases[i].UserID != viewer {
				leases[i].ID = 0
			}
		}
		send(w, 200, leases)
	})
	change := func(w http.ResponseWriter, r *http.Request) {
		account := r.PathValue("id")
		if !store.Exists(db, account) {
			send(w, 404, map[string]string{"error": "妙手账户不存在"})
			return
		}
		supplier, err := strconv.ParseInt(r.PathValue("supplier"), 10, 64)
		if err != nil || supplier < 0 {
			send(w, 400, map[string]string{"error": "厂家编号无效"})
			return
		}
		var id int64
		if value := r.Header.Get("X-Supplier-Lease"); value != "" {
			id, err = strconv.ParseInt(value, 10, 64)
			if err != nil || id <= 0 {
				send(w, 400, map[string]string{"error": "操作编号无效"})
				return
			}
		}
		user := currentUser(r)
		lease := store.SupplierLease{ID: id, Supplier: supplier, UserID: user.ID, Username: user.Username}
		if r.Method == http.MethodDelete {
			if err := store.ReleaseSupplier(r.Context(), db, account, lease); errors.Is(err, store.ErrSupplierLeaseLost) {
				send(w, 409, map[string]string{"error": err.Error()})
				return
			} else if err != nil {
				send(w, 500, map[string]string{"error": "释放厂家失败"})
				return
			}
			send(w, 200, map[string]bool{"ok": true})
			return
		}
		lease, err = store.AcquireSupplier(r.Context(), db, account, lease)
		if err != nil {
			send(w, 409, map[string]string{"error": err.Error()})
			return
		}
		send(w, 200, lease)
	}
	mux.HandleFunc("POST "+path+"/{supplier}", change)
	mux.HandleFunc("DELETE "+path+"/{supplier}", change)
}
