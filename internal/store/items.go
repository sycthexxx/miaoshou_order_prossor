package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func migrateItems(db *sql.DB) error {
	_, e := db.Exec(`CREATE TABLE IF NOT EXISTS orders(id INTEGER PRIMARY KEY AUTOINCREMENT,account_id INTEGER NOT NULL,external_order_id TEXT NOT NULL,order_number TEXT NOT NULL,payload TEXT NOT NULL,captured_at TEXT NOT NULL,UNIQUE(account_id,external_order_id));
 CREATE TABLE IF NOT EXISTS order_items(id INTEGER PRIMARY KEY AUTOINCREMENT,account_id INTEGER NOT NULL,package_id INTEGER NOT NULL REFERENCES packages(id),external_item_id TEXT NOT NULL,platform_sku_id TEXT NOT NULL,title TEXT NOT NULL,spec TEXT NOT NULL,quantity INTEGER NOT NULL,is_gift INTEGER NOT NULL,payload TEXT NOT NULL,UNIQUE(account_id,package_id,external_item_id));`)
	if e != nil {
		return e
	}
	var n int
	if e = db.QueryRow("SELECT count(*) FROM pragma_table_info('imported_archives') WHERE name='parser_version'").Scan(&n); e != nil {
		return e
	}
	if n == 0 {
		_, e = db.Exec("ALTER TABLE imported_archives ADD COLUMN parser_version INTEGER NOT NULL DEFAULT 1")
	}
	return e
}
func saveItems(tx *sql.Tx, account, externalPackage, at string, p object) error {
	var id int64
	var current string
	if e := tx.QueryRow("SELECT id,captured_at FROM packages WHERE account_id=? AND package_id=?", account, externalPackage).Scan(&id, &current); e != nil {
		return e
	}
	if current != at {
		return nil
	}
	info, _ := p["orderInfo"].(map[string]any)
	oid := text(p["opOrderId"])
	if oid == "" {
		oid = text(info["opOrderId"])
	}
	if oid != "" {
		raw, _ := json.Marshal(info)
		if _, e := tx.Exec(`INSERT INTO orders(account_id,external_order_id,order_number,payload,captured_at) VALUES(?,?,?,?,?) ON CONFLICT(account_id,external_order_id) DO UPDATE SET order_number=excluded.order_number,payload=excluded.payload,captured_at=excluded.captured_at WHERE excluded.captured_at>=orders.captured_at`, account, oid, text(info["platformOrderSn"]), string(raw), at); e != nil {
			return e
		}
	}
	ids := []any{account, id}
	marks := []string{}
	for _, item := range objects(p["items"]) {
		iid := text(item["opOrderItemId"])
		if iid == "" {
			return fmt.Errorf("missing order item id")
		}
		q, e := strconv.ParseInt(text(item["quantity"]), 10, 64)
		if e != nil || q < 0 {
			return fmt.Errorf("invalid item quantity")
		}
		gift := 0
		if text(item["isTiktokGift"]) == "1" || text(item["isGift"]) == "1" {
			gift = 1
		}
		raw, _ := json.Marshal(item)
		_, e = tx.Exec(`INSERT INTO order_items(account_id,package_id,external_item_id,platform_sku_id,title,spec,quantity,is_gift,payload) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(account_id,package_id,external_item_id) DO UPDATE SET platform_sku_id=excluded.platform_sku_id,title=excluded.title,spec=excluded.spec,quantity=excluded.quantity,is_gift=excluded.is_gift,payload=excluded.payload`, account, id, iid, text(item["platformSkuId"]), text(item["title"]), text(item["skuSubName"]), q, gift, string(raw))
		if e != nil {
			return e
		}
		ids = append(ids, iid)
		marks = append(marks, "?")
	}
	query := "DELETE FROM order_items WHERE account_id=? AND package_id=?"
	if len(marks) > 0 {
		query += " AND external_item_id NOT IN (" + strings.Join(marks, ",") + ")"
	}
	_, e := tx.Exec(query, ids...)
	return e
}
