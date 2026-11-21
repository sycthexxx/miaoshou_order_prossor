package store

import (
	"order_processor/internal/domain/procurement"
)

func Procurement(db rowsQuerier, account string, gifts bool) (object, error) {
	data, e := Orders(db, account, "", 1, int(^uint(0)>>1))
	if e != nil {
		return nil, e
	}
	report := procurement.Summarize(data["items"].([]object), gifts)
	if e := attachSuppliers(db, account, report); e != nil {
		return nil, e
	}
	if e := attachSourceStatuses(db, account, report); e != nil {
		return nil, e
	}
	unassigned := 0
	for _, group := range objects(report["groups"]) {
		supplier, _ := group["supplier"].(object)
		status, _ := group["sourceStatus"].(SourceStatus)
		if text(group["reason"]) == "" && text(supplier["id"]) == "" && status.State != "invalid" {
			unassigned++
		}
	}
	report["unassignedSources"] = unassigned
	return report, nil
}
