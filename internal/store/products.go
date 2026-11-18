package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

func migrateProducts(db *sql.DB) error {
	_, e := db.Exec(`CREATE TABLE IF NOT EXISTS shop_products(id INTEGER PRIMARY KEY AUTOINCREMENT,account_id INTEGER NOT NULL REFERENCES accounts(id),platform TEXT NOT NULL,shop_id TEXT NOT NULL,platform_item_id TEXT NOT NULL,payload TEXT NOT NULL,captured_at TEXT NOT NULL,source_file TEXT NOT NULL,UNIQUE(account_id,platform,shop_id,platform_item_id));`)
	return e
}
func saveProduct(tx *sql.Tx, account, at, file string, p object) error {
	platform, shop, id := text(p["platform"]), text(p["shopId"]), text(p["platformItemId"])
	if platform == "" || shop == "" || id == "" {
		return fmt.Errorf("product identity missing")
	}
	// Missing sourceList is unknown, whereas an explicit empty array means no source.
	if v, ok := p["sourceList"]; ok && v != nil {
		if _, ok := v.([]any); !ok {
			return fmt.Errorf("invalid sourceList")
		}
	}
	raw, e := json.Marshal(p)
	if e != nil {
		return e
	}
	_, e = tx.Exec(`INSERT INTO shop_products(account_id,platform,shop_id,platform_item_id,payload,captured_at,source_file) VALUES(?,?,?,?,?,?,?) ON CONFLICT(account_id,platform,shop_id,platform_item_id) DO UPDATE SET payload=excluded.payload,captured_at=excluded.captured_at,source_file=excluded.source_file WHERE excluded.captured_at>=shop_products.captured_at`, account, platform, shop, id, string(raw), at, file)
	return e
}

type productKey struct{ platform, shop, id string }

func productMap(db *sql.DB, account string) (map[productKey]object, error) {
	rows, e := db.Query(`SELECT id,platform,shop_id,platform_item_id,payload,captured_at FROM shop_products WHERE account_id=? ORDER BY id`, account)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[productKey]object{}
	for rows.Next() {
		var id, platform, shop, pid, raw, at string
		if e = rows.Scan(&id, &platform, &shop, &pid, &raw, &at); e != nil {
			return nil, e
		}
		var p object
		if e = decode([]byte(raw), &p); e != nil {
			return nil, e
		}
		p["id"] = id
		p["capturedAt"] = at
		out[productKey{platform, shop, pid}] = p
	}
	return out, rows.Err()
}
func sourceState(p object) string {
	if p == nil {
		return "not_synced"
	}
	v, ok := p["sourceList"].([]any)
	if !ok {
		return "unknown"
	}
	if len(v) == 0 {
		return "empty"
	}
	return "available"
}
func Products(db *sql.DB, account, q string, page int, statuses ...string) (object, error) {
	rows, e := db.Query(`SELECT id,payload,captured_at FROM shop_products WHERE account_id=? ORDER BY id DESC`, account)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	items := []object{}
	counts := map[string]int{"onsale": 0, "forsale": 0, "suspendSales": 0, "draft": 0, "deleted": 0}
	status := ""
	if len(statuses) > 0 {
		status = statuses[0]
	}
	total := 0
	for rows.Next() {
		var id, raw, at string
		if e = rows.Scan(&id, &raw, &at); e != nil {
			return nil, e
		}
		var p object
		if e = decode([]byte(raw), &p); e != nil {
			return nil, e
		}
		st := text(p["status"])
		counts[st]++
		if status != "" && st != status {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(raw), strings.ToLower(q)) {
			continue
		}
		total++
		if total <= (page-1)*20 || total > page*20 {
			continue
		}
		p["id"] = id
		p["capturedAt"] = at
		p["sourceState"] = sourceState(p)
		items = append(items, p)
	}
	return object{"items": items, "total": total, "counts": counts}, rows.Err()
}
