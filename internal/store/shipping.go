package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"order_processor/internal/domain/packing"
	"time"
)

func (s PackingRepository) ShippingLabel(ctx context.Context, account, packageID string) (packing.ShippingLabel, error) {
	var raw string
	err := s.DB.QueryRowContext(ctx, `SELECT result_json FROM shipping_labels WHERE account_id=? AND package_id=?`, account, packageID).Scan(&raw)
	var result packing.ShippingLabel
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	err = json.Unmarshal([]byte(raw), &result)
	return result, err
}
func (s PackingRepository) SaveShippingLabel(ctx context.Context, account, packageID string, result packing.ShippingLabel) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO shipping_labels(account_id,package_id,result_json,updated_at) VALUES(?,?,?,?) ON CONFLICT(account_id,package_id) DO UPDATE SET result_json=excluded.result_json,updated_at=excluded.updated_at`, account, packageID, string(raw), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
