package store

import (
	"database/sql"
	"encoding/json"
	"order_processor/internal/domain/orders"
)

// Scope snapshots describe membership, not the lifecycle payload. Historical
// packages remain available even when they leave a completed source view.
func migrateOrderSnapshots(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS order_scope_snapshots(id INTEGER PRIMARY KEY AUTOINCREMENT,account_id INTEGER NOT NULL,scope TEXT NOT NULL,captured_at TEXT NOT NULL,source_files TEXT NOT NULL,package_ids TEXT NOT NULL,UNIQUE(account_id,scope))`)
	return err
}

func saveOrderScope(tx *sql.Tx, account, scope, at string, files, ids []string) error {
	f, _ := json.Marshal(files)
	b, _ := json.Marshal(ids)
	_, err := tx.Exec(`INSERT INTO order_scope_snapshots(account_id,scope,captured_at,source_files,package_ids) VALUES(?,?,?,?,?) ON CONFLICT(account_id,scope) DO UPDATE SET captured_at=excluded.captured_at,source_files=excluded.source_files,package_ids=excluded.package_ids WHERE excluded.captured_at>order_scope_snapshots.captured_at`, account, scope, at, string(f), string(b))
	return err
}

type orderScopes = orders.Scopes

func loadOrderScopes(db rowsQuerier, account string) (orderScopes, error) {
	rows, err := db.Query(`SELECT scope,package_ids,captured_at FROM order_scope_snapshots WHERE account_id=?`, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := orderScopes{}
	times := map[string]string{}
	for rows.Next() {
		var scope, raw, at string
		if err = rows.Scan(&scope, &raw, &at); err != nil {
			return nil, err
		}
		var ids []string
		if err = json.Unmarshal([]byte(raw), &ids); err != nil {
			return nil, err
		}
		times[scope] = at
		out[scope] = map[string]bool{}
		for _, id := range ids {
			out[scope][id] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := applyEventMembership(db, account, out, times); err != nil {
		return nil, err
	}
	// An order can move between purchase views between two captures.
	for id := range out["waitPurchase"] {
		if out["hasPurchase"][id] {
			if times["waitPurchase"] > times["hasPurchase"] {
				delete(out["hasPurchase"], id)
			} else {
				delete(out["waitPurchase"], id)
			}
		}
	}
	return out, rows.Err()
}
