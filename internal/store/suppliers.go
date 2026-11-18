package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func migrateSuppliers(db *sql.DB) error {
	_, e := db.Exec(`CREATE TABLE IF NOT EXISTS suppliers(id INTEGER PRIMARY KEY AUTOINCREMENT,account_id INTEGER NOT NULL,platform TEXT NOT NULL,name TEXT NOT NULL,UNIQUE(account_id,platform,name));
 CREATE TABLE IF NOT EXISTS supplier_evidence(id INTEGER PRIMARY KEY AUTOINCREMENT,account_id INTEGER NOT NULL,package_id TEXT NOT NULL,purchase_id TEXT NOT NULL,supplier_id INTEGER NOT NULL REFERENCES suppliers(id),order_no TEXT NOT NULL,purchase_no TEXT NOT NULL,captured_at TEXT NOT NULL,source_file TEXT NOT NULL,UNIQUE(account_id,package_id,purchase_id));
 CREATE TABLE IF NOT EXISTS source_suppliers(id INTEGER PRIMARY KEY AUTOINCREMENT,account_id INTEGER NOT NULL,source TEXT NOT NULL,site TEXT NOT NULL,product_id TEXT NOT NULL,supplier_id INTEGER NOT NULL REFERENCES suppliers(id),updated_at TEXT NOT NULL,UNIQUE(account_id,source,site,product_id));`)
	return e
}

// Names are the only seller identity returned by this endpoint. Never treat a
// purchase-order number as a seller ID, or infer product ownership from gifts.
func saveOutbound(tx *sql.Tx, account, at, file string, p object) error {
	packageID := text(p["opOrderPackageId"])
	if packageID == "" {
		return fmt.Errorf("missing outbound package id")
	}
	for _, purchase := range objects(p["purchaseOrders"]) {
		name := strings.TrimSpace(text(purchase["purchaseOrderSeller"]))
		platform := text(purchase["purchasePlatform"])
		id := text(purchase["purchaseOrderId"])
		if name == "" {
			continue
		}
		if platform == "" || id == "" {
			return fmt.Errorf("missing purchase identity")
		}
		var previous string
		err := tx.QueryRow(`SELECT captured_at FROM supplier_evidence WHERE account_id=? AND package_id=? AND purchase_id=?`, account, packageID, id).Scan(&previous)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if previous > at {
			continue
		}
		if _, e := tx.Exec(`INSERT INTO suppliers(account_id,platform,name) VALUES(?,?,?) ON CONFLICT DO NOTHING`, account, platform, name); e != nil {
			return e
		}
		var supplierID int64
		if e := tx.QueryRow(`SELECT id FROM suppliers WHERE account_id=? AND platform=? AND name=?`, account, platform, name).Scan(&supplierID); e != nil {
			return e
		}
		_, e := tx.Exec(`INSERT INTO supplier_evidence(account_id,package_id,purchase_id,supplier_id,order_no,purchase_no,captured_at,source_file) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(account_id,package_id,purchase_id) DO UPDATE SET supplier_id=excluded.supplier_id,order_no=excluded.order_no,purchase_no=excluded.purchase_no,captured_at=excluded.captured_at,source_file=excluded.source_file WHERE excluded.captured_at>=supplier_evidence.captured_at`, account, packageID, id, supplierID, text(purchase["platformOrderSn"]), text(purchase["purchaseOrderSn"]), at, file)
		if e != nil {
			return e
		}
	}
	return nil
}

