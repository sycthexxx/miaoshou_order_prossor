package web

import (
	"context"
	"encoding/json"
	"fmt"
)

func (s PackingSubmitter) AmountDetail(ctx context.Context, account, packageID, orderID string) (json.RawMessage, error) {
	raw, err := s.request(ctx, account, "amount-detail", map[string]string{"packageId": packageID, "orderId": orderID})
	if err != nil {
		return nil, err
	}
	if raw.Error != "" {
		return nil, fmt.Errorf("%s", raw.Error)
	}
	if raw.Status != 200 || !json.Valid([]byte(raw.Body)) {
		return nil, fmt.Errorf("金额明细查询失败，请确认妙手登录状态后重试")
	}
	return json.RawMessage(raw.Body), nil
}
