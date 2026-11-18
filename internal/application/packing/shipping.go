package packing

import (
	"context"
	"fmt"
	"order_processor/internal/domain/packing"
)

type ShippingRepository interface {
	ShippingLabel(context.Context, string, string) (packing.ShippingLabel, error)
	SaveShippingLabel(context.Context, string, string, packing.ShippingLabel) error
}

type ShippingGateway interface {
	InspectLabel(context.Context, string, string) (packing.ShippingLabel, error)
	ApplyLabel(context.Context, string, string) (packing.ShippingLabel, error)
}

// ShippingFirst runs inside the existing single-package submission lock.
type ShippingFirst struct {
	Repository ShippingRepository
	Gateway    ShippingGateway
	Next       Submitter
}

func (s ShippingFirst) Submit(ctx context.Context, account string, in packing.Request) (packing.Result, error) {
	label, err := s.ensure(ctx, account, in.PackageID)
	if err != nil {
		return packing.Result{State: "not_sent", Message: "运单步骤未完成，未提交代打包：" + err.Error()}, nil
	}
	if label.State != "completed" {
		return packing.Result{State: "not_sent", Message: "运单步骤未完成，未提交代打包：" + label.Message, Archive: label.Archive}, nil
	}
	return s.Next.Submit(ctx, account, in)
}

func (s ShippingFirst) ensure(ctx context.Context, account, packageID string) (packing.ShippingLabel, error) {
	previous, err := s.Repository.ShippingLabel(ctx, account, packageID)
	if err != nil || previous.State == "completed" {
		return previous, err
	}
	current, err := s.Gateway.InspectLabel(ctx, account, packageID)
	if err != nil {
		return current, err
	}
	if current.State == "completed" {
		return current, s.Repository.SaveShippingLabel(context.WithoutCancel(ctx), account, packageID, current)
	}
	if current.State != "ready" {
		return current, nil
	}
	// An interrupted request may have reached ERP. Only a confirmed label can
	// resolve it automatically; an empty lookup is not permission to apply twice.
	if previous.State == "applying" || previous.State == "unknown" {
		return packing.ShippingLabel{State: "pending", Message: "上次申请结果待核对；请在妙手确认运单成功后重试，本系统不会重复申请。", Archive: previous.Archive}, nil
	}
	if err = s.Repository.SaveShippingLabel(ctx, account, packageID, packing.ShippingLabel{State: "applying"}); err != nil {
		return current, err
	}
	result, err := s.Gateway.ApplyLabel(ctx, account, packageID)
	if err != nil {
		result = packing.ShippingLabel{State: "unknown", Message: err.Error()}
	}
	if err = s.Repository.SaveShippingLabel(context.WithoutCancel(ctx), account, packageID, result); err != nil {
		return result, fmt.Errorf("申请结果保存失败，请在妙手核对：%w", err)
	}
	return result, nil
}
