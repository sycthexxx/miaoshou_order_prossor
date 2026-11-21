package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"order_processor/internal/domain/orders"
)

func saveOrderRecord(ctx context.Context, tx *sql.Tx, account, at, source, origin string, record orders.Record) error {
	id, p := record.ID, record.Fields
	var oldAt string
	err := tx.QueryRowContext(ctx, "SELECT captured_at FROM packages WHERE account_id=? AND package_id=?", account, id).Scan(&oldAt)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if oldAt > at {
		return nil
	}
	payloadFields := make(object, len(p)+2)
	for key, value := range p {
		payloadFields[key] = value
	}
	payloadFields["opOrderPackageId"] = id
	if origin != "" {
		payloadFields["dataSource"] = origin
	}
	payload, err := json.Marshal(payloadFields)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO packages(account_id,package_id,payload,captured_at,source_file) VALUES(?,?,?,?,?) ON CONFLICT(account_id,package_id) DO UPDATE SET payload=excluded.payload,captured_at=excluded.captured_at,source_file=excluded.source_file`, account, id, string(payload), at, source)
	if err != nil {
		return err
	}
	if err = saveItems(tx, account, id, at, payloadFields); err != nil {
		return err
	}
	return nil
}
