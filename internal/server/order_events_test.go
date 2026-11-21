package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/coder/websocket"
	"order_processor/internal/store"
	"os"
	"path/filepath"
	"testing"
)

func TestEventInboxRetriesPublicationWithoutDuplicateOrdersOrNotifications(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id, err := store.Create(db, "Inbox")
	if err != nil {
		t.Fatal(err)
	}
	inbox := filepath.Join(dir, "runtime", store.StorageKey(db, id), "order-events")
	if err = os.MkdirAll(inbox, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(inbox, "tip.json")
	pkg := map[string]any{"opOrderPackageId": "3", "platformOrderSn": "001", "appPackageStatus": "wait_seller_send", "appPackageOperateStatus": "wait_audit", "appPurchaseStatus": "0", "items": []map[string]any{{"opOrderItemId": "5", "quantity": 1}}}
	raw, err := json.Marshal(map[string]any{"version": 1, "accountId": id, "messageId": "m", "kind": "new_order", "orderNumber": "001", "capturedAt": "2026-11-21T08:00:00.000Z", "packages": []any{pkg}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	cache := &procurementCache{load: func(string, bool) (map[string]any, error) { return nil, errors.New("temporary report failure") }}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	auth := &authentication{db: db, sessions: map[string]loginSession{}}
	notifications := &orderEventNotifications{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/accounts/{id}/order-events/ws", notifications.stream(db, auth))
	handler := auth.protect(mux)
	cookie := adminSession(t, handler)
	server := httptest.NewServer(handler)
	defer server.Close()
	endpoint := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/accounts/" + id + "/order-events/ws"
	options := &websocket.DialOptions{HTTPHeader: http.Header{"Cookie": {cookie.String()}}}
	conn, _, err := websocket.Dial(ctx, endpoint, options)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	read := func(conn *websocket.Conn) (int64, []store.OrderEvent) {
		t.Helper()
		_, raw, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var message struct {
			Cursor int64
			Events []store.OrderEvent
		}
		if err = json.Unmarshal(raw, &message); err != nil {
			t.Fatal(err)
		}
		return message.Cursor, message.Events
	}
	if cursor, events := read(conn); cursor != 0 || len(events) != 0 {
		t.Fatal(cursor, events)
	}

	if err = drainOrderEvents(ctx, db, dir, cache, notifications); err == nil {
		t.Fatal("failed publication should retain the envelope")
	}
	if _, err = os.Stat(file); err != nil {
		t.Fatal(err)
	}
	cache.load = func(account string, _ bool) (map[string]any, error) { return map[string]any{"account": account}, nil }
	ch, unsub, err := cache.subscribe(id, false)
	if err != nil {
		t.Fatal(err)
	}
	defer unsub()
	before := <-ch
	if err = drainOrderEvents(ctx, db, dir, cache, notifications); err != nil {
		t.Fatal(err)
	}
	select {
	case after := <-ch:
		if after["revision"].(int64) <= before["revision"].(int64) {
			t.Fatal("stale report published")
		}
	default:
		t.Fatal("no procurement update")
	}
	if _, err = os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("expected successful envelope consumption", err)
	}
	zero := int64(0)
	batch, _, err := store.OrderEvents(db, id, &zero)
	if err != nil || len(batch) != 1 || batch[0].OrderNo != "001" {
		t.Fatal(batch, err)
	}
	cursor, pushed := read(conn)
	if cursor == 0 || len(pushed) != 1 || pushed[0].OrderNo != "001" {
		t.Fatal(cursor, pushed)
	}
	conn.CloseNow()
	resumed, _, err := websocket.Dial(ctx, endpoint+"?after=0", options)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.CloseNow()
	if next, events := read(resumed); next != cursor || len(events) != 1 {
		t.Fatal(next, events)
	}
	detail, err := store.Orders(db, id, "001", 1, 20, "waitProcess", "waitPurchase")
	if err != nil || detail["total"] != 1 {
		t.Fatal(detail, err)
	}
}
