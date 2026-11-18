// Package ordersync coordinates order acquisition and submission to our business
// repository. It depends on ports, never a vendor SDK or a concrete database.
package ordersync

import (
	"context"
	"errors"
	"order_processor/internal/domain/orders"
	"sync"
	"time"
)

type Source interface {
	Fetch(context.Context, func(string)) (orders.Batch, error)
}
type Repository interface {
	ImportBatch(context.Context, string, orders.Batch) (orders.ImportResult, error)
}
type Run struct {
	Source     Source
	StartedAt  string
	Archive    string
	SaveResult func(any) error
}
type Job struct {
	State      string               `json:"state"`
	Step       string               `json:"step"`
	Error      string               `json:"error,omitempty"`
	StartedAt  string               `json:"startedAt,omitempty"`
	FinishedAt string               `json:"finishedAt,omitempty"`
	Archive    string               `json:"archive,omitempty"`
	Result     *orders.ImportResult `json:"result,omitempty"`
}
type task struct {
	job    Job
	cancel context.CancelFunc
}
type Service struct {
	mu          sync.Mutex
	tasks       map[string]*task
	repository  Repository
	afterImport func(string)
}

var ErrRunning = errors.New("该账户的数据同步正在进行")

func New(repository Repository, afterImport func(string)) *Service {
	return &Service{tasks: map[string]*task{}, repository: repository, afterImport: afterImport}
}
func (s *Service) Status(account string) Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t := s.tasks[account]; t != nil {
		return t.job
	}
	return Job{State: "idle"}
}
func (s *Service) Stop(account string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t := s.tasks[account]; t != nil && t.cancel != nil {
		t.cancel()
		t.job.Step = "正在停止"
	}
}
func (s *Service) Start(account string, run Run) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t := s.tasks[account]; t != nil && t.job.State == "running" {
		return Job{}, ErrRunning
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	t := &task{job: Job{State: "running", Step: "准备获取订单数据", StartedAt: run.StartedAt, Archive: run.Archive}, cancel: cancel}
	s.tasks[account] = t
	go s.execute(ctx, account, t, run)
	return t.job, nil
}
func (s *Service) execute(ctx context.Context, account string, t *task, run Run) {
	defer t.cancel()
	progress := func(step string) { s.mu.Lock(); t.job.Step = step; s.mu.Unlock() }
	batch, err := run.Source.Fetch(ctx, progress)
	var result orders.ImportResult
	if err == nil {
		batch.CapturedAt = run.StartedAt
		batch.Archive = run.Archive
		progress("正在合并订单与补充数据")
		result, err = s.repository.ImportBatch(ctx, account, batch)
	}
	if err == nil && s.afterImport != nil {
		s.afterImport(account)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t.cancel = nil
	t.job.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err != nil {
		t.job.State = "failed"
		t.job.Error = err.Error()
		t.job.Step = "未更新本地订单"
		if ctx.Err() != nil {
			t.job.State = "cancelled"
			t.job.Error = "同步已停止或超时"
		}
	} else {
		t.job.State = "completed"
		t.job.Step = "订单与采购分组已更新"
		t.job.Result = &result
	}
	if run.SaveResult != nil {
		if err := run.SaveResult(t.job); err != nil {
			t.job.Error = "同步结果文件保存失败"
		}
	}
}
