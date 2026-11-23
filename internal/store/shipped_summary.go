package store

import (
	"encoding/json"
	"math"
	"slices"
	"sort"
	"strings"
)

type shippedOrder struct {
	PackageID string   `json:"packageId"`
	OrderNo   string   `json:"orderNo"`
	Shop      string   `json:"shop"`
	Items     []object `json:"items"`
}

type shippedWaybill struct {
	firstMileShipment
	PackageID string `json:"packageId"`
}

type shippedPurchase struct {
	Key       string           `json:"key"`
	Numbers   []string         `json:"numbers"`
	OfferIDs  []string         `json:"offerIds,omitempty"`
	Shipments []shippedWaybill `json:"shipments"`
	Orders    []*shippedOrder  `json:"orders"`
}

type shippedFactory struct {
	Name    string `json:"name"`
	ChatURL string `json:"chatUrl,omitempty"`
	OfferID string `json:"offerId,omitempty"`
	has1688 bool
	Missing int                `json:"missing"`
	Groups  []*shippedPurchase `json:"groups"`
}

func ShippedSummary(db rowsQuerier, account string) ([]*shippedFactory, error) {
	result, err := Orders(db, account, "", 1, math.MaxInt, "waitProcess", "hasPurchase")
	if err != nil {
		return nil, err
	}
	mappings, err := sourceSuppliers(db, account)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT e.package_id,e.purchase_id,s.name
		FROM supplier_evidence e JOIN suppliers s ON s.id=e.supplier_id AND s.account_id=e.account_id WHERE e.account_id=?`, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sellers := map[[2]string]string{}
	for rows.Next() {
		var pkg, purchase, name string
		if err = rows.Scan(&pkg, &purchase, &name); err != nil {
			return nil, err
		}
		sellers[[2]string{pkg, purchase}] = name
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	factories, err := groupShippedOrders(result["items"].([]object), sellers, mappings)
	if err != nil {
		return nil, err
	}
	if err := shippedSupplierContacts(db, account, factories); err != nil {
		return nil, err
	}
	return factories, nil
}

func groupShippedOrders(packages []object, sellers map[[2]string]string, mappings map[string]object) ([]*shippedFactory, error) {
	factories := map[string]*shippedFactory{}
	groups := map[[2]string]*shippedPurchase{}
	orderIndex := map[[3]string]*shippedOrder{}
	for _, p := range packages {
		pkg := text(p["opOrderPackageId"])
		items := objects(p["items"])
		info, _ := p["orderInfo"].(map[string]any)
		orderNo := firstText(p["platformOrderSn"], info["platformOrderSn"])
		shop := firstText(p["shopName"], p["platformShopName"])
		var purchases []firstMilePurchase
		if err := json.Unmarshal(p["firstMilePurchases"].(json.RawMessage), &purchases); err != nil {
			return nil, err
		}
		if len(purchases) == 0 {
			purchases = []firstMilePurchase{{Shipments: []firstMileShipment{{Number: text(p["headLogisticsNo"])}}}}
		}
		for _, purchase := range purchases {
			number := ""
			if purchase.Platform != "" {
				number = validShippedNumber(purchase.Number)
			}
			linked := shippedItems(items, purchase.ItemIDs, len(purchases) == 1)
			// Keep the group header and contact aligned with the linked offer's supplier.
			name := shippedFactoryFromSources(linked, mappings)
			if name == "" {
				name = sellers[[2]string{pkg, purchase.ID}]
			}
			if name == "" {
				name = "未指定厂家"
			}
			factory := factories[name]
			if factory == nil {
				factory = &shippedFactory{Name: name, Groups: []*shippedPurchase{}}
				factories[name] = factory
			}
			factory.has1688 = factory.has1688 || purchase.Platform == "1688"
			if name != "未指定厂家" && factory.OfferID == "" {
				factory.OfferID = shippedContactOffer(linked, mappings, name)
			}
			shipments := []firstMileShipment{}
			for _, shipment := range purchase.Shipments {
				shipment.Number = validShippedNumber(shipment.Number)
				if shipment.Number != "" && shipment.Number != purchase.Number {
					shipments = append(shipments, shipment)
				}
			}
			add := func(key string, waybills []firstMileShipment) {
				groupKey := [2]string{name, key}
				group := groups[groupKey]
				if group == nil {
					group = &shippedPurchase{Key: key, Shipments: []shippedWaybill{}}
					if number != "" {
						group.Numbers = []string{number}
					}
					groups[groupKey] = group
					factory.Groups = append(factory.Groups, group)
				}
				for _, shipment := range waybills {
					group.addWaybill(shippedWaybill{firstMileShipment: shipment, PackageID: pkg})
				}
				orderKey := [3]string{name, key, pkg}
				order := orderIndex[orderKey]
				if order == nil {
					order = &shippedOrder{PackageID: pkg, OrderNo: orderNo, Shop: shop, Items: []object{}}
					orderIndex[orderKey] = order
					group.Orders = append(group.Orders, order)
				}
				for _, item := range linked {
					for _, source := range objects(item["sourceList"]) {
						offer := shippedOfferID(source)
						if offer != "" && !slices.Contains(group.OfferIDs, offer) {
							group.OfferIDs = append(group.OfferIDs, offer)
						}
					}
					found := false
					for _, existing := range order.Items {
						found = found || text(item["opOrderItemId"]) == text(existing["opOrderItemId"])
					}
					if !found {
						order.Items = append(order.Items, shippedSKU(item))
					}
				}
			}
			switch {
			case number != "":
				add("purchase:"+number, shipments)
			case len(shipments) > 0:
				add("waybill:"+shipments[0].Number, shipments)
			default:
				add("package:"+pkg+":"+purchase.ID, nil)
			}
		}
	}
	result := make([]*shippedFactory, 0, len(factories))
	for _, f := range factories {
		f.Groups = mergeSharedWaybills(f.Groups)
		for _, group := range f.Groups {
			if len(group.Shipments) == 0 {
				f.Missing++
			}
		}
		sort.SliceStable(f.Groups, func(i, j int) bool {
			return len(f.Groups[i].Shipments) == 0 && len(f.Groups[j].Shipments) > 0
		})
		result = append(result, f)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Missing != result[j].Missing {
			return result[i].Missing > result[j].Missing
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func shippedItems(items []object, ids []string, single bool) []object {
	if len(ids) == 0 && single {
		return items
	}
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	result := []object{}
	for _, item := range items {
		if set[text(item["opOrderItemId"])] {
			result = append(result, item)
		}
	}
	return result
}

func shippedOfferID(source object) string {
	offer := firstText(source["sourceItemId"], source["productId"])
	if text(source["source"]) != "1688" || !offerNumber.MatchString(offer) {
		return ""
	}
	return offer
}

func shippedContactOffer(items []object, mappings map[string]object, factory string) string {
	var offer string
	for _, item := range items {
		for _, source := range objects(item["sourceList"]) {
			product := shippedOfferID(source)
			if product == "" {
				continue
			}
			key, _ := json.Marshal([]string{"1688", firstText(source["sourceSite"], source["site"]), product})
			name := text(mappings["source:"+string(key)]["name"])
			if name == factory {
				return product
			}
			if name != "" && strings.HasSuffix(name, factory) {
				offer = product
			}
		}
	}
	return offer
}

func shippedFactoryFromSources(items []object, mappings map[string]object) string {
	var name string
	for _, item := range items {
		for _, source := range objects(item["sourceList"]) {
			key, _ := json.Marshal([]string{text(source["source"]), firstText(source["sourceSite"], source["site"]),
				firstText(source["sourceItemId"], source["productId"])})
			match := text(mappings["source:"+string(key)]["name"])
			if match == "" {
				continue
			}
			if name != "" && name != match {
				return ""
			}
			name = match
		}
	}
	return name
}

func shippedSKU(item object) object {
	out := object{}
	for _, key := range []string{"opOrderItemId", "title", "cnTitle", "skuSubName", "skuSubCnName", "platformSkuId", "quantity",
		"picUrlThumb", "picUrl", "originalPicUrl", "isGift", "isTiktokGift"} {
		out[key] = item[key]
	}
	out["opOrderItemId"] = text(item["opOrderItemId"])
	out["platformSkuId"] = text(item["platformSkuId"])
	return out
}

func validShippedNumber(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "0", "-", "--", "—":
		return ""
	}
	return value
}

func firstText(values ...any) string {
	for _, value := range values {
		if s := text(value); s != "" {
			return s
		}
	}
	return ""
}

func (g *shippedPurchase) addWaybill(shipment shippedWaybill) {
	for i, existing := range g.Shipments {
		if existing.Number != shipment.Number {
			continue
		}
		if (existing.TraceID == "" || existing.TraceID == "0") && shipment.TraceID != "" && shipment.TraceID != "0" {
			g.Shipments[i] = shipment
		}
		return
	}
	g.Shipments = append(g.Shipments, shipment)
}

func mergeSharedWaybills(groups []*shippedPurchase) []*shippedPurchase {
	parent := make([]int, len(groups))
	waybills := map[string]int{}
	root := func(i int) int {
		for parent[i] != i {
			i = parent[i]
		}
		return i
	}
	for i, group := range groups {
		parent[i] = i
		for _, shipment := range group.Shipments {
			if owner, ok := waybills[shipment.Number]; ok {
				parent[root(i)] = root(owner)
			} else {
				waybills[shipment.Number] = i
			}
		}
	}
	result := []*shippedPurchase{}
	for i, group := range groups {
		target := groups[root(i)]
		if target == group {
			result = append(result, group)
			continue
		}
		target.Numbers = append(target.Numbers, group.Numbers...)
		for _, offer := range group.OfferIDs {
			if !slices.Contains(target.OfferIDs, offer) {
				target.OfferIDs = append(target.OfferIDs, offer)
			}
		}
		for _, shipment := range group.Shipments {
			target.addWaybill(shipment)
		}
		for _, order := range group.Orders {
			index := slices.IndexFunc(target.Orders, func(row *shippedOrder) bool { return row.PackageID == order.PackageID })
			if index < 0 {
				target.Orders = append(target.Orders, order)
				continue
			}
			existing := target.Orders[index]
			for _, item := range order.Items {
				if !slices.ContainsFunc(existing.Items, func(row object) bool { return text(row["opOrderItemId"]) == text(item["opOrderItemId"]) }) {
					existing.Items = append(existing.Items, item)
				}
			}
		}
	}
	return result
}

// PendingShippedWaybills returns one query per domestic waybill in the shipped summary.
func PendingShippedWaybills(db rowsQuerier, account string) ([]map[string]string, error) {
	factories, err := ShippedSummary(db, account)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var queries []map[string]string
	for _, factory := range factories {
		for _, group := range factory.Groups {
			for _, waybill := range group.Shipments {
				if seen[waybill.Number] || waybill.TraceID == "" || waybill.TraceID == "0" {
					continue
				}
				if waybill.Tracking != nil && waybill.Tracking.Status != "已打包" &&
					waybill.Tracking.Status != "已揽收" && waybill.Tracking.Status != "运输中" {
					continue
				}
				seen[waybill.Number] = true
				queries = append(queries, map[string]string{
					"kind": "domestic", "number": waybill.Number, "company": waybill.Company,
					"traceId": waybill.TraceID, "packageId": waybill.PackageID,
				})
			}
		}
	}
	return queries, nil
}
