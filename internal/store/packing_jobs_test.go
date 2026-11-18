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
	if job.State != "queued" || job.Counts["unknown"] != 1 || job.Counts["completed"] != 1 || job.Counts["pending"] != 1 {
		t.Fatal(job)
	}
	third, err := s.Claim(ctx, now.Add(time.Minute))
	if err != nil || third == nil || third.ID == second.ID {
		t.Fatal(third, err)
	}
	if err = s.Finish(ctx, *third, packing.Result{State: "completed"}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err = s.Action(ctx, account, id, "confirm-completed", second.ID); err != nil {
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
	retry, _ := s.Claim(ctx, now.Add(5*time.Second))
	if retry == nil || retry.ID != first.ID {
		t.Fatal("retry didn't preserve request")
	}
}

func TestUnknownResultAllowsFollowingTasksBeforeReview(t *testing.T) {
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
	next, err := s.Claim(ctx, now.Add(time.Hour))
	if err != nil || next == nil || next.Account != b {
		t.Fatal("subsequent job blocked", next, err)
	}
	if err = s.Action(ctx, a, id, "confirm-completed", first.ID); err != nil {
		t.Fatal(err)
	}
}

func TestEditedPackageRunsAfterUntriedSiblings(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, _ := Create(db, "a")
	s := PackingJobs{db}
	id := createPackingFixture(t, s, a, 3)
	now := time.Now()
	claim := func() *packing.JobItem {
		t.Helper()
		now = now.Add(time.Minute)
		i, e := s.Claim(ctx, now)
		if e != nil || i == nil {
			t.Fatal(i, e)
		}
		return i
	}
	finish := func(i *packing.JobItem, state string) {
		t.Helper()
		if err := s.Finish(ctx, *i, packing.Result{State: state}, now); err != nil {
			t.Fatal(err)
		}
	}
	first := claim()
	finish(first, "unknown")
	second := claim()
	edited := first.Request
	edited.Items[0].Waybills[0].Number = "corrected"
	if err := s.RetryEdited(ctx, a, id, first.ID, edited); err != nil {
		t.Fatal(err)
	}
	if err := s.RetryEdited(ctx, a, id, first.ID, edited); err == nil {
		t.Fatal("duplicate retry")
	}
	finish(second, "completed")
	third := claim()
	if third.ID == first.ID || third.ID == second.ID {
		t.Fatal("wrong order", third.ID)
	}
	finish(third, "completed")
	retry := claim()
	if retry.ID != first.ID || retry.Request.Items[0].Waybills[0].Number != "corrected" {
		t.Fatal(retry)
	}
	finish(retry, "completed")
	job, err := s.Detail(ctx, a, id)
	if err != nil || job.State != "completed" || job.Counts["completed"] != 3 {
		t.Fatal(job, err)
	}
}

func TestRetrySkippedFromCompletedJob(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, _ := Create(db, "a")
	s := PackingJobs{db}
	id := createPackingFixture(t, s, a, 1)
	now := time.Now()
	item, err := s.Claim(ctx, now)
	if err != nil || item == nil {
		t.Fatal(err)
	}
	if err = s.Finish(ctx, *item, packing.Result{State: "failed", Message: "fixture reason"}, now); err != nil {
		t.Fatal(err)
	}
	if err = s.Action(ctx, a, id, "skip", item.ID); err != nil {
		t.Fatal(err)
	}
	job, err := s.Detail(ctx, a, id)
	if err != nil || job.State != "completed" || job.Items[0].Result.Message != "fixture reason" {
		t.Fatal(job, err)
	}
	if err = s.Action(ctx, a, id, "retry", item.ID); err != nil {
		t.Fatal(err)
	}
	next, err := s.Claim(ctx, now.Add(time.Minute))
	if err != nil || next == nil || next.ID != item.ID {
		t.Fatal(next, err)
	}
	if err = s.Finish(ctx, *next, packing.Result{State: "completed", Message: "successful response"}, now); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err = db.QueryRow("SELECT result_json FROM packing_job_items WHERE id=?", next.ID).Scan(&raw); err != nil || raw != "{}" {
		t.Fatal(raw, err)
	}
}
