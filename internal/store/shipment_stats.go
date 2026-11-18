package store

import (
	"database/sql"
	"encoding/json"
)

type ShipmentSKU struct {
	Daily          map[string]int64 `json:"daily"`
	PicURLThumb    string           `json:"picUrlThumb,omitempty"`
	PicURL         string           `json:"picUrl,omitempty"`
	OriginalPicURL string           `json:"originalPicUrl,omitempty"`
	SKU            string           `json:"sku"`
	Title          string           `json:"title"`
	Spec           string           `json:"spec"`
	Shop           string           `json:"shop"`
	Quantity       int64            `json:"quantity"`
}
type ShipmentSupplier struct {
	ID       int64         `json:"id"`
	Name     string        `json:"name"`
	Quantity int64         `json:"quantity"`
	SKUs     []ShipmentSKU `json:"skus"`
}

// Collapse product-to-source mappings before joining order lines. Multiple candidate
// sources identify a factory only when every candidate maps to the same factory.
func ShipmentStats(db *sql.DB, account, start, end string) ([]ShipmentSupplier, error) {
	rows, err := db.Query(`
	WITH factories AS (
	 SELECT sp.id, CASE WHEN count(*)=count(m.supplier_id) AND min(m.supplier_id)=max(m.supplier_id) THEN min(m.supplier_id) END supplier_id
	 FROM shop_products sp JOIN json_each(sp.payload,'$.sourceList') src
	 LEFT JOIN source_suppliers m ON m.account_id=sp.account_id
	  AND m.source=src.value->>'$.source' AND m.site=coalesce(src.value->>'$.sourceSite','')
	  AND m.product_id=cast(src.value->>'$.sourceItemId' AS TEXT)
	 WHERE sp.account_id=? GROUP BY sp.id
	), lines AS (
	 SELECT i.*, coalesce(nullif(i.payload->>'$.shopId',''),p.payload->>'$.shopId','') shop,
	  coalesce(nullif(i.payload->>'$.platform',''),p.payload->>'$.platform','') platform,
	  coalesce(nullif(substr(p.payload->>'$.orderInfo.gmtOrderStart',1,10),''),'日期未提供') day, i.payload->>'$.platformItemId' product, p.package_id external_package
	 FROM packages p JOIN order_items i ON i.package_id=p.id AND i.account_id=p.account_id
	 WHERE p.account_id=? AND i.quantity>0
	  AND (?='' OR substr(p.payload->>'$.orderInfo.gmtOrderStart',1,10) BETWEEN ? AND ?)
	), daily AS (
	 SELECT coalesce(s.id,0) supplier, coalesce(s.name,'未指定厂家') name, l.platform_sku_id sku,l.platform,l.day,
      CASE WHEN l.platform_sku_id='' THEN coalesce(nullif(l.product,''),l.title) END fallback_product,
      CASE WHEN l.platform_sku_id='' THEN l.spec END fallback_spec,
	  max(l.title) title,coalesce(max(nullif(t.payload->>'$.skuSubCnName','')),max(l.spec)) spec,l.shop,
	  coalesce(max(l.payload->>'$.picUrlThumb'),'') thumb,
	  coalesce(max(l.payload->>'$.picUrl'),'') pic,
	  coalesce(max(l.payload->>'$.originalPicUrl'),'') original,
	  sum(l.quantity) qty
	 FROM lines l LEFT JOIN shop_products sp ON sp.account_id=l.account_id AND sp.platform=l.platform AND sp.shop_id=l.shop AND sp.platform_item_id=l.product
	 LEFT JOIN factories f ON f.id=sp.id LEFT JOIN suppliers s ON s.id=f.supplier_id AND s.account_id=l.account_id
	 LEFT JOIN translations t ON t.account_id=l.account_id AND t.package_id=l.external_package AND t.item_id=l.external_item_id
	 GROUP BY s.id,l.platform,l.shop,l.platform_sku_id,fallback_product,fallback_spec,l.day
    ), totals AS (
      SELECT supplier,name,sku,max(title) title,max(spec) spec,shop,max(thumb) thumb,max(pic) pic,max(original) original,
       sum(qty) qty,json_group_object(day,qty) daily_counts
      FROM daily GROUP BY supplier,platform,shop,sku,fallback_product,fallback_spec
	)
	SELECT supplier,name,sku,title,spec,shop,thumb,pic,original,qty,daily_counts,sum(qty) OVER(PARTITION BY supplier) total
	FROM totals ORDER BY total DESC,name,supplier,qty DESC,sku,shop,title,spec`, account, account, start, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ShipmentSupplier{}
	for rows.Next() {
		var supplier ShipmentSupplier
		var sku ShipmentSKU
		var counts []byte
		if err := rows.Scan(&supplier.ID, &supplier.Name, &sku.SKU, &sku.Title, &sku.Spec, &sku.Shop, &sku.PicURLThumb, &sku.PicURL, &sku.OriginalPicURL, &sku.Quantity, &counts, &supplier.Quantity); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(counts, &sku.Daily); err != nil {
			return nil, err
		}
		if len(out) == 0 || out[len(out)-1].ID != supplier.ID {
			out = append(out, supplier)
		}
		group := &out[len(out)-1]
		group.SKUs = append(group.SKUs, sku)
	}
	return out, rows.Err()
}
