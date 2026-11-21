package store

import (
	"database/sql"
	"encoding/json"
	"order_processor/internal/domain/orders"
	"strings"
)

type object = orders.Fields

func text(v any) string            { return orders.Text(v) }
func objects(v any) []object       { return orders.Objects(v) }
func decode(b []byte, v any) error { return orders.Decode(b, v) }
func migrateOrders(db *sql.DB) error {
	for _, table := range []string{"packages", "translations", "imported_archives"} {
		var exists, hasID int
		db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&exists)
		if exists == 0 {
			continue
		}
		db.QueryRow("SELECT count(*) FROM pragma_table_info(?) WHERE name='id'", table).Scan(&hasID)
		if hasID > 0 {
			continue
		}
		tx, e := db.Begin()
		if e != nil {
			return e
		}
		var ddl, columns string
		switch table {
		case "packages":
			ddl = "account_id INTEGER NOT NULL,package_id TEXT NOT NULL,payload TEXT NOT NULL,captured_at TEXT NOT NULL,source_file TEXT NOT NULL,UNIQUE(account_id,package_id)"
			columns = "account_id,package_id,payload,captured_at,source_file"
		case "translations":
			ddl = "account_id INTEGER NOT NULL,package_id TEXT NOT NULL,item_id TEXT NOT NULL,payload TEXT NOT NULL,captured_at TEXT NOT NULL,UNIQUE(account_id,package_id,item_id)"
			columns = "account_id,package_id,item_id,payload,captured_at"
		case "imported_archives":
			ddl = "account_id INTEGER NOT NULL,file TEXT NOT NULL,UNIQUE(account_id,file)"
			columns = "account_id,file"
		}
		_, e = tx.Exec("ALTER TABLE " + table + " RENAME TO " + table + "_legacy; CREATE TABLE " + table + "(id INTEGER PRIMARY KEY AUTOINCREMENT," + ddl + "); INSERT INTO " + table + "(" + columns + ") SELECT " + columns + " FROM " + table + "_legacy; DROP TABLE " + table + "_legacy;")
		if e != nil {
			tx.Rollback()
			return e
		}
		if e = tx.Commit(); e != nil {
			return e
		}
	}

	_, e := db.Exec(`
CREATE TABLE IF NOT EXISTS packages(id INTEGER PRIMARY KEY AUTOINCREMENT,account_id INTEGER NOT NULL,package_id TEXT NOT NULL,payload TEXT NOT NULL,captured_at TEXT NOT NULL,source_file TEXT NOT NULL,UNIQUE(account_id,package_id));
CREATE TABLE IF NOT EXISTS translations(id INTEGER PRIMARY KEY AUTOINCREMENT,account_id INTEGER NOT NULL,package_id TEXT NOT NULL,item_id TEXT NOT NULL,payload TEXT NOT NULL,captured_at TEXT NOT NULL,UNIQUE(account_id,package_id,item_id));
CREATE TABLE IF NOT EXISTS imported_archives(id INTEGER PRIMARY KEY AUTOINCREMENT,account_id INTEGER NOT NULL,file TEXT NOT NULL,UNIQUE(account_id,file));
`)
	return e
}

func Orders(db rowsQuerier, account, query string, page, size int, statuses ...string) (object, error) {
	// A bounded page is returned; source snapshots stay in SQLite for reprocessing.
	rows, e := db.Query("SELECT id,package_id,payload,captured_at,source_file FROM packages WHERE account_id=? ORDER BY captured_at DESC,package_id DESC", account)
	if e != nil {
		return nil, e
	}
	all := []object{}
	for rows.Next() {
		var id, payload, at, file string
		var localID int64
		if e = rows.Scan(&localID, &id, &payload, &at, &file); e != nil {
			rows.Close()
			return nil, e
		}
		var p object
		if e = decode([]byte(payload), &p); e != nil {
			rows.Close()
			return nil, e
		}
		items := objects(p["items"])
		p["id"] = localID
		p["items"] = items
		p["capturedAt"] = at
		p["sourceFile"] = file
		all = append(all, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	tr, e := db.Query("SELECT package_id,item_id,payload FROM translations WHERE account_id=?", account)
	if e != nil {
		return nil, e
	}
	translations := map[string]object{}
	for tr.Next() {
		var pid, iid, payload string
		if e = tr.Scan(&pid, &iid, &payload); e != nil {
			tr.Close()
			return nil, e
		}
		var t object
		if e = decode([]byte(payload), &t); e != nil {
			tr.Close()
			return nil, e
		}
		translations[pid+"/"+iid] = t
	}
	e = tr.Err()
	tr.Close()
	if e != nil {
		return nil, e
	}
	products, e := productMap(db, account)
	if e != nil {
		return nil, e
	}
	scopes, e := loadOrderScopes(db, account)
	if e != nil {
		return nil, e
	}
	filtered := []object{}
	status := "all"
	if len(statuses) > 0 && statuses[0] != "" {
		status = statuses[0]
	}
	purchase := ""
	if len(statuses) > 1 && status == "waitProcess" {
		purchase = statuses[1]
	}
	counts := map[string]int{"waitPurchase": 0, "hasPurchase": 0, "all": 0, "waitProcess": 0, "waitShip": 0, "submitPlatform": 0, "waitReceiverConfirm": 0, "isolation": 0, "closed": 0}
	for _, p := range all {
		matches := false
		tabs, purchaseScope := scopes.Membership(p)
		p["procurementEligible"] = purchaseScope == "waitPurchase"
		for _, tab := range tabs {
			counts[tab]++
			if tab == "waitProcess" && purchaseScope != "" {
				counts[purchaseScope]++
			}
			if tab == status {
				matches = true
			}
		}
		if matches && status == "waitProcess" && purchase != "" {
			matches = purchaseScope == purchase
		}
		if !matches {
			continue
		}
		for _, item := range p["items"].([]object) {
			platform, shop := text(item["platform"]), text(item["shopId"])
			if platform == "" {
				platform = text(p["platform"])
			}
			if shop == "" {
				shop = text(p["shopId"])
			}
			product := products[productKey{platform, shop, text(item["platformItemId"])}]
			item["sourceState"] = sourceState(product)
			if product != nil {
				item["productLocalId"] = product["id"]
				item["sourceList"] = product["sourceList"]
			}
			if t := translations[text(p["opOrderPackageId"])+"/"+text(item["opOrderItemId"])]; t != nil {
				item["skuSubCnName"] = t["skuSubCnName"]
				item["cnTitle"] = t["cnTitle"]
			}
		}
		b, _ := json.Marshal(p)
		if query == "" || strings.Contains(strings.ToLower(string(b)), strings.ToLower(query)) {
			filtered = append(filtered, p)
		}
	}
	total := len(filtered)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	return object{"items": filtered[start:end], "total": total, "page": page, "pageSize": size, "counts": counts, "status": status, "purchaseTab": purchase}, nil
}
