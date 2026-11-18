package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"order_processor/internal/domain/orders"
	"time"
)

// ImportBatch commits a verified, normalized batch atomically, regardless of its source.
func ImportBatch(ctx context.Context, db *sql.DB, account string, batch orders.Batch) (orders.ImportResult, error) {
	var result orders.ImportResult
	at, source := batch.CapturedAt, batch.Archive
	started, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return result, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRowContext(ctx, "SELECT 1 FROM accounts WHERE id=? AND deleted=0", account).Scan(&exists); err != nil {
		return result, err
	}
	if batch.Policy.RequireUnchangedAccount {
		// Compare real timestamps, including millisecond browser timestamps. Do not
		// overwrite a browser capture that completed while the API was downloading.
		rows, err := tx.QueryContext(ctx, `SELECT captured_at FROM packages WHERE account_id=? UNION ALL SELECT captured_at FROM order_scope_snapshots WHERE account_id=?`, account, account)
		if err != nil {
			return result, err
		}
		for rows.Next() {
			var v string
			if err = rows.Scan(&v); err != nil {
				rows.Close()
				return result, err
			}
			t, e := time.Parse(time.RFC3339Nano, v)
			if e != nil || !t.Before(started) {
				rows.Close()
				return result, fmt.Errorf("同步期间本地订单已更新，请等待网页采集完成后重试")
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return result, err
		}

	}
	for _, record := range batch.Records {
		missing, supplemented, err := saveOrderRecord(ctx, tx, account, at, source, batch.Source, record, batch.Policy)
		if err != nil {
			return result, err
		}
		result.MissingRefundLines += missing
		result.SupplementedLines += supplemented
	}

	for _, scope := range batch.Scopes {
		if err = saveOrderScope(tx, account, scope.Name, at, []string{source}, scope.PackageIDs); err != nil {
			return result, err
		}
		if scope.Name == "waitPurchase" {
			result.WaitPurchase = len(scope.PackageIDs)
		}
		if scope.Name == "hasPurchase" {
			result.HasPurchase = len(scope.PackageIDs)
		}
	}
	result.Packages = len(batch.Records)
	if err = tx.Commit(); err != nil {
		return orders.ImportResult{}, err
	}
	return result, nil
}

func saveOrderRecord(ctx context.Context, tx *sql.Tx, account, at, source, origin string, record orders.Record, policy orders.ImportPolicy) (int, int, error) {
	id, p := record.ID, record.Fields
	var oldRaw, oldAt string
	err := tx.QueryRowContext(ctx, "SELECT payload,captured_at FROM packages WHERE account_id=? AND package_id=?", account, id).Scan(&oldRaw, &oldAt)
	old := object{}
	if err != nil && err != sql.ErrNoRows {
		return 0, 0, err
	}
	if oldRaw != "" {
		if err = decode([]byte(oldRaw), &old); err != nil {
			return 0, 0, err
		}
	}
	if oldAt > at {
		return 0, 0, nil
	}
	merged, missing, supplemented, err := orders.Merge(old, p, oldAt, policy)
	if err != nil {
		return 0, 0, err
	}
	merged["opOrderPackageId"] = id
	if origin != "" {
		merged["dataSource"] = origin
	}
	payload, err := json.Marshal(merged)
	if err != nil {
		return 0, 0, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO packages(account_id,package_id,payload,captured_at,source_file) VALUES(?,?,?,?,?) ON CONFLICT(account_id,package_id) DO UPDATE SET payload=excluded.payload,captured_at=excluded.captured_at,source_file=excluded.source_file`, account, id, string(payload), at, source)
	if err != nil {
		return 0, 0, err
	}
	if err = saveItems(tx, account, id, at, merged); err != nil {
		return 0, 0, err
	}
	return missing, supplemented, nil

}

// OrderRepository implements the application's order submission port.
type OrderRepository struct{ DB *sql.DB }

func (r OrderRepository) ImportBatch(ctx context.Context, account string, batch orders.Batch) (orders.ImportResult, error) {
	return ImportBatch(ctx, r.DB, account, batch)
}
