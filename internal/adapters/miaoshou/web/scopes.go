package web

import (
	"net/url"
	"order_processor/internal/domain/orders"
	"strconv"
	"strings"
)

func RequestScope(body string) string {
	q, err := url.ParseQuery(body)
	if err != nil || q.Get("source") != "orderProcess" {
		return ""
	}
	// Only a whole view can replace membership. Unknown active filters are not
	// interpreted as defaults, so a user's narrowed search cannot remove orders.
	defaults := map[string]string{"source": "orderProcess", "waitProcessTab": "all", "supplierProcessStatus": "all", "priceType": "profit", "logisticsGroupType": "cascader", "isLogisticsCompanyGroupMode": "1"}
	for k, vs := range q {
		for _, v := range vs {
			if v == "" {
				continue
			}
			if k == "page" || k == "pageSize" || k == "appPackageTab" || k == "purchaseTab" || k == "sortField" || k == "sortType" || strings.HasSuffix(k, "Rp") {
				continue
			}
			if defaults[k] != v {
				return ""
			}
		}
	}
	tab := q.Get("appPackageTab")
	switch tab {
	case "waitProcess":
		p := q.Get("purchaseTab")
		if p == "waitPurchase" || p == "hasPurchase" {
			return p
		}
	case "waitShip", "submitPlatform", "waitReceiverConfirm", "isolation", "closed":
		return tab
	}
	return ""
}

func singlePageScope(at, file, request string, data orders.Fields) *orders.Scope {
	scope := RequestScope(request)
	if scope == "" {
		return nil
	}
	q, _ := url.ParseQuery(request)
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("pageSize"))
	total, err := strconv.Atoi(orders.Text(data["total"]))
	if err != nil || total < 0 || page != 1 || size <= 0 || size < total {
		return nil
	}
	if p := orders.Text(data["page"]); p != "" && p != "1" {
		return nil
	}
	list, ok := data["packageList"].([]any)
	if !ok || len(list) != total {
		return nil
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, raw := range list {
		p, ok := raw.(map[string]any)
		if !ok {
			return nil
		}
		id := orders.Text(p["opOrderPackageId"])
		if id == "" || seen[id] {
			return nil
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return &orders.Scope{Name: scope, CapturedAt: at, Files: []string{file}, PackageIDs: ids}
}
