package packing_test

import (
	"context"
	app "order_processor/internal/application/packing"
	"order_processor/internal/domain/packing"
	"order_processor/internal/store"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type blockingSubmitter struct {
	started chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (s *blockingSubmitter) Submit(context.Context, string, packing.Request) (packing.Result, error) {
	s.calls.Add(1)
	close(s.started)
	<-s.release
	return packing.Result{State: "completed"}, nil
}
func TestQueueNeverSubmitsInParallel(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, _ := store.Create(db, "test")
	repo := store.PackingJobs{DB: db}
	items := []packing.JobItem{}
	for _, id := range []string{"1", "2", "3"} {
		items = append(items, packing.JobItem{Request: packing.Request{PackageID: id}, Lines: []packing.Candidate{{Selection: packing.Selection{PackageID: id, ItemID: id}}}})
	}
	if _, err = repo.Create(context.Background(), account, packing.BatchInput{Name: "serial", IntervalSeconds: 5}, items); err != nil {
		t.Fatal(err)
	}
	submitter := &blockingSubmitter{started: make(chan struct{}), release: make(chan struct{})}
	q := app.Queue{Repository: repo, Submitter: submitter}
	done := make(chan error, 1)
	go func() { _, err := q.Step(context.Background(), time.Now()); done <- err }()
	<-submitter.started
	var wg sync.WaitGroup
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, err := q.Step(context.Background(), time.Now().Add(time.Hour))
			if err != nil || claimed {
				t.Errorf("parallel claim: %v %v", claimed, err)
			}
		}()
	}
	wg.Wait()
	if submitter.calls.Load() != 1 {
		t.Fatal("parallel network writes")
	}
	close(submitter.release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}
