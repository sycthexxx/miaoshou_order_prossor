package store

import (
	"context"
	"order_processor/internal/domain/packing"
)

// Single submissions are recorded before the external write, like queue items.
// Their running state also prevents the queue from claiming another package.
func (s PackingJobs) StartSingle(ctx context.Context, account string, request packing.Request) (packing.JobItem, error) {
	candidates, err := s.packageLines(ctx, account, request.PackageID)
	if err != nil {
		return packing.JobItem{}, err
	}
	byID := map[string]packing.Candidate{}
	for _, line := range candidates {
		byID[line.ItemID] = line
	}
	lines := []packing.Candidate{}
	for _, i := range request.Items {
		lines = append(lines, byID[i.OrderItemID])
	}
	packageNo := lines[0].PackageNo
	if packageNo == "" {
		packageNo = request.PackageID
	}
	item := packing.JobItem{Account: account, Request: request, Lines: lines, State: "running"}
	id, err := s.create(ctx, account, packing.BatchInput{Name: "单包裹代打包 · " + packageNo, IntervalSeconds: 5}, []packing.JobItem{item}, "running", "running")
	if err != nil {
		return packing.JobItem{}, err
	}
	item.JobID = id
	err = s.DB.QueryRowContext(ctx, `SELECT id FROM packing_job_items WHERE job_id=?`, id).Scan(&item.ID)
	return item, err
}
