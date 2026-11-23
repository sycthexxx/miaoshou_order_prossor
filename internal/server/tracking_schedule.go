package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math/rand/v2"
	"net/http"
	"order_processor/internal/store"
	"os"
	"path/filepath"
	"time"
)

const trackingSettingsFile = "tracking-sync-settings.json"

type trackingSchedule struct {
	Enabled    *bool `json:"enabled"`
	MinMinutes int   `json:"minMinutes"`
	MaxMinutes int   `json:"maxMinutes"`
	MinSeconds int   `json:"minSeconds"`
	MaxSeconds int   `json:"maxSeconds"`
}

func (s trackingSchedule) valid() bool {
	return s.Enabled != nil && s.MinMinutes >= 1 && s.MinMinutes <= s.MaxMinutes && s.MaxMinutes <= 1440 &&
		s.MinSeconds >= 1 && s.MinSeconds <= s.MaxSeconds && s.MaxSeconds <= 300
}

func readTrackingSchedule(file string) (trackingSchedule, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		enabled := false
		return trackingSchedule{Enabled: &enabled, MinMinutes: 50, MaxMinutes: 100, MinSeconds: 3, MaxSeconds: 10}, nil
	}
	var settings trackingSchedule
	if err == nil && (json.Unmarshal(data, &settings) != nil || !settings.valid()) {
		err = errors.New("物流轨迹同步设置无效")
	}
	return settings, err
}

func registerTrackingSchedule(ctx context.Context, mux *http.ServeMux, dir string, g *browserGateway, notifications *orderEventNotifications) {
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		mux.HandleFunc(method+" /api/accounts/{id}/first-mile/tracking-sync-settings", func(w http.ResponseWriter, r *http.Request) {
			if !currentUser(r).IsAdmin {
				send(w, 403, map[string]string{"error": "只有管理员可以管理物流轨迹同步"})
				return
			}
			key := store.StorageKey(g.db, r.PathValue("id"))
			if key == "" {
				send(w, 404, map[string]string{"error": "账户不存在"})
				return
			}
			file := filepath.Join(dir, "runtime", key, trackingSettingsFile)
			if method == http.MethodGet {
				settings, err := readTrackingSchedule(file)
				if err != nil {
					send(w, 500, map[string]string{"error": "读取物流轨迹同步设置失败"})
					return
				}
				send(w, 200, settings)
				return
			}
			var settings trackingSchedule
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&settings) != nil || !settings.valid() {
				send(w, 400, map[string]string{"error": "轮次间隔须为 1～1440 分钟，逐单间隔须为 1～300 秒，最小值不能大于最大值"})
				return
			}
			if err := saveSyncSettings(file, settings); err != nil {
				send(w, 500, map[string]string{"error": "保存物流轨迹同步设置失败"})
				return
			}
			send(w, 200, settings)
		})
	}
	if ctx != nil {
		go g.runTrackingSchedules(ctx, dir, notifications)
	}
}

func trackingDelay(minimum, maximum int, unit time.Duration) time.Duration {
	return time.Duration(minimum+rand.IntN(maximum-minimum+1)) * unit
}

func (g *browserGateway) runTrackingSchedules(ctx context.Context, dir string, notifications *orderEventNotifications) {
	type scheduled struct {
		settings [4]int
		next     time.Time
		done     chan struct{}
		cancel   context.CancelFunc
	}
	schedules := map[string]scheduled{}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			accounts, err := store.Accounts(g.db)
			if err != nil {
				log.Printf("物流轨迹定时同步读取账户失败：%v", err)
				continue
			}
			enabled := make(map[string]bool, len(accounts))
			for _, account := range accounts {
				settings, err := readTrackingSchedule(filepath.Join(dir, "runtime", account.StorageKey, trackingSettingsFile))
				if err != nil || !*settings.Enabled {
					continue
				}
				enabled[account.ID] = true
				task := schedules[account.ID]
				if task.done != nil {
					select {
					case <-task.done:
						task = scheduled{}
					default:
						continue
					}
				}
				values := [4]int{settings.MinMinutes, settings.MaxMinutes, settings.MinSeconds, settings.MaxSeconds}
				if values != task.settings {
					task.settings = values
					task.next = now.Add(trackingDelay(settings.MinMinutes, settings.MaxMinutes, time.Minute))
				}
				if !now.Before(task.next) {
					runCtx, cancel := context.WithCancel(ctx)
					task.done, task.cancel = make(chan struct{}), cancel
					go func() {
						defer close(task.done)
						defer cancel()
						g.syncPendingWaybills(runCtx, account, settings, notifications)
					}()
				}
				schedules[account.ID] = task
			}
			for id, task := range schedules {
				if enabled[id] {
					continue
				}
				if task.done != nil {
					task.cancel()
					select {
					case <-task.done:
					default:
						continue
					}
				}
				delete(schedules, id)
			}
		}
	}
}

func (g *browserGateway) syncPendingWaybills(ctx context.Context, account store.Account, settings trackingSchedule, notifications *orderEventNotifications) {
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	target, err := g.runtime.Resolve(checkCtx, account.StorageKey, false)
	if err != nil {
		cancel()
		return
	}
	status, data, err := g.call(checkCtx, target, account.StorageKey, http.MethodGet, "status")
	cancel()
	if err != nil || status != http.StatusOK || data["state"] != "verified" {
		return
	}
	queries, err := store.PendingShippedWaybills(g.db, account.ID)
	if err != nil {
		log.Printf("账户 %s 读取待同步物流轨迹失败：%v", account.ID, err)
		return
	}
	success, failed := 0, 0
	for i, query := range queries {
		if i > 0 {
			timer := time.NewTimer(trackingDelay(settings.MinSeconds, settings.MaxSeconds, time.Second))
			select {
			case <-ctx.Done():
				timer.Stop()
			case <-timer.C:
			}
		}
		if ctx.Err() != nil {
			break
		}
		var pending bool
		err := g.db.QueryRowContext(ctx, `SELECT COALESCE((SELECT status FROM domestic_tracking
			WHERE account_id=? AND number=?), '') IN ('','已打包','已揽收','运输中')`, account.ID, query["number"]).Scan(&pending)
		if err != nil {
			log.Printf("账户 %s 读取国内物流状态失败：%v", account.ID, err)
			break
		}
		if !pending {
			continue
		}
		if _, _, err := g.syncDomesticTracking(ctx, account.ID, query); err != nil {
			failed++
			continue
		}
		success++
	}
	if success > 0 {
		notifications.publishOrderData(account.ID)
	}
	log.Printf("账户 %s 物流轨迹自动同步完成：成功 %d，失败 %d", account.ID, success, failed)
}
