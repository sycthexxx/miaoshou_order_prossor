package packing

import (
	"context"
	"errors"
	"order_processor/internal/domain/packing"
	"testing"
	"time"
)

type singleRepo struct {
	startErr, finishErr error
	recorded            packing.Result
	started             bool
}

func (s *singleRepo) PackageItemIDs(context.Context, string, string) ([]string, error) {
	return []string{"i"}, nil
}
func (s *singleRepo) StartSingle(context.Context, string, packing.Request) (packing.JobItem, error) {
	s.started = true
	return packing.JobItem{JobID: 7, ID: 8}, s.startErr
}
func (s *singleRepo) Finish(ctx context.Context, _ packing.JobItem, r packing.Result, _ time.Time) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	s.recorded = r
	return s.finishErr
}

type singleSubmitFunc func(context.Context, string, packing.Request) (packing.Result, error)

func (f singleSubmitFunc) Submit(c context.Context, a string, r packing.Request) (packing.Result, error) {
	return f(c, a, r)
}
func TestSingleRecordingFailurePaths(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		startErr, submitErr, finishErr error
		state, want                    string
	}{
		{name: "save before sending failed", startErr: errors.New("db error")},
		{name: "not sent", submitErr: errors.New("browser unavailable"), want: "not_sent"},
		{name: "unknown response", state: "unknown", want: "unknown"},
		{name: "result persistence failed", state: "completed", finishErr: errors.New("db error"), want: "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			repo := &singleRepo{startErr: tc.startErr, finishErr: tc.finishErr}
			calls := 0
			svc := Service{Repository: repo, Recorder: repo, Submitter: singleSubmitFunc(func(context.Context, string, packing.Request) (packing.Result, error) {
				calls++
				if !repo.started {
					t.Fatal("sent before task saved")
				}
				cancel()
				return packing.Result{State: tc.state}, tc.submitErr
			})}
			result, err := svc.Submit(ctx, "1", packing.Request{PackageID: "p", Items: []packing.Item{{OrderItemID: "i", Waybills: []packing.Waybill{{Company: "c", Number: "n"}}}}})
			if tc.startErr != nil {
				if calls != 0 || err == nil {
					t.Fatal(calls, err)
				}
				return
			}
			if err != nil || calls != 1 || result.State != tc.want || result.TaskID != 7 || repo.recorded.TaskID != 7 {
				t.Fatal(result, repo.recorded, calls, err)
			}
		})
	}
}
