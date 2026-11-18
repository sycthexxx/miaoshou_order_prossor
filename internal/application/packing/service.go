package packing

import (
	"context"
	"fmt"
	"order_processor/internal/domain/packing"
	"time"
)

type Repository interface {
	PackageItemIDs(context.Context, string, string) ([]string, error)
}
type Submitter interface {
	Submit(context.Context, string, packing.Request) (packing.Result, error)
}
type SingleJobRecorder interface {
	StartSingle(context.Context, string, packing.Request) (packing.JobItem, error)
	Finish(context.Context, packing.JobItem, packing.Result, time.Time) error
}

type Service struct {
	Recorder   SingleJobRecorder
	Repository Repository
	Submitter  Submitter
}

func (s Service) Submit(ctx context.Context, account string, in packing.Request) (packing.Result, error) {
	if err := in.Validate(); err != nil {
		return packing.Result{}, err
	}
	ids, err := s.Repository.PackageItemIDs(ctx, account, in.PackageID)
	if err != nil {
		return packing.Result{}, err
	}
	available := map[string]bool{}
	for _, id := range ids {
		available[id] = true
	}
	for _, item := range in.Items {
		if !available[item.OrderItemID] {
			return packing.Result{}, fmt.Errorf("商品 %s 不属于当前账户的包裹 %s", item.OrderItemID, in.PackageID)
		}
	}
	if s.Recorder == nil {
		return s.Submitter.Submit(ctx, account, in)
	}
	job, err := s.Recorder.StartSingle(ctx, account, in)
	if err != nil {
		return packing.Result{}, err
	}
	result, submitErr := s.Submitter.Submit(ctx, account, in)
	if submitErr != nil {
		result = packing.Result{State: "not_sent", Message: submitErr.Error()}
	}
	result.TaskID = job.JobID
	if err := s.Recorder.Finish(context.WithoutCancel(ctx), job, result, time.Now()); err != nil {
		result.State = "unknown"
		result.Message = fmt.Sprintf("提交已发起，但任务结果保存失败，请核对任务 #%d：%v", job.JobID, err)
		return result, nil
	}
	return result, nil
}
