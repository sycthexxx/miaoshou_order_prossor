package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestDomesticTrackingStatusUsesNewestStep(t *testing.T) {
	pickup := TrackingStep{Time: "2026-11-23 08:00:00", Description: "揽收"}
	transit := TrackingStep{Time: "2026-11-23 09:00:00", Description: "预计送达"}
	delivered := TrackingStep{Time: "2026-11-23 10:00:00", Description: "快件已签收"}
	for _, test := range []struct {
		steps []TrackingStep
		want  string
	}{
		{nil, "已打包"},
		{[]TrackingStep{pickup}, "已揽收"},
		{[]TrackingStep{transit, pickup}, "运输中"},
		{[]TrackingStep{pickup, delivered}, "已送达"},
	} {
		if state, _ := currentDomesticTracking(test.steps, defaultTrackingRules()); state.Status != test.want {
			t.Fatalf("got %q, want %q", state.Status, test.want)
		}
	}
}

func TestDomesticTrackingSharesStateAndPreservesNewerEvent(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "tracking.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, err := Create(db, "tracking")
	if err != nil {
		t.Fatal(err)
	}
	other, err := Create(db, "other")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	state, err := SaveDomesticTracking(context.Background(), db, account, "YT100", now,
		[]TrackingStep{{Time: "2026-11-23 08:00:00", Description: "揽收"}, {Time: "2026-11-23 10:00:00", Description: "已送达"}})
	if err != nil || state.Status != "已送达" {
		t.Fatal(state, err)
	}
	state, err = SaveDomesticTracking(context.Background(), db, account, "YT100", now.Add(time.Second), nil)
	if err != nil || state.Status != "已送达" {
		t.Fatal("empty response overwrote a known event", state, err)
	}
	states, err := domesticTrackingStates(db, account)
	if err != nil {
		t.Fatal(err)
	}
	for _, purchaseID := range []string{"1", "2"} {
		raw, err := withDomesticTracking(json.RawMessage(`[{"id":"`+purchaseID+`","shipments":[{"number":"YT100"}]}]`), states)
		var purchases []firstMilePurchase
		if err != nil || json.Unmarshal(raw, &purchases) != nil || purchases[0].Shipments[0].Tracking.Status != "已送达" {
			t.Fatalf("missing shared tracking status for purchase %s: %s %v", purchaseID, raw, err)
		}
	}
	unrelated, err := domesticTrackingStates(db, other)
	if err != nil || len(unrelated) != 0 {
		t.Fatal("tracking leaked across accounts", unrelated, err)
	}
}

func TestDomesticTrackingMatchesPickupAndReceiptWording(t *testing.T) {
	for _, text := range []string{
		"收件人:本人取件(一体机扫描)，如有疑问请联系快递员",
		"您的快件已由【门口】 收取，如遇问题请联系快递员",
		"您的快件已由【南兴43】 收取",
	} {
		state, _ := currentDomesticTracking([]TrackingStep{{Description: text}}, defaultTrackingRules())
		if state.Status != "已送达" {
			t.Fatalf("%q yielded %q", text, state.Status)
		}
	}
	for _, description := range []string{"尚未签收", "请本人取件", "预计今晚送达", "已收取运费"} {
		state, _ := currentDomesticTracking([]TrackingStep{{Description: description}}, defaultTrackingRules())
		if state.Status == "已送达" {
			t.Fatalf("false delivery for %q", description)
		}
	}
}

func TestDomesticTrackingUsesSavedAccountRules(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "rules.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, err := Create(db, "custom rules")
	if err != nil {
		t.Fatal(err)
	}
	steps := []TrackingStep{{Description: "快件已交给仓库前台"}}
	rules, err := NormalizeTrackingRules([]TrackingRule{{Status: "仓库已收货", Keywords: []string{"已交给 + 前台"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveDomesticTrackingRules(ctx, db, account, rules); err != nil {
		t.Fatal(err)
	}
	state, err := SaveDomesticTracking(ctx, db, account, "YT100", time.Now(), steps)
	if err != nil || state.Status != "仓库已收货" {
		t.Fatal(state, err)
	}
}
