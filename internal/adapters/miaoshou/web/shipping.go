package web

import (
	"context"
	"encoding/json"
	"fmt"
	"order_processor/internal/domain/packing"
	"strings"
)

func (s PackingSubmitter) labelRequest(ctx context.Context, account, packageID, action string) (browserResponse, error) {
	return s.request(ctx, account, "waybill", map[string]string{"action": action, "packageId": packageID})
}

func (s PackingSubmitter) InspectLabel(ctx context.Context, account, packageID string) (packing.ShippingLabel, error) {
	raw, err := s.labelRequest(ctx, account, packageID, "inspect")
	if err != nil {
		return packing.ShippingLabel{}, err
	}
	return inspectLabel(raw, packageID), nil
}

func inspectLabel(raw browserResponse, packageID string) packing.ShippingLabel {
	r := packing.ShippingLabel{State: "not_sent", Message: "无法读取最新运单状态，请检查妙手登录状态后重试。", Archive: raw.Archive}
	if raw.Error != "" {
		r.Message = raw.Error
		return r
	}
	var data struct {
		Result  string `json:"result"`
		Package struct {
			ID        string `json:"opOrderPackageId"`
			Number    string `json:"logisticsNo"`
			AppliedAt string `json:"gmtApplyLogisticsNo"`
			Failed    string `json:"isApplyTrackingNoFail"`
		} `json:"orderPackageDetailInfo"`
	}
	if json.Unmarshal([]byte(raw.Body), &data) != nil || raw.Status != 200 || data.Result != "success" || data.Package.ID != packageID {
		return r
	}
	if n := strings.TrimSpace(data.Package.Number); n != "" {
		r.State = "completed"
		r.Number = n
		r.Message = "妙手已有运单号，跳过申请。"
	} else if data.Package.AppliedAt != "" && data.Package.Failed != "1" {
		r.State = "pending"
		r.Message = "妙手运单正在申请或结果尚未返回，请稍后重试。"
	} else {
		r.State = "ready"
		r.Message = "尚未申请运单。"
	}
	return r
}

func (s PackingSubmitter) ApplyLabel(ctx context.Context, account, packageID string) (packing.ShippingLabel, error) {
	// Match ERP's preflight: a previously cancelled label may still exist at
	// the carrier. Never bypass its duplicate-charge warning automatically.
	check, err := s.labelRequest(ctx, account, packageID, "check")
	if err != nil {
		return packing.ShippingLabel{State: "not_sent", Message: err.Error()}, nil
	}
	var checked struct {
		Result string `json:"result"`
		Errors []any  `json:"errors"`
	}
	if check.Error != "" || check.Status != 200 || json.Unmarshal([]byte(check.Body), &checked) != nil || checked.Result != "success" || len(checked.Errors) > 0 {
		return packing.ShippingLabel{State: "not_sent", Message: "运单申请前检查未通过，请在妙手处理后重试。", Archive: check.Archive}, nil
	}
	raw, err := s.labelRequest(ctx, account, packageID, "apply")
	if err != nil {
		return packing.ShippingLabel{State: "not_sent", Message: err.Error()}, nil
	}
	if raw.Error != "" {
		return packing.ShippingLabel{State: raw.State, Message: raw.Error, Archive: raw.Archive}, nil
	}
	r := packingResult(raw.Status, raw.Body, raw.Archive)
	label := packing.ShippingLabel{State: r.State, Archive: r.Archive}
	if r.State == "completed" && r.SuccessCount == 1 {
		label.Message = "运单申请成功。"
		return label, nil
	}
	if r.State == "completed" || r.State == "partial" {
		label.State = "unknown"
	}
	label.Message = "申请运单未确认成功，请在妙手核对后重试。"
	if details, ok := r.Details.([]any); ok && len(details) > 0 {
		b, _ := json.Marshal(r.Details)
		label.Message = fmt.Sprintf("申请运单失败：%s", b)
	}
	return label, nil
}
