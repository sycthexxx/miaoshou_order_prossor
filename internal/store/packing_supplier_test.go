package store

import (
	"context"
	"fmt"
	"order_processor/internal/domain/packing"
	"path/filepath"
	"testing"
)

func TestSupplierPackingAcrossProductsCombinesPackageAndGift(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, _ := Create(db, "factory test")
	for n := 1; n <= 3; n++ {
		product := fmt.Sprintf(`{"platform":"tiktok","shopId":"shop","platformItemId":"p%d","sourceList":[{"source":"1688","sourceItemId":"%d","sourceItemUrl":"https://detail.1688.com/offer/%d.html"}]}`, n, n, n)
		if _, err = db.Exec(`INSERT INTO shop_products(account_id,platform,shop_id,platform_item_id,payload,captured_at,source_file) VALUES(?,'tiktok','shop',?,?,'2026','test')`, account, fmt.Sprintf("p%d", n), product); err != nil {
			t.Fatal(err)
		}
	}
	result, _ := db.Exec(`INSERT INTO suppliers(account_id,platform,name) VALUES(?,'1688','factory')`, account)
	supplier, _ := result.LastInsertId()
	for _, id := range []string{"1", "2"} {
		if err = AssignSupplier(db, account, "1688", "", id, supplier); err != nil {
			t.Fatal(err)
		}
	}
	payload := `{"opOrderPackageId":"100","appPackageNo":"MS100","platform":"tiktok","shopId":"shop","appPackageStatus":"wait_seller_send","appPackageOperateStatus":"wait_process","appPurchaseStatus":"0","items":[{"opOrderItemId":"11","platformItemId":"p1","skuSubName":"黑色,L","quantity":1,"refundQuantity":0},{"opOrderItemId":"12","platformItemId":"p2","skuSubName":"黑色,L","quantity":1,"refundQuantity":0},{"opOrderItemId":"13","platformItemId":"p3","quantity":1,"refundQuantity":0},{"opOrderItemId":"gift","isGift":"1","quantity":1}]}`
	if _, err = db.Exec(`INSERT INTO packages(account_id,package_id,payload,captured_at,source_file) VALUES(?,'100',?,'2026','test')`, account, payload); err != nil {
		t.Fatal(err)
	}
	repo := PackingJobs{db}
	key := fmt.Sprintf("supplier:%d", supplier)
	candidates, err := repo.Candidates(account, key, false)
	if err != nil || len(candidates) != 2 {
		t.Fatal(candidates, err)
	}
	single, err := repo.Candidates(account, `source:["1688","","1"]`, false)
	if err != nil || len(single) != 1 {
		t.Fatal("single product changed", single, err)
	}
	in := packing.BatchInput{Name: "factory task", SourceKey: key, IntervalSeconds: 5, Selections: []packing.Selection{{PackageID: "100", ItemID: "11"}, {PackageID: "100", ItemID: "12"}}, Info: packing.Item{Purchase: &packing.Purchase{Platform: "1688", OrderNumber: "5127929209710117142", Buyer: "buyer", Seller: "seller"}, AutoSyncAmount: true}}
	items, err := packing.PrepareBatch(in, candidates)
	if err != nil || len(items) != 1 || len(items[0].Request.Items) != 2 {
		t.Fatal(items, err)
	}
	items[0], err = repo.IncludePackageGifts(context.Background(), account, items[0])
	if err != nil || len(items[0].Request.Items) != 3 {
		t.Fatal(items, err)
	}
	id, err := repo.Create(context.Background(), account, in, items)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := repo.Detail(context.Background(), account, id)
	if err != nil || len(detail.Items) != 1 || len(detail.Items[0].Request.Items) != 3 {
		t.Fatal(detail, err)
	}
	in.Selections = append(in.Selections, packing.Selection{PackageID: "100", ItemID: "13"})
	if _, err = packing.PrepareBatch(in, candidates); err == nil {
		t.Fatal("unrelated product accepted")
	}
}
