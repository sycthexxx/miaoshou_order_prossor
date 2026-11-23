package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"order_processor/internal/store"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var firstMileRunID = regexp.MustCompile(`^[0-9]{13}$`)

type firstMileSettings struct {
	Enabled         *bool `json:"enabled"`
	IntervalMinutes int   `json:"intervalMinutes"`
}

func (s firstMileSettings) valid() bool {
	return s.Enabled != nil && s.IntervalMinutes >= 1 && s.IntervalMinutes <= 1440
}

func saveSyncSettings(file string, settings any) error {
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), ".sync-settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := json.NewEncoder(tmp).Encode(settings); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), file)
}

func registerFirstMile(mux *http.ServeMux, db *sql.DB, dir string, g *browserGateway) {
	accountRoute := func(fn func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			key := store.StorageKey(db, r.PathValue("id"))
			if key == "" {
				send(w, 404, map[string]string{"error": "账户不存在"})
				return
			}
			fn(w, r, filepath.Join(dir, "runtime", key))
		}
	}
	const route = "/api/accounts/{id}/first-mile"
	mux.HandleFunc("POST "+route+"/runs", accountRoute(func(w http.ResponseWriter, r *http.Request, _ string) {
		if !currentUser(r).IsAdmin {
			send(w, 403, map[string]string{"error": "只有管理员可以手动同步"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		key := store.StorageKey(db, r.PathValue("id"))
		target, err := g.runtime.Resolve(ctx, key, false)
		if err != nil {
			send(w, 409, map[string]string{"error": "请先在账户管理打开并登录妙手浏览器"})
			return
		}
		status, data, err := g.call(ctx, target, key, http.MethodPost, "first-mile-start")
		if err != nil || (status != 202 && status != 409) {
			send(w, 503, map[string]string{"error": "暂时无法启动头程物流同步"})
			return
		}
		send(w, status, data)
	}))
	mux.HandleFunc("GET "+route+"/settings", accountRoute(func(w http.ResponseWriter, r *http.Request, dir string) {
		enabled := true
		settings := firstMileSettings{Enabled: &enabled, IntervalMinutes: 60}
		data, err := os.ReadFile(filepath.Join(dir, "first-mile-settings.json"))
		if err == nil {
			settings = firstMileSettings{}
			if json.Unmarshal(data, &settings) != nil || !settings.valid() {
				err = errors.New("invalid first-mile settings")
			}
		}
		if err != nil && !os.IsNotExist(err) {
			send(w, 500, map[string]string{"error": "读取头程同步设置失败"})
			return
		}
		send(w, 200, settings)
	}))
	mux.HandleFunc("PUT "+route+"/settings", accountRoute(func(w http.ResponseWriter, r *http.Request, dir string) {
		if !currentUser(r).IsAdmin {
			send(w, 403, map[string]string{"error": "只有管理员可以修改同步设置"})
			return
		}
		var settings firstMileSettings
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&settings) != nil || !settings.valid() {
			send(w, 400, map[string]string{"error": "请输入 1 到 1440 分钟的整数同步间隔"})
			return
		}
		if err := saveSyncSettings(filepath.Join(dir, "first-mile-settings.json"), settings); err != nil {
			send(w, 500, map[string]string{"error": "保存头程同步设置失败"})
			return
		}
		send(w, 200, settings)
	}))
	mux.HandleFunc("GET "+route+"/runs", accountRoute(func(w http.ResponseWriter, r *http.Request, dir string) {
		before := r.URL.Query().Get("before")
		if before != "" && !firstMileRunID.MatchString(before) {
			send(w, 400, map[string]string{"error": "记录游标无效"})
			return
		}
		dir = filepath.Join(dir, "first-mile-runs")
		entries, err := os.ReadDir(dir)
		if err != nil && !os.IsNotExist(err) {
			send(w, 500, map[string]string{"error": "读取头程同步记录失败"})
			return
		}
		runs := []map[string]json.RawMessage{}
		next, last := "", ""
		for i := len(entries) - 1; i >= 0; i-- {
			name := entries[i].Name()
			id := strings.TrimSuffix(name, ".json")
			if id == name || !firstMileRunID.MatchString(id) || before != "" && id >= before || entries[i].IsDir() {
				continue
			}
			if len(runs) == 20 {
				next = last
				break
			}
			data, err := os.ReadFile(filepath.Join(dir, name))
			var run map[string]json.RawMessage
			if err != nil || json.Unmarshal(data, &run) != nil || run == nil {
				send(w, 500, map[string]string{"error": "读取头程同步记录失败"})
				return
			}
			delete(run, "items")
			if string(run["state"]) == `"running"` {
				if _, err := os.Stat(filepath.Join(dir, id+".stop")); err == nil {
					run["state"] = json.RawMessage(`"stopping"`)
				}
			}
			runs = append(runs, run)
			last = id
		}
		send(w, 200, map[string]any{"runs": runs, "next": next})
	}))
	mux.HandleFunc("GET "+route+"/runs/{run}", accountRoute(func(w http.ResponseWriter, r *http.Request, dir string) {
		id := r.PathValue("run")
		if !firstMileRunID.MatchString(id) {
			send(w, 404, map[string]string{"error": "同步记录不存在"})
			return
		}
		data, err := os.ReadFile(filepath.Join(dir, "first-mile-runs", id+".json"))
		if os.IsNotExist(err) {
			send(w, 404, map[string]string{"error": "同步记录不存在"})
			return
		}
		if err != nil || !json.Valid(data) {
			send(w, 500, map[string]string{"error": "读取头程同步明细失败"})
			return
		}
		send(w, 200, json.RawMessage(data))
	}))
	mux.HandleFunc("POST "+route+"/runs/{run}/stop", accountRoute(func(w http.ResponseWriter, r *http.Request, dir string) {
		if !currentUser(r).IsAdmin {
			send(w, 403, map[string]string{"error": "只有管理员可以停止同步"})
			return
		}
		id := r.PathValue("run")
		if !firstMileRunID.MatchString(id) {
			send(w, 404, map[string]string{"error": "同步记录不存在"})
			return
		}
		dir = filepath.Join(dir, "first-mile-runs")
		data, err := os.ReadFile(filepath.Join(dir, id+".json"))
		var run struct {
			State   string
			CanStop bool
		}
		switch {
		case os.IsNotExist(err):
			send(w, 404, map[string]string{"error": "同步记录不存在"})
		case err != nil || json.Unmarshal(data, &run) != nil:
			send(w, 500, map[string]string{"error": "读取头程同步记录失败"})
		case run.State != "running":
			send(w, 409, map[string]string{"error": "本轮同步已结束"})
		case !run.CanStop:
			send(w, 409, map[string]string{"error": "本轮由旧版浏览器进程启动"})
		default:
			if err := os.WriteFile(filepath.Join(dir, id+".stop"), nil, 0600); err != nil {
				send(w, 500, map[string]string{"error": "发送停止请求失败"})
				return
			}
			send(w, 202, map[string]string{"state": "stopping"})
		}
	}))
}
