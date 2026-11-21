package store

import (
	"context"
	"encoding/json"
	"order_processor/internal/domain/orders"
	"path/filepath"
	"testing"
)

func TestSaveOrderRecordUsesCanonicalRecordID(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, err := Create(db, "account")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	record := orders.Record{ID: "package-1", Fields: orders.Fields{"items": []any{map[string]any{"opOrderItemId": "item-1", "quantity": 1}}}}
	if err = saveOrderRecord(context.Background(), tx, account, "2026-01-01T00:00:00Z", "capture.json", "web", record); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err = tx.QueryRow("SELECT payload FROM packages WHERE account_id=? AND package_id=?", account, record.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var payload orders.Fields
	if err = json.Unmarshal([]byte(raw), &payload); err != nil || payload["opOrderPackageId"] != record.ID {
		t.Fatalf("canonical package ID: %v, %v", payload, err)
	}
}
