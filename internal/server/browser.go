package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httputil"
	"order_processor/internal/adapters/miaoshou/web/browser"
	"order_processor/internal/store"
	"strings"
	"sync"
	"time"
)

type browserGate struct {
	sync.Mutex
	desktop bool
}
type accountBrowserRuntime interface {
	browser.Runtime
	Restart(context.Context, string) (browser.Target, error)
	Stop(context.Context, string) error
}

type browserGateway struct {
	db      *sql.DB
	runtime accountBrowserRuntime
	client  *http.Client
	gates   sync.Map

	packingMu sync.Mutex
}

func (g *browserGateway) gate(id string) *browserGate {
	v, _ := g.gates.LoadOrStore(id, &browserGate{})
	return v.(*browserGate)
}
func (g *browserGateway) call(ctx context.Context, t browser.Target, key, method, action string) (int, map[string]any, error) {
	req, e := http.NewRequestWithContext(ctx, method, t.API.String()+"/accounts/"+key+"/"+action, nil)
	if e != nil {
		return 0, nil, e
	}
	// Never forward the employee browser's cookies, authorization, or proxy headers.
	req.Header.Set("Authorization", "Bearer "+t.Token)
	resp, e := g.client.Do(req)
	if e != nil {
		return 0, nil, e
	}
	defer resp.Body.Close()
	var data map[string]any
	e = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&data)
	return resp.StatusCode, data, e
}
func (g *browserGateway) action(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.Exists(g.db, id) {
			send(w, 404, map[string]string{"error": "账户不存在"})
			return
		}
		gate := g.gate(id)
		restart := action == "restart-container"
		if restart {
			if !gate.TryLock() {
				send(w, 409, map[string]string{"error": "账户浏览器正在执行其他操作，请稍后重试"})
				return
			}
			defer gate.Unlock()
		} else if action != "status" {
			gate.Lock()
			defer gate.Unlock()
			if gate.desktop && action != "stop-sync" {
				send(w, 409, map[string]string{"error": "请先关闭该账户的远程桌面，再操作浏览器或启动同步"})
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 100*time.Second)
		defer cancel()
		key := store.StorageKey(g.db, id)
		target, e := g.runtime.Resolve(ctx, key, action == "login")
		workerAction := action
		if restart {
			if e == nil {
				probe, done := context.WithTimeout(ctx, 2*time.Second)
				_, state, _ := g.call(probe, target, key, "GET", "status")
				done()
				job, _ := state["sync"].(map[string]any)
				if job["state"] == "running" {
					send(w, 409, map[string]string{"error": "订单正在同步，请在同步结束后重启容器"})
					return
				}
			}
			target, e = g.runtime.Restart(ctx, key)
			workerAction = "login"
		}
		if errors.Is(e, browser.ErrNotStarted) && action == "status" {
			send(w, 200, map[string]any{"state": "closed", "browserMode": "docker"})
			return
		}
		if e != nil {
			message, code := browserRuntimeError(e)
			send(w, 503, map[string]string{"error": message, "errorCode": code, "browserMode": "docker"})
			return
		}
		status, data, e := g.call(ctx, target, key, r.Method, workerAction)
		if restart && (e != nil || status != 200 || data["state"] == "closed" || data == nil) {
			send(w, 503, map[string]string{"error": "容器已重启，浏览器未打开，请点击「打开远程浏览器」重试", "browserMode": "docker"})
			return
		}
		if e != nil {
			send(w, 503, map[string]string{"error": "浏览器服务暂不可用，请查看启动终端", "errorCode": "worker_unavailable", "browserMode": "docker"})
			return
		}
		data["browserMode"] = "docker"
		if target.Desktop != nil {
			data["desktopURL"] = "/desktop/" + id + "/vnc.html?autoconnect=1&resize=scale&path=desktop/" + id + "/websockify"
		}
		send(w, status, data)
	}
}
func (g *browserGateway) desktop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !store.Exists(g.db, id) {
		http.NotFound(w, r)
		return
	}
	target, e := g.runtime.Resolve(r.Context(), store.StorageKey(g.db, id), false)
	if e != nil {
		message, _ := browserRuntimeError(e)
		http.Error(w, message, 409)
		return
	}
	if target.Desktop == nil {
		http.Error(w, "请先打开该账户浏览器", 409)
		return
	}
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		gate := g.gate(id)
		gate.Lock()
		if gate.desktop {
			gate.Unlock()
			http.Error(w, "该账户远程桌面已被占用", 409)
			return
		}
		code, state, e := g.call(r.Context(), target, store.StorageKey(g.db, id), "GET", "status")
		job, _ := state["sync"].(map[string]any)
		if e != nil || code != 200 || job["state"] == "running" {
			gate.Unlock()
			http.Error(w, "同步中或浏览器不可用，请稍后连接", 409)
			return
		}
		gate.desktop = true
		gate.Unlock()
		defer func() { gate.Lock(); gate.desktop = false; gate.Unlock() }()
	}
	proxy := &httputil.ReverseProxy{Rewrite: func(p *httputil.ProxyRequest) {
		p.SetURL(target.Desktop)
		p.Out.URL.Path = "/" + r.PathValue("path")
		p.Out.URL.RawPath = ""
		// Only protocol headers are needed by noVNC; do not expose company cookies.
		h := make(http.Header)
		for _, name := range []string{"Accept", "Accept-Encoding", "User-Agent", "Connection", "Upgrade", "Sec-WebSocket-Key", "Sec-WebSocket-Version", "Sec-WebSocket-Protocol", "Sec-WebSocket-Extensions"} {
			if v := p.In.Header.Values(name); len(v) > 0 {
				h[name] = v
			}
		}
		p.Out.Header = h
	}, ErrorHandler: func(w http.ResponseWriter, r *http.Request, e error) { http.Error(w, "远程桌面暂不可用", 502) }}
	proxy.ServeHTTP(w, r)
}

func browserRuntimeError(err error) (string, string) {
	if errors.Is(err, browser.ErrNotStarted) {
		return "该账户浏览器尚未启动，请点击打开登录窗口", "browser_not_started"
	}
	var known *browser.RuntimeError
	if errors.As(err, &known) {
		return known.Message, known.Code
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "浏览器运行环境响应超时，请查看启动终端和 Docker 容器状态", "runtime_timeout"
	}
	return "浏览器运行环境检查失败，请查看启动终端和 Docker 容器状态", "runtime_unavailable"
}
