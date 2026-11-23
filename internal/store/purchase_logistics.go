package store

import (
	"database/sql"
	"encoding/json"
	"sort"
)

type firstMileShipment struct {
	Company  string            `json:"company"`
	Number   string            `json:"number"`
	TraceID  string            `json:"traceId,omitempty"`
	Tracking *DomesticTracking `json:"tracking,omitempty"`
}

type firstMilePurchase struct {
	ID        string              `json:"id"`
	Number    string              `json:"number"`
	Platform  string              `json:"platform"`
	Status    string              `json:"status"`
	Shipments []firstMileShipment `json:"shipments"`
	ItemIDs   []string            `json:"itemIds,omitempty"`
}

func savePurchaseLogistics(tx *sql.Tx, account, packageID, at string, purchases, items []object) error {
	linkedItems := map[string]map[string]bool{}
	for _, item := range items {
		itemID := text(item["opOrderItemId"])
		if itemID == "" {
			continue
		}
		related, _ := item["relatePurchaseOrderIds"].([]any)
		for _, value := range append([]any{item["purchaseOrderId"]}, related...) {
			id := text(value)
			if id == "" || id == "0" {
				continue
			}
			if linkedItems[id] == nil {
				linkedItems[id] = map[string]bool{}
			}
			linkedItems[id][itemID] = true
		}
	}
	records := make([]firstMilePurchase, 0, len(purchases))
	for _, p := range purchases {
		if text(p["purchaseOrderId"]) == "" || text(p["isUnrelated"]) == "1" || text(p["isUnrelated"]) == "true" {
			continue
		}
		record := firstMilePurchase{
			ID: text(p["purchaseOrderId"]), Number: text(p["purchaseOrderSn"]),
			Platform: text(p["purchasePlatformName"]), Status: text(p["purchaseOrderStatus"]),
			Shipments: []firstMileShipment{},
		}
		for id := range linkedItems[record.ID] {
			record.ItemIDs = append(record.ItemIDs, id)
		}
		sort.Strings(record.ItemIDs)
		seen := map[[2]string]int{}
		shipmentRows := objects(p["purchaseOrderPackages"])
		if len(shipmentRows) == 0 {
			shipmentRows = []object{p}
		}
		for _, row := range shipmentRows {
			shipment := firstMileShipment{Company: text(row["purchaseOrderLogisticsName"]), Number: text(row["purchaseOrderWaybillCode"]), TraceID: text(row["purchaseOrderFilterId"])}
			if shipment.Company == "" && shipment.Number == "" {
				continue
			}
			key := [2]string{shipment.Company, shipment.Number}
			if index, ok := seen[key]; ok {
				if record.Shipments[index].TraceID == "" {
					record.Shipments[index].TraceID = shipment.TraceID
				}
				continue
			}
			seen[key] = len(record.Shipments)
			record.Shipments = append(record.Shipments, shipment)
		}
		if len(record.Shipments) == 0 {
			fallback := firstMileShipment{Company: text(p["purchaseOrderLogisticsName"]), Number: text(p["purchaseOrderWaybillCode"]), TraceID: text(p["purchaseOrderFilterId"])}
			if fallback.Company != "" || fallback.Number != "" {
				record.Shipments = append(record.Shipments, fallback)
			}
		}
		sort.Slice(record.Shipments, func(a, b int) bool {
			left, right := record.Shipments[a], record.Shipments[b]
			if left.Number != right.Number {
				return left.Number < right.Number
			}
			return left.Company < right.Company
		})
		records = append(records, record)
	}
	sort.Slice(records, func(a, b int) bool { return records[a].ID < records[b].ID })
	raw, err := json.Marshal(records)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO package_purchase_logistics(account_id,package_id,captured_at,purchases) VALUES(?,?,?,?)
  ON CONFLICT(account_id,package_id) DO UPDATE SET captured_at=excluded.captured_at,purchases=excluded.purchases
  WHERE excluded.captured_at>=package_purchase_logistics.captured_at`, account, packageID, at, string(raw))
	return err
}
