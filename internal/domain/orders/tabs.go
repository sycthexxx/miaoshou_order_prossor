package orders

// Tabs follow Miaoshou's processing view. After-sales may overlap lifecycle
// states: even a finished package can contain an item marked isRefund=1.
func Tabs(p Fields) []string {
	tabs := []string{"all"}
	if Text(p["isIsolation"]) == "1" {
		return append(tabs, "isolation")
	}
	state, op := Text(p["appPackageStatus"]), Text(p["appPackageOperateStatus"])
	if state == "wait_seller_send" {
		switch op {
		case "wait_audit", "wait_process":
			tabs = append(tabs, "waitProcess")
		case "wait_ship":
			tabs = append(tabs, "waitShip")
		case "ship_success", "ship_fail":
			tabs = append(tabs, "submitPlatform")
		}
	}
	if state == "wait_receiver_confirm" {
		tabs = append(tabs, "waitReceiverConfirm")
	}
	afterSales := state == "cancelled" || state == "returned"
	for _, i := range Objects(p["items"]) {
		if Text(i["isRefund"]) == "1" {
			afterSales = true
		}
	}
	if afterSales {
		tabs = append(tabs, "closed")
	}
	return tabs
}
