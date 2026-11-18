package store

import (
	"database/sql"
	"order_processor/internal/domain/procurement"
)

func Procurement(db *sql.DB, account string, gifts bool) (object, error) {
	data, e := Orders(db, account, "", 1, int(^uint(0)>>1))
	if e != nil {
		return nil, e
	}
	report := procurement.Summarize(data["items"].([]object), gifts)
	if e := attachSuppliers(db, account, report); e != nil {
		return nil, e
	}
	return report, nil
}
