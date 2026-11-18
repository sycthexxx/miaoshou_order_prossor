package store

import (
	"context"
	"encoding/json"
	"order_processor/internal/domain/packing"
	"path/filepath"
	"testing"
	"time"
)

func TestCompletedTaskRerunPreservesHistoryAndIncludesGifts(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, _ := Create(db, "test")
	repo := PackingJobs{DB: db}
	_, err = db.Exec(`INSERT INTO packages(account_id,package_id,payload,captured_at,source_file) VALUES(?,?,?,'2026','fixture')`, account, "p", `{"appPackageNo":"MS1","platformOrderSn":587225079869441951,"appPurchaseStatus":"2","items":[{"opOrderItemId":"main","quantity":1,"picUrl":"https://images.example/blue.jpg"},{"opOrderItemId":"gift","quantity":1,"picUrl":"https://images.example/socks.jpg","isGift":true}]}`)
	if err != nil {
		t.Fatal(err)
	}
	info := packing.Item{OrderItemID: "main", Purchase: &packing.Purchase{Platform: "1688", OrderNumber: "old", Buyer: "buyer", Seller: "seller"}, AutoSyncAmount: true}
	original, err := repo.Create(ctx, account, packing.BatchInput{Name: "original", SourceKey: "not-in-procurement", IntervalSeconds: 5}, []packing.JobItem{{Request: packing.Request{PackageID: "p", Items: []packing.Item{info}}, Lines: []packing.Candidate{{Selection: packing.Selection{PackageID: "p", ItemID: "main"}, PackageNo: "MS1", Quantity: 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RerunPreview(ctx, account, original); err == nil {
		t.Fatal("queued task editable")
	}
	item, _ := repo.Claim(ctx, time.Now())
	if err := repo.Finish(ctx, *item, packing.Result{State: "completed", Archive: "original-response"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	old, _ := repo.Detail(ctx, account, original)
	before, _ := json.Marshal(old)
	if len(old.Items[0].Lines) != 1 || old.Items[0].Lines[0].OrderNo != "587225079869441951" || old.Items[0].Lines[0].Image.URL != "https://images.example/blue.jpg" {
		t.Fatal("historical metadata missing or scope changed", old)
	}
	list, e := repo.List(ctx, account)
	if e != nil || len(list[0].PackingInfos) != 1 || list[0].PackingInfos[0].Purchase.OrderNumber != "old" || list[0].PackingInfos[0].OrderItemID != "" {
		t.Fatal("saved packing information missing", list, e)
	}
	var savedLines string
	db.QueryRow(`SELECT lines_json FROM packing_job_items WHERE job_id=?`, original).Scan(&savedLines)
	var saved []packing.Candidate
	json.Unmarshal([]byte(savedLines), &saved)
	if saved[0].Image != nil || saved[0].OrderNo != "" {
		t.Fatal("display enrichment rewrote history", saved)
	}
	if _, err := repo.RerunPreview(ctx, "other-account", original); err == nil {
		t.Fatal("account isolation lost")
	}
	preview, err := repo.RerunPreview(ctx, account, original)
	if err != nil || len(preview.Items[0].Request.Items) != 2 || !preview.Items[0].Lines[1].Gift {
		t.Fatal(preview, err)
	}
	if preview.Items[0].Lines[0].Image.URL != "https://images.example/blue.jpg" || preview.Items[0].Lines[1].Image.URL != "https://images.example/socks.jpg" {
		t.Fatal("SKU images mixed up", preview.Items[0].Lines)
	}
	req := preview.Items[0].Request
	for i := range req.Items {
		req.Items[i].Purchase.OrderNumber = "corrected"
	}
	in := PackingRerunInput{IntervalSeconds: 5, Requests: []packing.Request{req}}
	bad := in
	bad.Requests = []packing.Request{{PackageID: "foreign", Items: req.Items}}
	if _, err := repo.Rerun(ctx, account, original, bad); err == nil {
		t.Fatal("scope injection accepted")
	}
	id, err := repo.Rerun(ctx, account, original, in)
	if err != nil || id == original {
		t.Fatal(id, err)
	}
	newJob, _ := repo.Detail(ctx, account, id)
	if newJob.State != "queued" || len(newJob.Items) != 1 || newJob.Items[0].State != "pending" || len(newJob.Items[0].Request.Items) != 2 {
		t.Fatal(newJob)
	}
	for _, i := range newJob.Items[0].Request.Items {
		if i.Purchase.OrderNumber != "corrected" {
			t.Fatal(i)
		}
	}
	afterJob, _ := repo.Detail(ctx, account, original)
	after, _ := json.Marshal(afterJob)
	if string(before) != string(after) {
		t.Fatal("original history modified")
	}
	if _, err := repo.Rerun(ctx, account, original, in); err == nil {
		t.Fatal("duplicate active rerun accepted")
	}
}
