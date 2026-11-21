package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"order_processor/internal/store"
)

func serveAccountStream[T any](w http.ResponseWriter, r *http.Request, auth *authentication, updates <-chan T, push func(T, func(any) error) error) {
	account := r.PathValue("id")
	permitted := func() bool {
		user, ok := auth.user(r)
		if !ok {
			return false
		}
		if user.IsAdmin {
			return store.Exists(auth.db, account)
		}
		return auth.allowed(r, r.Pattern, account, user.ID)
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"localhost:5173", "127.0.0.1:5173"}})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx := conn.CloseRead(r.Context())
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	write := func(message any) error {
		if !permitted() {
			return websocket.CloseError{Code: websocket.StatusPolicyViolation}
		}
		payload, err := json.Marshal(message)
		if err != nil {
			return err
		}
		writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return conn.Write(writeCtx, websocket.MessageText, payload)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !permitted() {
				return
			}
			pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				return
			}
		case update, ok := <-updates:
			if !ok || push(update, write) != nil {
				return
			}
		}
	}
}
