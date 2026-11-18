package store

import (
	"context"
	"order_processor/internal/domain/packing"
	"path/filepath"
	"testing"
)

func TestShippingLabelSurvivesRestartAndIsAccountScoped(t *testing.T) {
	file := filepath.Join(t.TempDir(), "orders.db")
	db, err := Open(file)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := Create(db, "a")
	b, _ := Create(db, "b")
	ctx := context.Background()
	want := packing.ShippingLabel{State: "completed", Number: "DCS123", Archive: "runtime/a/waybill/1"}
	if err = (PackingRepository{DB: db}).SaveShippingLabel(ctx, a, "7", want); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r, err := (PackingRepository{DB: db}).ShippingLabel(ctx, a, "7")
	if err != nil || r != want {
		t.Fatal(r, err)
	}
	r, err = (PackingRepository{DB: db}).ShippingLabel(ctx, b, "7")
	if err != nil || r.State != "" {
		t.Fatal(r, err)
	}
}
