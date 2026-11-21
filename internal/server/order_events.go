package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"order_processor/internal/store"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func orderEventCursor(r *http.Request) (*int64, error) {
	raw, ok := r.URL.Query()["after"]
	if !ok {
		return nil, nil
	}
	n, err := strconv.ParseInt(raw[0], 10, 64)
	if err != nil || n < 0 {
		return nil, errors.New("消息游标无效")
	}
	return &n, nil
}

func orderEventRoutes(ctx context.Context, mux *http.ServeMux, db *sql.DB, dir string, cache *procurementCache, notifications *orderEventNotifications) {
	mux.HandleFunc("GET /api/accounts/{id}/order-events", func(w http.ResponseWriter, r *http.Request) {
		after, err := orderEventCursor(r)
		if err != nil {
			send(w, 400, map[string]string{"error": "消息游标无效"})
			return
		}
		events, cursor, err := store.OrderEvents(db, r.PathValue("id"), after)
		if err != nil {
			send(w, 500, map[string]string{"error": "读取订单提醒失败"})
			return
		}
		send(w, 200, map[string]any{"cursor": cursor, "events": events})
	})
	if ctx == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			if err := drainOrderEvents(ctx, db, dir, cache, notifications); err != nil && ctx.Err() == nil {
				log.Printf("订单提醒：%v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func drainOrderEvents(ctx context.Context, db *sql.DB, dir string, cache *procurementCache, notifications *orderEventNotifications) error {
	accounts, err := store.Accounts(db)
	if err != nil {
		return err
	}
	var failures []error
	for _, account := range accounts {
		files, err := filepath.Glob(filepath.Join(dir, "runtime", account.StorageKey, "order-events", "*.json"))
		if err != nil {
			return err
		}
		var applied []string
		for _, file := range files {
			if err := ctx.Err(); err != nil {
				return err
			}
			raw, err := os.ReadFile(file)
			if err == nil {
				err = store.ImportOrderEvent(ctx, db, account, raw)
			}
			if err != nil {
				failures = append(failures, fmt.Errorf("账户 %s 文件 %s：%w", account.ID, filepath.Base(file), err))
				continue
			}
			applied = append(applied, file)
		}
		if len(applied) == 0 {
			continue
		}
		if notifications != nil {
			notifications.publish(account.ID)
		}
		// Keep the files until all active procurement variants have been updated.
		// A crash between commit, broadcast, and removal can safely replay them.
		if err := cache.refreshAccount(account.ID); err != nil {
			failures = append(failures, err)
			continue
		}
		for _, file := range applied {
			if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
				failures = append(failures, err)
			}
		}
	}
	return errors.Join(failures...)
}
