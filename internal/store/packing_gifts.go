package store

import (
	"context"
	"order_processor/internal/domain/packing"
	"strconv"
)

func (s PackingJobs) packageLines(ctx context.Context, account, packageID string) ([]packing.Candidate, error) {
	var payload string
	if err := s.DB.QueryRowContext(ctx, `SELECT payload FROM packages WHERE account_id=? AND package_id=?`, account, packageID).Scan(&payload); err != nil {
		return nil, err
	}
	return packageCandidates(payload, packageID)
}

func packageCandidates(payload, packageID string) ([]packing.Candidate, error) {
	var p object
	if err := decode([]byte(payload), &p); err != nil {
		return nil, err
	}
	lines := []packing.Candidate{}
	for _, item := range objects(p["items"]) {
		spec := text(item["skuSubCnName"])
		if spec == "" {
			spec = text(item["skuSubName"])
		}
		quantity, _ := strconv.ParseInt(text(item["quantity"]), 10, 64)
		gift := text(item["isTiktokGift"])
		if gift == "" {
			gift = text(item["isGift"])
		}
		lines = append(lines, packing.Candidate{Selection: packing.Selection{PackageID: packageID, ItemID: text(item["opOrderItemId"])}, PackageNo: text(p["appPackageNo"]), OrderNo: packageOrderNo(p, item), Spec: spec, SKU: text(item["platformSkuId"]), Quantity: quantity, Gift: gift == "1" || gift == "true", Image: &packing.SKUImage{OrderItemID: text(item["opOrderItemId"]), Title: text(item["title"]), Thumbnail: text(item["picUrlThumb"]), URL: text(item["picUrl"]), Original: text(item["originalPicUrl"])}})
	}
	return lines, nil
}

func (s PackingJobs) IncludePackageGifts(ctx context.Context, account string, item packing.JobItem) (packing.JobItem, error) {
	lines, err := s.packageLines(ctx, account, item.Request.PackageID)
	if err != nil {
		return item, err
	}
	// Enrich display metadata without changing the saved scope or quantities.
	metadata := map[string]packing.Candidate{}
	for _, line := range lines {
		metadata[line.ItemID] = line
	}
	for index, line := range item.Lines {
		if current, ok := metadata[line.ItemID]; ok {
			if item.Lines[index].Image == nil {
				item.Lines[index].Image = current.Image
			}
			if item.Lines[index].OrderNo == "" {
				item.Lines[index].OrderNo = current.OrderNo
			}
		}
	}
	return packing.IncludePackageGifts(item, lines), nil
}

func packageOrderNo(p, item object) string {
	if n := text(item["platformOrderSn"]); n != "" {
		return n
	}
	if n := text(p["platformOrderSn"]); n != "" {
		return n
	}
	order, _ := p["orderInfo"].(map[string]any)
	return text(order["platformOrderSn"])
}
