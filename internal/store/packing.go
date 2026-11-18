package store

import (
	"context"
	"database/sql"
	"errors"
)

type PackingRepository struct{ DB *sql.DB }

func (s PackingRepository) PackageItemIDs(ctx context.Context, account, packageID string) ([]string, error) {
	var payload string
	err := s.DB.QueryRowContext(ctx, "SELECT payload FROM packages WHERE account_id=? AND package_id=?", account, packageID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("当前账户中没有该包裹，请先同步订单")
	}
	if err != nil {
		return nil, err
	}
	var p object
	if err = decode([]byte(payload), &p); err != nil {
		return nil, err
	}
	ids := []string{}
	for _, item := range objects(p["items"]) {
		ids = append(ids, text(item["opOrderItemId"]))
	}
	return ids, nil
}
