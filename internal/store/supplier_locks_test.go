package store

import (
	"context"
	"errors"
	"order_processor/internal/domain/packing"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentSupplierPurchasers(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, _ := Create(db, "一店")
	other, _ := Create(db, "二店")
	if _, err := db.Exec(`INSERT INTO suppliers(id,account_id,platform,name) VALUES(91,?,'1688','甲厂'),(92,?,'1688','乙厂')`, account, account); err != nil {
		t.Fatal(err)
	}
	alice := SupplierLease{Supplier: 91, UserID: 1, Username: "甲"}
	bobID, err := SaveUser(ctx, db, 0, UserInput{Username: "乙", Password: "测试", Accounts: []string{account}})
	if err != nil {
		t.Fatal(err)
	}
	bob := SupplierLease{Supplier: 91, UserID: bobID, Username: "乙"}
	var wg sync.WaitGroup
	var leases [2]SupplierLease
	var errs [2]error
	for i, l := range []SupplierLease{alice, bob} {
		wg.Add(1)
		go func(i int, l SupplierLease) {
			defer wg.Done()
			leases[i], errs[i] = AcquireSupplier(ctx, db, account, l)
		}(i, l)
	}
	wg.Wait()
	if (errs[0] == nil) == (errs[1] == nil) {
		t.Fatalf("expected one purchaser: %v", errs)
	}
	owner, loser := leases[0], bob
	if errs[1] == nil {
		owner, loser = leases[1], alice
	}
	loser.Supplier = 92
	otherFactory, err := AcquireSupplier(ctx, db, account, loser)
	if err != nil {
		t.Fatal(err)
	}
	invalid := owner
	invalid.UserID = loser.UserID
	if _, err := AcquireSupplier(ctx, db, account, invalid); !errors.Is(err, ErrSupplierLeaseLost) {
		t.Fatal("wrong owner renewed", err)
	}
	if _, err := AcquireSupplier(ctx, db, other, alice); err == nil {
		t.Fatal("foreign supplier")
	}
	if err := ReleaseSupplier(ctx, db, account, invalid); !errors.Is(err, ErrSupplierLeaseLost) {
		t.Fatal(err)
	}
	if _, err := AcquireSupplier(ctx, db, account, owner); err != nil {
		t.Fatal(err)
	}
	recovered := owner
	recovered.ID = 0
	if got, err := AcquireSupplier(ctx, db, account, recovered); err != nil || got.ID != owner.ID {
		t.Fatalf("owner reload: %+v %v", got, err)
	}
	if err := ReleaseSupplier(ctx, db, account, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireSupplier(ctx, db, account, owner); !errors.Is(err, ErrSupplierLeaseLost) {
		t.Fatal(err)
	}
	replacement, err := AcquireSupplier(ctx, db, account, SupplierLease{Supplier: 91, UserID: loser.UserID, Username: loser.Username})
	if err != nil {
		t.Fatal(err)
	}
	if err := ReleaseSupplier(ctx, db, account, owner); !errors.Is(err, ErrSupplierLeaseLost) {
		t.Fatal(err)
	}
	if _, err := AcquireSupplier(ctx, db, account, replacement); err != nil {
		t.Fatal(err)
	}
	repo := PackingJobs{db}
	forged := otherFactory
	forged.Supplier = 91
	if _, err = repo.CreateWithLease(ctx, account, packing.BatchInput{Name: "错误锁"}, nil, forged); !errors.Is(err, ErrSupplierLeaseLost) {
		t.Fatal("wrong factory submitted", err)
	}
}

func TestPluginWritesRespectSupplierLease(t *testing.T) {
	db, account, _ := captureFixture(t)
	captureProduct(t, db, account, "100", "CN", "HK")
	captureProduct(t, db, account, "200", "CN")
	original, err := CaptureSupplier(db, 1, account, "100", "已占用厂家")
	if err != nil {
		t.Fatal(err)
	}
	supplier := original["supplierId"].(int64)
	_, err = AcquireSupplier(context.Background(), db, account, SupplierLease{Supplier: supplier, UserID: 1, Username: "采购甲"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CaptureSupplier(db, 2, account, "100", "新厂家"); !errors.Is(err, ErrSupplierBusy) {
		t.Fatal("source reassigned", err)
	}
	if _, err = CaptureSupplier(db, 2, account, "200", "已占用厂家"); !errors.Is(err, ErrSupplierBusy) {
		t.Fatal("target reassigned", err)
	}
	if err = AssignSupplier(db, 2, account, "1688", "CN", "100", 0); !errors.Is(err, ErrSupplierBusy) {
		t.Fatal("source cleared", err)
	}
	if _, err = SaveProcurementMapping(db, 2, account, "100", 0, sampleMapping("黑色")); !errors.Is(err, ErrSupplierBusy) {
		t.Fatal("mapping overwritten", err)
	}
	if _, err = SaveProcurementMapping(db, 1, account, "100", 0, sampleMapping("黑色")); err != nil {
		t.Fatal(err)
	}
}

func TestRestartClearsSupplierPurchases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	account, err := Create(db, "店")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	lease, err := AcquireSupplier(ctx, db, account, SupplierLease{UserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate an old installation upgrading with an outstanding SQLite lease.
	if _, err = db.Exec("CREATE TABLE supplier_leases(id INTEGER); INSERT INTO supplier_leases VALUES(1)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if found, err := SupplierLeases(ctx, db, account); err != nil || len(found) != 0 {
		t.Fatalf("purchases after restart: %v, %v", found, err)
	}
	if _, err = AcquireSupplier(ctx, db, account, lease); !errors.Is(err, ErrSupplierLeaseLost) {
		t.Fatal("stale purchase restored after restart", err)
	}
	var tables int
	if err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='supplier_leases'").Scan(&tables); err != nil || tables != 0 {
		t.Fatal("legacy lease table still present", tables, err)
	}
}
