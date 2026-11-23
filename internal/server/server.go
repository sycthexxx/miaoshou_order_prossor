package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"order_processor/internal/store"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func send(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func NewWithRuntimeContext(ctx context.Context, db *sql.DB, dir string, runtime accountBrowserRuntime, publicOrigin string) http.Handler {
	mux := http.NewServeMux()
	type pluginSettings struct {
		Account string `json:"account"`
		Gifts   bool   `json:"gifts"`
	}
	mux.HandleFunc("GET /api/plugin-query-settings", func(w http.ResponseWriter, r *http.Request) {
		var settings pluginSettings
		err := db.QueryRow(`SELECT a.id, COALESCE(s.include_gifts,0) FROM accounts a
            LEFT JOIN plugin_query_settings s ON s.account_id=a.id
            WHERE a.deleted=0 AND (? OR EXISTS (SELECT 1 FROM user_accounts u WHERE u.user_id=? AND u.account_id=a.id))
            ORDER BY (s.id IS NULL), a.id LIMIT 1`, currentUser(r).IsAdmin, currentUser(r).ID).Scan(&settings.Account, &settings.Gifts)
		if err != nil && err != sql.ErrNoRows {
			send(w, 500, map[string]string{"error": "插件设置读取失败"})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		send(w, 200, settings)
	})
	mux.HandleFunc("PUT /api/plugin-query-settings", func(w http.ResponseWriter, r *http.Request) {
		var settings pluginSettings
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&settings) != nil || store.StorageKey(db, settings.Account) == "" {
			send(w, 400, map[string]string{"error": "请选择有效账户"})
			return
		}
		_, err := db.Exec(`INSERT INTO plugin_query_settings(id,account_id,include_gifts) VALUES(1,?,?)
            ON CONFLICT(id) DO UPDATE SET account_id=excluded.account_id,include_gifts=excluded.include_gifts`, settings.Account, settings.Gifts)
		if err != nil {
			send(w, 500, map[string]string{"error": "插件设置保存失败"})
			return
		}
		send(w, 200, settings)
	})
	mux.HandleFunc("GET /api/accounts", func(w http.ResponseWriter, r *http.Request) {
		a, e := accessibleAccounts(db, currentUser(r))
		if e != nil {
			send(w, 500, map[string]string{"error": "账户读取失败"})
			return
		}
		send(w, 200, a)
	})
	mux.HandleFunc("POST /api/accounts", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name string `json:"name"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil {
			send(w, 400, map[string]string{"error": "请求格式错误"})
			return
		}
		in.Name = strings.TrimSpace(in.Name)
		if in.Name == "" || len([]rune(in.Name)) > 60 {
			send(w, 400, map[string]string{"error": "请输入1至60字的账户名称"})
			return
		}
		id, e := store.Create(db, in.Name)
		if e != nil {
			send(w, 409, map[string]string{"error": "创建失败，请检查名称是否重复"})
			return
		}
		send(w, 201, map[string]string{"id": id})
	})
	g := &browserGateway{db: db, runtime: runtime, client: &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
	registerUserAdmin(mux, db)
	registerFirstMile(mux, db, dir, g)
	registerTrackingRules(mux, db)
	registerSupplierLocks(mux, db)
	mux.HandleFunc("DELETE /api/accounts/{id}", g.removeAccount)
	packingJobRoutes(ctx, mux, g)
	proxy := g.action
	mux.HandleFunc("POST /api/accounts/{id}/packages/{package}/packing", g.packing)
	mux.HandleFunc("GET /api/accounts/{id}/logistics-company", g.logisticsCompany)
	mux.HandleFunc("GET /api/accounts/{id}/purchase-info", g.purchaseInfo)
	mux.HandleFunc("GET /api/accounts/{id}/packages/{package}/amount-detail", g.amountDetail)
	mux.HandleFunc("GET /api/accounts/{id}/packages/{package}/tracking", g.tracking)
	mux.HandleFunc("GET /desktop/{id}/{path...}", g.desktop)
	mux.HandleFunc("POST /api/accounts/{id}/login", proxy("login"))
	mux.HandleFunc("POST /api/accounts/{id}/restart-container", proxy("restart-container"))
	mux.HandleFunc("POST /api/accounts/{id}/close", proxy("close"))
	mux.HandleFunc("GET /api/accounts/{id}/status", proxy("status"))
	mux.HandleFunc("POST /api/accounts/{id}/sync", proxy("sync"))
	mux.HandleFunc("POST /api/accounts/{id}/sync-products", proxy("sync-products"))
	mux.HandleFunc("GET /api/accounts/{id}/products", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.Exists(db, id) {
			send(w, 404, map[string]string{"error": "账户不存在"})
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 || page > 1000000 {
			page = 1
		}
		d, e := store.Products(db, id, r.URL.Query().Get("q"), page, r.URL.Query().Get("status"))
		if e != nil {
			send(w, 500, map[string]string{"error": "产品读取失败"})
			return
		}
		send(w, 200, d)
	})
	mux.HandleFunc("POST /api/accounts/{id}/sync-pending", proxy("sync-pending"))
	mux.HandleFunc("POST /api/accounts/{id}/stop-sync", proxy("stop-sync"))
	cache := procurementRoutes(mux, db)
	notifications := &orderEventNotifications{}
	registerTrackingSchedule(ctx, mux, dir, g, notifications)
	orderEventRoutes(ctx, mux, db, dir, cache, notifications)
	sourceStatusRoutes(mux, db, cache)
	supplierRoutes(mux, db, cache, notifications)
	mux.HandleFunc("GET /api/accounts/{id}/archives", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.Exists(db, id) {
			send(w, 404, map[string]string{"error": "账户不存在"})
			return
		}
		entries, e := os.ReadDir(filepath.Join(dir, "raw", store.StorageKey(db, id)))
		if e != nil && !os.IsNotExist(e) {
			send(w, 500, map[string]string{"error": "读取存档失败"})
			return
		}
		out := []any{}
		for i := len(entries) - 1; i >= 0 && len(out) < 100; i-- {
			entry := entries[i]
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			b, e := os.ReadFile(filepath.Join(dir, "raw", store.StorageKey(db, id), entry.Name()))
			if e != nil {
				continue
			}
			var m map[string]any
			if json.Unmarshal(b, &m) != nil {
				continue
			}
			delete(m, "bodyBase64")
			delete(m, "requestBody")
			m["file"] = entry.Name()
			out = append(out, m)
		}
		send(w, 200, out)
	})

	mux.HandleFunc("GET /api/accounts/{id}/shipped-summary", func(w http.ResponseWriter, r *http.Request) {
		groups, err := store.ShippedSummary(db, r.PathValue("id"))
		if err != nil {
			send(w, 500, map[string]string{"error": "读取已出货汇总失败"})
			return
		}
		send(w, 200, groups)
	})
	mux.HandleFunc("GET /api/accounts/{id}/orders", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.Exists(db, id) {
			send(w, 404, map[string]string{"error": "账户不存在"})
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 || page > 1000000 {
			page = 1
		}
		data, e := store.Orders(db, id, r.URL.Query().Get("q"), page, 20, r.URL.Query().Get("status"), r.URL.Query().Get("purchaseTab"))
		if e != nil {
			send(w, 500, map[string]string{"error": "订单读取失败"})
			return
		}
		send(w, 200, data)
	})
	mux.Handle("/", http.FileServer(http.Dir("dist")))
	auth := &authentication{db: db, sessions: map[string]loginSession{}}
	mux.HandleFunc("GET /api/accounts/{id}/procurement/ws", cache.stream(auth))
	mux.HandleFunc("GET /api/accounts/{id}/order-events/ws", notifications.stream(db, auth))
	return requestBoundary(auth.protect(mux), publicOrigin)
}
