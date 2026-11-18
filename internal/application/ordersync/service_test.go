package ordersync

import (
	"context"
	"errors"
	"order_processor/internal/domain/orders"
	"testing"
	"time"
)

type sourceFunc func(context.Context, func(string)) (orders.Batch, error)

func (f sourceFunc) Fetch(ctx context.Context, progress func(string)) (orders.Batch, error) {
	return f(ctx, progress)
}

type repositoryFunc func(context.Context, string, orders.Batch) (orders.ImportResult, error)

func (f repositoryFunc) ImportBatch(ctx context.Context, account string, b orders.Batch) (orders.ImportResult, error) {
	return f(ctx, account, b)
}
func completed(t *testing.T, s *Service) Job {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		job := s.Status("1")
		if job.State != "running" {
			return job
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("job did not finish")
	return Job{}
}
func TestSourceAndRepositoryAreIndependent(t *testing.T) {
	imported := make(chan orders.Batch, 1)
	service := New(repositoryFunc(func(ctx context.Context, account string, b orders.Batch) (orders.ImportResult, error) {
		if account != "1" {
			t.Error(account)
		}
		imported <- b
		return orders.ImportResult{Packages: len(b.Records)}, nil
	}), nil)
	source := sourceFunc(func(ctx context.Context, progress func(string)) (orders.Batch, error) {
		progress("读取本地业务输入")
		return orders.Batch{Source: "another-channel", Records: []orders.Record{{ID: "package-1", Fields: orders.Fields{"quantity": 2}}}}, nil
	})
	_, err := service.Start("1", Run{Source: source, StartedAt: "2026-01-01T00:00:00Z", Archive: "capture/1"})
	if err != nil {
		t.Fatal(err)
	}
	job := completed(t, service)
	if job.State != "completed" || job.Result.Packages != 1 {
		t.Fatal(job)
	}
	batch := <-imported
	if batch.Source != "another-channel" || batch.CapturedAt != "2026-01-01T00:00:00Z" || batch.Archive != "capture/1" {
		t.Fatal(batch)
	}
}
func TestFailedAcquisitionDoesNotSubmitPartialOrders(t *testing.T) {
	service := New(repositoryFunc(func(context.Context, string, orders.Batch) (orders.ImportResult, error) {
		t.Error("failed source must not submit")
		return orders.ImportResult{}, nil
	}), nil)
	source := sourceFunc(func(context.Context, func(string)) (orders.Batch, error) {
		return orders.Batch{Records: []orders.Record{{ID: "partial"}}}, errors.New("incomplete page")
	})
	service.Start("1", Run{Source: source})
	job := completed(t, service)
	if job.State != "failed" || job.Error != "incomplete page" {
		t.Fatal(job)
	}
}
