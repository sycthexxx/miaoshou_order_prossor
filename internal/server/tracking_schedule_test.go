package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"order_processor/internal/adapters/miaoshou/web/browser"
	"order_processor/internal/store"
	"path/filepath"
	"testing"
	"time"
)

func TestTrackingRoundDeduplicatesAndUpdatesShippedWaybills(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "tracking.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, err := store.Create(db, "tracking")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct{ id, status, number string }{
		{"1", "wait_seller_send", "YT100"}, {"2", "wait_seller_send", "YT100"},
		{"3", "wait_seller_send", "YT200"}, {"4", "wait_receiver_confirm", "YT300"},
	} {
		pkg, _ := json.Marshal(map[string]string{"opOrderPackageId": row.id, "appPackageStatus": row.status,
			"appPackageOperateStatus": "wait_process", "appPurchaseStatus": "1"})
		purchases, _ := json.Marshal([]any{map[string]any{"shipments": []any{map[string]string{
			"number": row.number, "traceId": row.id, "company": "圆通",
		}}}})
		if _, err := db.Exec(`INSERT INTO packages(account_id,package_id,payload,captured_at,source_file) VALUES(?,?,?,'2026','test')`, account, row.id, string(pkg)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO package_purchase_logistics VALUES(?,?,'2026',?)`, account, row.id, string(purchases)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.SaveDomesticTracking(ctx, db, account, "YT200", time.Now(), []store.TrackingStep{
		{Time: "2026-01-01 10:00:00", Description: "已送达"},
	}); err != nil {
		t.Fatal(err)
	}
	calls := make(chan string, 8)
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			send(w, 200, map[string]string{"state": "verified"})
			return
		}
		var query map[string]string
		if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
			send(w, 400, nil)
			return
		}
		calls <- query["number"]
		send(w, 200, map[string]any{"status": 200, "body": `{"number":"YT100","steps":[{"time":"2026-01-01 12:00:00","description":"已送达"}]}`})
	}))
	defer worker.Close()
	endpoint, _ := url.Parse(worker.URL)
	g := &browserGateway{db: db, runtime: fixedRuntime{browser.Target{API: endpoint}}, client: worker.Client()}
	notifications := &orderEventNotifications{}
	g.syncPendingWaybills(ctx, store.Account{ID: account, StorageKey: store.StorageKey(db, account)},
		trackingSchedule{MinSeconds: 1, MaxSeconds: 1}, notifications)
	if len(calls) != 1 || <-calls != "YT100" {
		t.Fatal("expected only one query for the shared, untracked waybill")
	}
	var status string
	var synced int64
	if err := db.QueryRow(`SELECT status,requested_at FROM domestic_tracking WHERE account_id=? AND number='YT100'`, account).
		Scan(&status, &synced); err != nil || status != "已送达" || synced <= 0 || notifications.orderData[account] == "" {
		t.Fatalf("expected stored status, sync time and notification: %s %d %v", status, synced, err)
	}
}

func TestTrackingScheduleDefaultsAndRanges(t *testing.T) {
	settings, err := readTrackingSchedule(filepath.Join(t.TempDir(), trackingSettingsFile))
	if err != nil || !settings.valid() || *settings.Enabled || settings.MinMinutes != 50 || settings.MaxMinutes != 100 {
		t.Fatalf("unexpected defaults: %+v %v", settings, err)
	}
	settings.MinSeconds = settings.MaxSeconds + 1
	if settings.valid() || trackingDelay(3, 3, time.Second) != 3*time.Second {
		t.Fatal("invalid interval accepted")
	}
}
