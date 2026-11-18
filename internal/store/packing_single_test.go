package store

import (
	"context"
	"order_processor/internal/domain/packing"
	"path/filepath"
	"testing"
	"time"
)

func TestSingleSubmissionPersistsAndBlocksQueue(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, _ := Create(db, "single test")
	_, err = db.Exec(`INSERT INTO packages(account_id,package_id,payload,captured_at,source_file) VALUES(?,?,?,?,?)`, account, "77", `{"appPackageNo":"MS77","items":[{"opOrderItemId":"88","platformSkuId":1737815246129562642,"quantity":2,"skuSubName":"黑色,L"}]}`, "2026-11-17", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	repo := PackingJobs{DB: db}
	queued := createPackingFixture(t, repo, account, 1)
	req := packing.Request{PackageID: "77", Items: []packing.Item{{OrderItemID: "88", Waybills: []packing.Waybill{{Company: "中通", Number: "001"}}}}}
	item, err := repo.StartSingle(ctx, account, req)
	if err != nil {
		t.Fatal(err)
	}
	if item.Lines[0].SKU != "1737815246129562642" {
		t.Fatal("SKU precision lost", item.Lines)
	}
	if next, err := repo.Claim(ctx, time.Now()); err != nil || next != nil {
		t.Fatal("queue overlapped direct submission", next, err)
	}
	if _, err := repo.StartSingle(ctx, account, req); err == nil {
		t.Fatal("parallel direct submission accepted")
	}
	if err := repo.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	recovered, err := repo.Detail(ctx, account, item.JobID)
	if err != nil || recovered.State != "paused" || recovered.Items[0].State != "unknown" {
		t.Fatal(recovered, err)
	}
	if next, err := repo.Claim(ctx, time.Now()); err != nil || next != nil {
		t.Fatal("unknown direct write didn't block queue", next, err)
	}
	if err := repo.Action(ctx, account, item.JobID, "confirm-completed", item.ID); err != nil {
		t.Fatal(err)
	}
	next, err := repo.Claim(ctx, time.Now().Add(time.Hour))
	if err != nil || next == nil || next.JobID != queued {
		t.Fatal("queue did not resume after resolution", next, err)
	}
}
