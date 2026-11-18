package packing

import (
	"context"
	"errors"
	"order_processor/internal/domain/packing"
	"testing"
)

type labelFixture struct {
	stored, inspected, applied packing.ShippingLabel
	steps                      []string
	failSave                   bool
}

func (f *labelFixture) ShippingLabel(context.Context, string, string) (packing.ShippingLabel, error) {
	return f.stored, nil
}
func (f *labelFixture) SaveShippingLabel(_ context.Context, _, _ string, r packing.ShippingLabel) error {
	f.steps = append(f.steps, "save:"+r.State)
	if f.failSave {
		return errors.New("disk error")
	}
	f.stored = r
	return nil
}
func (f *labelFixture) InspectLabel(context.Context, string, string) (packing.ShippingLabel, error) {
	f.steps = append(f.steps, "inspect")
	return f.inspected, nil
}
func (f *labelFixture) ApplyLabel(context.Context, string, string) (packing.ShippingLabel, error) {
	f.steps = append(f.steps, "apply")
	if f.stored.State != "applying" {
		panic("write before persistence")
	}
	return f.applied, nil
}
func (f *labelFixture) Submit(context.Context, string, packing.Request) (packing.Result, error) {
	f.steps = append(f.steps, "packing")
	return packing.Result{State: "failed"}, nil
}

func TestShippingSavedBeforePackingAndSkippedOnRerun(t *testing.T) {
	f := &labelFixture{inspected: packing.ShippingLabel{State: "ready"}, applied: packing.ShippingLabel{State: "completed"}}
	s := ShippingFirst{Repository: f, Gateway: f, Next: f}
	for i := 0; i < 2; i++ {
		s.Submit(context.Background(), "1", packing.Request{PackageID: "7"})
	}
	want := []string{"inspect", "save:applying", "apply", "save:completed", "packing", "packing"}
	if len(f.steps) != len(want) {
		t.Fatal(f.steps)
	}
	for i := range want {
		if f.steps[i] != want[i] {
			t.Fatal(f.steps)
		}
	}
}

func TestShippingUncertainAndExistingLabels(t *testing.T) {
	for _, tc := range []struct {
		name, previous, current, apply string
		writes, packs                  int
	}{
		{"existing", "", "completed", "", 0, 1},
		{"restart during application", "applying", "ready", "", 0, 0},
		{"lost response", "unknown", "ready", "", 0, 0},
		{"reconciled after interruption", "unknown", "completed", "", 0, 1},
		{"ERP pending", "", "pending", "", 0, 0},
		{"application failed", "", "ready", "failed", 1, 0},
		{"application lost response", "", "ready", "unknown", 1, 0},
		{"retry known failure", "failed", "ready", "completed", 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &labelFixture{stored: packing.ShippingLabel{State: tc.previous}, inspected: packing.ShippingLabel{State: tc.current}, applied: packing.ShippingLabel{State: tc.apply}}
			ShippingFirst{Repository: f, Gateway: f, Next: f}.Submit(context.Background(), "1", packing.Request{PackageID: "7"})
			writes, packs := 0, 0
			for _, step := range f.steps {
				if step == "apply" {
					writes++
				}
				if step == "packing" {
					packs++
				}
			}
			if writes != tc.writes || packs != tc.packs {
				t.Fatal(f.steps)
			}
		})
	}
}

func TestShippingSaveFailureStopsSubmission(t *testing.T) {
	f := &labelFixture{inspected: packing.ShippingLabel{State: "ready"}, failSave: true}
	r, _ := (ShippingFirst{Repository: f, Gateway: f, Next: f}).Submit(context.Background(), "1", packing.Request{PackageID: "7"})
	if r.State != "not_sent" || len(f.steps) != 2 {
		t.Fatal(r, f.steps)
	}
}
