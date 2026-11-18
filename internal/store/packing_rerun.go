package store

import (
	"context"
	"errors"
	"fmt"
	"order_processor/internal/domain/packing"
	"time"
)

type PackingRerunInput struct {
	IntervalSeconds int               `json:"intervalSeconds"`
	Requests        []packing.Request `json:"requests"`
}

// Reuse the saved scope even if these orders have left the procurement list.
func (s PackingJobs) RerunPreview(ctx context.Context, account string, id int64) (packing.Job, error) {
	job, err := s.Detail(ctx, account, id)
	if err != nil {
		return job, err
	}
	if job.State != "completed" {
		return job, errors.New("只有已完成任务可以修改并重新执行")
	}
	for index, item := range job.Items {
		job.Items[index], err = s.IncludePackageGifts(ctx, account, item)
		if err != nil {
			return job, err
		}
	}
	return job, nil
}

func (s PackingJobs) Rerun(ctx context.Context, account string, id int64, in PackingRerunInput) (int64, error) {
	original, err := s.RerunPreview(ctx, account, id)
	if err != nil {
		return 0, err
	}
	if in.IntervalSeconds < 3 || in.IntervalSeconds > 300 {
		return 0, errors.New("提交间隔应为 3 至 300 秒")
	}
	if len(in.Requests) != len(original.Items) {
		return 0, errors.New("重新执行的包裹范围与原任务不一致")
	}
	requests := map[string]packing.Request{}
	for _, request := range in.Requests {
		if err := request.Validate(); err != nil {
			return 0, err
		}
		if _, exists := requests[request.PackageID]; exists {
			return 0, errors.New("包裹重复")
		}
		requests[request.PackageID] = request
	}
	items := []packing.JobItem{}
	for _, previous := range original.Items {
		request, ok := requests[previous.Request.PackageID]
		if !ok || len(request.Items) != len(previous.Request.Items) {
			return 0, errors.New("重新执行的商品范围发生变化，请关闭表单重新打开")
		}
		ids := map[string]bool{}
		for _, item := range previous.Request.Items {
			ids[item.OrderItemID] = true
		}
		for _, item := range request.Items {
			if !ids[item.OrderItemID] {
				return 0, errors.New("重新执行包含原任务之外的商品")
			}
		}
		job := packing.JobItem{Request: request, Lines: previous.Lines}
		job = packing.IncludePackageGifts(job, previous.Lines)
		items = append(items, job)
	}
	name := fmt.Sprintf("%s · 重执行 #%d · %s", original.Name, id, time.Now().Format("150405"))
	return s.Create(ctx, account, packing.BatchInput{Name: name, SourceKey: original.SourceKey, IntervalSeconds: in.IntervalSeconds}, items)
}
