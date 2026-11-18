package store

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var offerNumber = regexp.MustCompile(`^[0-9]{1,40}$`)

// CaptureSupplier preserves an association only when its supplier name matches.
// Different names reassign this product without renaming or deleting old suppliers.
func CaptureSupplier(db *sql.DB, account, product, name string) (object, error) {
	name = strings.TrimSpace(name)
	if !offerNumber.MatchString(product) || name == "" || len([]rune(name)) > 120 {
		return nil, fmt.Errorf("商品ID或厂家名称不正确")
	}
	tx, e := db.Begin()
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	rows, e := tx.Query(`SELECT DISTINCT COALESCE(json_extract(s.value,'$.sourceSite'),'') FROM shop_products p,json_each(p.payload,'$.sourceList') s WHERE p.account_id=? AND json_extract(s.value,'$.source')='1688' AND CAST(json_extract(s.value,'$.sourceItemId') AS TEXT)=? ORDER BY 1`, account, product)
	if e != nil {
		return nil, e
	}
	sites := []string{}
	for rows.Next() {
		var site string
		if e = rows.Scan(&site); e != nil {
			rows.Close()
			return nil, e
		}
		sites = append(sites, site)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	if len(sites) == 0 {
		return nil, fmt.Errorf("该1688商品不属于当前账户已同步的货源")
	}
	missing, changed := []string{}, []string{}
	retained := 0
	var supplier int64
	for _, site := range sites {
		var id int64
		var existingName string
		e = tx.QueryRow(`SELECT m.supplier_id,s.name FROM source_suppliers m JOIN suppliers s ON s.id=m.supplier_id AND s.account_id=m.account_id WHERE m.account_id=? AND m.source='1688' AND m.site=? AND m.product_id=?`, account, site, product).Scan(&id, &existingName)
		if e == sql.ErrNoRows {
			missing = append(missing, site)
		} else if e != nil {
			return nil, e
		} else if strings.TrimSpace(existingName) == name {
			retained++
			if supplier == 0 {
				supplier = id
			}
		} else {
			changed = append(changed, site)
		}
	}
	result := object{"productId": product, "assigned": int64(0), "updated": int64(0), "retained": retained, "created": false, "supplierName": name}
	if len(missing)+len(changed) == 0 {
		if e = tx.Commit(); e != nil {
			return nil, e
		}
		result["supplierId"] = supplier
		return result, nil
	}
	// Reuse a matching current-account supplier, including legacy whitespace in
	// its display name. Do not rename records shared by other source products.
	if supplier == 0 {
		rows, e = tx.Query(`SELECT id,name FROM suppliers WHERE account_id=? AND platform='1688' ORDER BY id`, account)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var id int64
			var candidate string
			if e = rows.Scan(&id, &candidate); e != nil {
				rows.Close()
				return nil, e
			}
			if strings.TrimSpace(candidate) == name && supplier == 0 {
				supplier = id
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
	}
	if supplier == 0 {
		res, err := tx.Exec(`INSERT INTO suppliers(account_id,platform,name) VALUES(?,'1688',?) ON CONFLICT(account_id,platform,name) DO NOTHING`, account, name)
		if err != nil {
			return nil, err
		}
		created, err := res.RowsAffected()
		if err != nil {
			return nil, err
		}
		result["created"] = created > 0
		if e = tx.QueryRow(`SELECT id FROM suppliers WHERE account_id=? AND platform='1688' AND name=?`, account, name).Scan(&supplier); e != nil {
			return nil, e
		}
	}
	assigned, updated := int64(0), int64(0)
	at := time.Now().UTC().Format(time.RFC3339Nano)
	for _, site := range missing {
		res, err := tx.Exec(`INSERT INTO source_suppliers(account_id,source,site,product_id,supplier_id,updated_at) VALUES(?,'1688',?,?,?,?)`, account, site, product, supplier, at)
		if err != nil {
			return nil, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return nil, err
		}
		assigned += n
	}
	for _, site := range changed {
		res, err := tx.Exec(`UPDATE source_suppliers SET supplier_id=?,updated_at=? WHERE account_id=? AND source='1688' AND site=? AND product_id=?`, supplier, at, account, site, product)
		if err != nil {
			return nil, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return nil, err
		}
		updated += n
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	result["assigned"] = assigned
	result["updated"] = updated
	result["supplierId"] = supplier
	return result, nil
}
