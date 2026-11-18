package store

import (
	"context"
	"order_processor/internal/domain/packing"
	"path/filepath"
	"testing"
	"time"
)

func createPackingFixture(t *testing.T, s PackingJobs, account string, n int) int64 {
	t.Helper()
	items := []packing.JobItem{}
	for i := 0; i < n; i++ {
		pid := string(rune('a' + i))
		items = append(items, packing.JobItem{Request: packing.Request{PackageID: pid, Items: []packing.Item{{OrderItemID: pid, Waybills: []packing.Waybill{{Company: "company", Number: "001"}}}}}, Lines: []packing.Candidate{{Selection: packing.Selection{PackageID: pid, ItemID: pid}, PackageNo: "MS" + pid, Quantity: 1}}})
	}
	id, err := s.Create(context.Background(), account, packing.BatchInput{Name: "test", SourceKey: "source", IntervalSeconds: 5}, items)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func TestPackingQueuePauseDelayAndRestart(t *testing.T) {
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "orders.db")
	db, err := Open(file)
	if err != nil {
		t.Fatal(err)
	}
	account, _ := Create(db, "test")
	s := PackingJobs{db}
	id := createPackingFixture(t, s, account, 3)
	now := time.Now()
	first, err := s.Claim(ctx, now)
	if err != nil || first == nil {
		t.Fatal(first, err)
	}
	// Even a competing runner cannot claim a second package until Finish commits.
	if second, err := s.Claim(ctx, now.Add(time.Hour)); err != nil || second != nil {
		t.Fatal("parallel claim", second, err)
	}
	if err = s.Action(ctx, account, id, "pause", 0); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(ctx, *first, packing.Result{State: "completed"}, now); err != nil {
		t.Fatal(err)
	}
	if second, _ := s.Claim(ctx, now.Add(time.Hour)); second != nil {
		t.Fatal("pause lost while request was active")
	}
	if err = s.Action(ctx, account, id, "resume", 0); err != nil {
		t.Fatal(err)
	}
	if second, _ := s.Claim(ctx, now.Add(4*time.Second)); second != nil {
		t.Fatal("interval ignored")
	}
	second, err := s.Claim(ctx, now.Add(5*time.Second))
	if err != nil || second == nil {
		t.Fatal(second, err)
	}
	db.Close()
	db, err = Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s = PackingJobs{db}
	if err = s.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	job, err := s.Detail(ctx, account, id)
	if err != nil {
		t.Fatal(err)
	}
	if job.State != "paused" || job.Counts["unknown"] != 1 || job.Counts["completed"] != 1 || job.Counts["pending"] != 1 {
		t.Fatal(job)
	}
	if err = s.Action(ctx, account, id, "resume", 0); err == nil {
		t.Fatal("unknown result auto-retried")
	}
	if err = s.Action(ctx, account, id, "confirm-completed", second.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.Action(ctx, account, id, "resume", 0); err != nil {
		t.Fatal(err)
	}
	third, err := s.Claim(ctx, now.Add(time.Minute))
	if err != nil || third == nil || third.ID == second.ID {
		t.Fatal(third, err)
	}
	if err = s.Finish(ctx, *third, packing.Result{State: "completed"}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	job, _ = s.Detail(ctx, account, id)
	if job.State != "completed" || job.Counts["completed"] != 3 {
		t.Fatal(job)
	}
}
func TestPackingQueueCancelAndAccountIsolation(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, _ := Create(db, "a")
	b, _ := Create(db, "b")
	s := PackingJobs{db}
	id := createPackingFixture(t, s, a, 2)
	if _, err = s.Detail(ctx, b, id); err == nil {
		t.Fatal("read another account")
	}
	if err = s.Action(ctx, b, id, "cancel", 0); err == nil {
		t.Fatal("changed another account")
	}
	current, _ := s.Claim(ctx, time.Now())
	if err = s.Action(ctx, a, id, "cancel", 0); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(ctx, *current, packing.Result{State: "completed"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	job, _ := s.Detail(ctx, a, id)
	if job.State != "cancelled" || job.Counts["completed"] != 1 || job.Counts["cancelled"] != 1 {
		t.Fatal(job)
	}
	if next, _ := s.Claim(ctx, time.Now().Add(time.Hour)); next != nil {
		t.Fatal("cancelled remainder submitted")
	}
}
func TestPackingQueueGlobalCooldownAndFailureReview(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, _ := Create(db, "a")
	b, _ := Create(db, "b")
	s := PackingJobs{db}
	id := createPackingFixture(t, s, a, 1)
	createPackingFixture(t, s, b, 1)
	now := time.Now()
	first, _ := s.Claim(ctx, now)
	if err = s.Finish(ctx, *first, packing.Result{State: "failed", Message: "fixture rejected", Archive: "receipt"}, now); err != nil {
		t.Fatal(err)
	}
	if next, _ := s.Claim(ctx, now.Add(time.Second)); next != nil {
		t.Fatal("next task bypassed cooldown")
	}
	if err = s.Action(ctx, a, id, "retry", first.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.Action(ctx, a, id, "resume", 0); err != nil {
		t.Fatal(err)
	}
	retry, _ := s.Claim(ctx, now.Add(5*time.Second))
	if retry == nil || retry.ID != first.ID {
		t.Fatal("retry didn't preserve request")
	}
}

func TestUnknownResultStopsAllFollowingTasksUntilReviewed(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, _ := Create(db, "a")
	b, _ := Create(db, "b")
	s := PackingJobs{db}
	id := createPackingFixture(t, s, a, 1)
	createPackingFixture(t, s, b, 1)
	now := time.Now()
	first, _ := s.Claim(ctx, now)
	if err = s.Finish(ctx, *first, packing.Result{State: "unknown"}, now); err != nil {
		t.Fatal(err)
	}
	if next, _ := s.Claim(ctx, now.Add(time.Hour)); next != nil {
		t.Fatal("unknown write was treated as finished")
	}
	if err = s.Action(ctx, a, id, "cancel", 0); err != nil {
		t.Fatal(err)
	}
	if next, _ := s.Claim(ctx, now.Add(time.Hour)); next != nil {
		t.Fatal("cancel allowed overlap with unknown write")
	}
	if err = s.Action(ctx, a, id, "confirm-completed", first.ID); err != nil {
		t.Fatal(err)
	}
	if next, _ := s.Claim(ctx, now.Add(time.Hour)); next == nil || next.Account != b {
		t.Fatal("queue did not continue after review")
	}
}