func Suppliers(db *sql.DB, account string) ([]object, error) {
	rows, e := db.Query(`SELECT id,platform,name FROM suppliers WHERE account_id=? ORDER BY name,id`, account)
	if e != nil {
		return nil, e
	}
	result := []object{}
	for rows.Next() {
		var id int64
		var platform, name string
		if e = rows.Scan(&id, &platform, &name); e != nil {
			rows.Close()
			return nil, e
		}
		result = append(result, object{"id": id, "platform": platform, "name": name, "orders": []object{}, "products": []object{}})
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	for _, s := range result {
		r, e := db.Query(`SELECT package_id,order_no,purchase_no,captured_at,source_file FROM supplier_evidence WHERE account_id=? AND supplier_id=? ORDER BY captured_at DESC`, account, s["id"])
		if e != nil {
			return nil, e
		}
		for r.Next() {
			var p, o, n, at, f string
			if e = r.Scan(&p, &o, &n, &at, &f); e != nil {
				r.Close()
				return nil, e
			}
			s["orders"] = append(s["orders"].([]object), object{"packageId": p, "orderNo": o, "purchaseNo": n, "capturedAt": at, "sourceFile": f})
		}
		e = r.Err()
		r.Close()
		if e != nil {
			return nil, e
		}
		r, e = db.Query(`SELECT source,site,product_id FROM source_suppliers WHERE account_id=? AND supplier_id=? ORDER BY product_id`, account, s["id"])
		if e != nil {
			return nil, e
		}
		for r.Next() {
			var source, site, id string
			if e = r.Scan(&source, &site, &id); e != nil {
				r.Close()
				return nil, e
			}
			s["products"] = append(s["products"].([]object), object{"source": source, "site": site, "productId": id})
		}
		e = r.Err()
		r.Close()
		if e != nil {
			return nil, e
		}
	}
	return result, nil
}

func AssignSupplier(db *sql.DB, userID int64, account, source, site, product string, supplier int64) error {
	if source == "" || product == "" || len(source) > 100 || len(site) > 100 || len(product) > 200 || supplier < 0 {
		return fmt.Errorf("货源参数不正确")
	}
	purchases := purchasesFor(db)
	purchases.RLock()
	defer purchases.RUnlock()
	tx, e := db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	// Only allow an actual source from this account's synchronized shop products.
	var valid int
	e = tx.QueryRow(`SELECT 1 FROM shop_products p,json_each(p.payload,'$.sourceList') s WHERE p.account_id=? AND json_extract(s.value,'$.source')=? AND COALESCE(json_extract(s.value,'$.sourceSite'),'')=? AND CAST(json_extract(s.value,'$.sourceItemId') AS TEXT)=? LIMIT 1`, account, source, site, product).Scan(&valid)
	if e != nil {
		return fmt.Errorf("货源不属于当前账户已同步的产品")
	}
	if e = purchases.checkSourceAvailable(tx, account, userID, source, product, &site, supplier); e != nil {
		return e
	}
	if supplier == 0 {
		_, e = tx.Exec(`DELETE FROM source_suppliers WHERE account_id=? AND source=? AND site=? AND product_id=?`, account, source, site, product)
	} else {
		if e = tx.QueryRow(`SELECT 1 FROM suppliers WHERE id=? AND account_id=? AND platform=?`, supplier, account, source).Scan(&valid); e != nil {
			return fmt.Errorf("请选择当前账户相同平台的卖家")
		}
		_, e = tx.Exec(`INSERT INTO source_suppliers(account_id,source,site,product_id,supplier_id,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(account_id,source,site,product_id) DO UPDATE SET supplier_id=excluded.supplier_id,updated_at=excluded.updated_at`, account, source, site, product, supplier, time.Now().UTC().Format(time.RFC3339Nano))
	}
	if e != nil {
		return e
	}
	return tx.Commit()
}

func attachSuppliers(db *sql.DB, account string, report object) error {
	rows, e := db.Query(`SELECT m.source,m.site,m.product_id,s.id,s.name,s.platform FROM source_suppliers m JOIN suppliers s ON s.id=m.supplier_id AND s.account_id=m.account_id WHERE m.account_id=?`, account)
	if e != nil {
		return e
	}
	defer rows.Close()
	bySource := map[string]object{}
	for rows.Next() {
		var source, site, product, name, platform string
		var id int64
		if e = rows.Scan(&source, &site, &product, &id, &name, &platform); e != nil {
			return e
		}
		key, _ := json.Marshal([]string{source, site, product})
		bySource["source:"+string(key)] = object{"id": id, "name": name, "platform": platform}
	}
	if e = rows.Err(); e != nil {
		return e
	}
	for _, g := range report["groups"].([]object) {
		g["supplier"] = bySource[text(g["key"])]
	}
	return nil
}
