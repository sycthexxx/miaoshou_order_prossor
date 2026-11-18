package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrProcurementMappingInput = errors.New("采购映射参数不正确")
var ErrProcurementMappingSource = errors.New("该1688商品不属于当前账户已同步的货源")
var ErrProcurementMappingConflict = errors.New("采购映射已被其他页面更新，请重新读取后再保存")

// Shared purchasing rules only. Browser DOM selectors are deliberately absent.
type ProcurementSKUMapping struct {
	Tail        []string `json:"tail,omitempty"`
	Exception   string   `json:"exception,omitempty"`
	LegacyAlias string   `json:"legacyAlias,omitempty"`
}
type ProcurementMapping struct {
	Version int                              `json:"version"`
	Colors  map[string]string                `json:"colors"`
	SKUs    map[string]ProcurementSKUMapping `json:"skus"`
}
type ProcurementMappingRecord struct {
	Revision int64              `json:"revision"`
	Mapping  ProcurementMapping `json:"mapping"`
}

func emptyProcurementMapping() ProcurementMapping {
	return ProcurementMapping{Version: 2, Colors: map[string]string{}, SKUs: map[string]ProcurementSKUMapping{}}
}
func migrateProcurementMappings(db *sql.DB) error {
	_, e := db.Exec(`CREATE TABLE IF NOT EXISTS procurement_mappings(
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 account_id INTEGER NOT NULL REFERENCES accounts(id),
 source TEXT NOT NULL DEFAULT '1688',
 product_id TEXT NOT NULL,
 revision INTEGER NOT NULL CHECK(revision > 0),
 mapping TEXT NOT NULL,
 updated_at TEXT NOT NULL,
 UNIQUE(account_id,source,product_id));`)
	return e
}
func validateProcurementMapping(mapping *ProcurementMapping) error {
	if mapping.Version != 2 || len(mapping.Colors) > 500 || len(mapping.SKUs) > 1000 {
		return fmt.Errorf("%w：版本必须为2，颜色最多500条，SKU最多1000条", ErrProcurementMappingInput)
	}
	valid := func(value string, limit int) bool {
		return strings.TrimSpace(value) != "" && len([]rune(value)) <= limit
	}
	for key, value := range mapping.Colors {
		if !valid(key, 200) || !valid(value, 200) {
			return fmt.Errorf("%w：颜色名称不能为空且最多200字", ErrProcurementMappingInput)
		}
	}
	for spec, rule := range mapping.SKUs {
		if !valid(spec, 300) || len(rule.Tail) > 10 || len([]rune(rule.Exception)) > 300 || len([]rune(rule.LegacyAlias)) > 300 {
			return fmt.Errorf("%w：SKU名称/别名最多300字，规格维度最多10个", ErrProcurementMappingInput)
		}
		if (rule.Exception != "" && !valid(rule.Exception, 300)) || (rule.LegacyAlias != "" && !valid(rule.LegacyAlias, 300)) {
			return fmt.Errorf("%w：别名不能只含空白", ErrProcurementMappingInput)
		}
		for _, part := range rule.Tail {
			if !valid(part, 200) {
				return fmt.Errorf("%w：规格维度不能为空且最多200字", ErrProcurementMappingInput)
			}
		}
	}
	if mapping.Colors == nil {
		mapping.Colors = map[string]string{}
	}
	if mapping.SKUs == nil {
		mapping.SKUs = map[string]ProcurementSKUMapping{}
	}
	return nil
}

type mappingQuerier interface{ QueryRow(string, ...any) *sql.Row }

func requireProcurementSource(q mappingQuerier, account, product string) error {
	if !offerNumber.MatchString(product) {
		return ErrProcurementMappingInput
	}
	var valid int
	e := q.QueryRow(`SELECT 1 FROM shop_products p,json_each(p.payload,'$.sourceList') s WHERE p.account_id=? AND json_extract(s.value,'$.source')='1688' AND CAST(json_extract(s.value,'$.sourceItemId') AS TEXT)=? LIMIT 1`, account, product).Scan(&valid)
	if e == sql.ErrNoRows {
		return ErrProcurementMappingSource
	}
	return e
}
func readProcurementMapping(q mappingQuerier, account, product string) (ProcurementMappingRecord, error) {
	result := ProcurementMappingRecord{Mapping: emptyProcurementMapping()}
	var raw string
	e := q.QueryRow(`SELECT revision,mapping FROM procurement_mappings WHERE account_id=? AND source='1688' AND product_id=?`, account, product).Scan(&result.Revision, &raw)
	if e == sql.ErrNoRows {
		return result, nil
	}
	if e != nil {
		return result, e
	}
	if e = json.Unmarshal([]byte(raw), &result.Mapping); e != nil {
		return result, e
	}
	return result, validateProcurementMapping(&result.Mapping)
}
func GetProcurementMapping(db *sql.DB, account, product string) (ProcurementMappingRecord, error) {
	if e := requireProcurementSource(db, account, product); e != nil {
		return ProcurementMappingRecord{}, e
	}
	return readProcurementMapping(db, account, product)
}
func SaveProcurementMapping(db *sql.DB, userID int64, account, product string, expectedRevision int64, mapping ProcurementMapping) (ProcurementMappingRecord, error) {
	if expectedRevision < 0 || expectedRevision >= 9007199254740991 {
		return ProcurementMappingRecord{}, ErrProcurementMappingInput
	}
	if e := validateProcurementMapping(&mapping); e != nil {
		return ProcurementMappingRecord{}, e
	}
	purchases := purchasesFor(db)
	purchases.RLock()
	defer purchases.RUnlock()
	tx, e := db.Begin()
	if e != nil {
		return ProcurementMappingRecord{}, e
	}
	defer tx.Rollback()
	if e = requireProcurementSource(tx, account, product); e != nil {
		return ProcurementMappingRecord{}, e
	}
	if e = purchases.checkSourceAvailable(tx, account, userID, "1688", product, nil, -1); e != nil {
		return ProcurementMappingRecord{}, e
	}
	current, e := readProcurementMapping(tx, account, product)
	if e != nil {
		return current, e
	}
	if current.Revision != expectedRevision {
		return current, ErrProcurementMappingConflict
	}
	raw, e := json.Marshal(mapping)
	if e != nil {
		return current, e
	}
	at := time.Now().UTC().Format(time.RFC3339Nano)
	if expectedRevision == 0 {
		_, e = tx.Exec(`INSERT INTO procurement_mappings(account_id,source,product_id,revision,mapping,updated_at) VALUES(?,'1688',?,1,?,?)`, account, product, string(raw), at)
	} else {
		_, e = tx.Exec(`UPDATE procurement_mappings SET revision=revision+1,mapping=?,updated_at=? WHERE account_id=? AND source='1688' AND product_id=? AND revision=?`, string(raw), at, account, product, expectedRevision)
	}
	if e != nil {
		return current, e
	}
	if e = tx.Commit(); e != nil {
		return current, e
	}
	return ProcurementMappingRecord{Revision: expectedRevision + 1, Mapping: mapping}, nil
}
