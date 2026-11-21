package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"order_processor/internal/adapters/miaoshou/web/browser"
	"order_processor/internal/store"
	"strconv"
	"time"
)

func registerUserAdmin(mux *http.ServeMux, db *sql.DB) {
	mux.HandleFunc("GET /api/users", func(w http.ResponseWriter, r *http.Request) {
		users, err := store.Users(r.Context(), db)
		if err != nil {
			send(w, 500, map[string]string{"error": "用户列表读取失败"})
			return
		}
		send(w, 200, users)
	})
	save := func(w http.ResponseWriter, r *http.Request) {
		var id int64
		if r.Method == http.MethodPut {
			var err error
			id, err = strconv.ParseInt(r.PathValue("user"), 10, 64)
			if err != nil || id < 1 {
				send(w, 404, map[string]string{"error": "用户不存在"})
				return
			}
		}
		var input store.UserInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil {
			send(w, 400, map[string]string{"error": "用户信息格式错误"})
			return
		}
		id, err := store.SaveUser(r.Context(), db, id, input)
		if err != nil {
			userAdminError(w, err)
			return
		}
		status := 200
		if r.Method == http.MethodPost {
			status = 201
		}
		send(w, status, map[string]int64{"id": id})
	}
	mux.HandleFunc("POST /api/users", save)
	mux.HandleFunc("PUT /api/users/{user}", save)
	mux.HandleFunc("DELETE /api/users/{user}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("user"), 10, 64)
		if err != nil || id < 1 {
			send(w, 404, map[string]string{"error": "用户不存在"})
			return
		}
		if err = store.DeleteUser(r.Context(), db, id); err != nil {
			userAdminError(w, err)
			return
		}
		send(w, 200, map[string]bool{"ok": true})
	})
}

func userAdminError(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		send(w, 404, map[string]string{"error": "用户不存在"})
		return
	}
	send(w, 400, map[string]string{"error": err.Error()})
}

func (g *browserGateway) removeAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	gate := g.gate(id)
	if !gate.TryLock() {
		send(w, 409, map[string]string{"error": "账户正在执行其他操作，请稍后删除"})
		return
	}
	defer gate.Unlock()
	if gate.desktop {
		send(w, 409, map[string]string{"error": "请先关闭远程桌面并结束订单同步"})
		return
	}
	key := store.StorageKey(g.db, id)
	if key == "" {
		send(w, 404, map[string]string{"error": "账户不存在"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var pending bool
	if err := g.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM packing_jobs WHERE account_id=? AND state IN ('queued','running','paused'))", id).Scan(&pending); err != nil {
		send(w, 500, map[string]string{"error": "读取账户任务失败"})
		return
	}
	if pending {
		send(w, 409, map[string]string{"error": "该账户有尚未结束的打包任务，请先完成或取消任务"})
		return
	}
	t, err := g.runtime.Resolve(ctx, key, false)
	if err == nil {
		// The worker's close operation also refuses to interrupt an active sync.
		code, reply, err := g.call(ctx, t, key, http.MethodPost, "close")
		if err != nil {
			send(w, 503, map[string]string{"error": "关闭账户浏览器失败，请稍后重试"})
			return
		}
		if code != 200 {
			send(w, code, reply)
			return
		}
		if err = g.runtime.Stop(ctx, key); err != nil {
			message, _ := browserRuntimeError(err)
			send(w, 503, map[string]string{"error": message})
			return
		}
	} else if !errors.Is(err, browser.ErrNotStarted) {
		message, _ := browserRuntimeError(err)
		send(w, 503, map[string]string{"error": message})
		return
	}
	if err = store.RemoveAccount(ctx, g.db, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			send(w, 404, map[string]string{"error": "账户不存在"})
			return
		}
		send(w, 409, map[string]string{"error": err.Error()})
		return
	}
	send(w, 200, map[string]bool{"ok": true})
}
