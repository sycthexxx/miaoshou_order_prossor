package packing

import (
	"context"
	"order_processor/internal/domain/packing"
	"time"
)

type JobRepository interface {
	Recover(context.Context) error
	Claim(context.Context, time.Time) (*packing.JobItem, error)
	Finish(context.Context, packing.JobItem, packing.Result, time.Time) error
}

// One runner submits one package at a time. The database owns all queue state.
type Queue struct {
	Repository JobRepository
	Submitter  Submitter
}

func (q Queue) Step(ctx context.Context, now time.Time) (bool, error) {
	item, err := q.Repository.Claim(ctx, now)
	if err != nil || item == nil {
		return false, err
	}
	result, err := q.Submitter.Submit(ctx, item.Account, item.Request)
	if err != nil {
		result = packing.Result{State: "not_sent", Message: err.Error()}
	}
	return true, q.Repository.Finish(context.WithoutCancel(ctx), *item, result, time.Now())
}
func (q Queue) Run(ctx context.Context) error {
	if err := q.Repository.Recover(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			call, cancel := context.WithTimeout(ctx, 90*time.Second)
			_, err := q.Step(call, now)
			cancel()
			if err != nil {
				return err
			}
		}
	}
}
