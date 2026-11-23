package web

import (
	"context"
	"encoding/json"
	"errors"
)

func (s PackingSubmitter) Tracking(ctx context.Context, account string, payload map[string]string) (json.RawMessage, error) {
	raw, err := s.request(ctx, account, "tracking", payload)
	if err != nil {
		return nil, err
	}
	if raw.Status != 200 || !json.Valid([]byte(raw.Body)) {
		return nil, errors.New("暂时无法读取物流轨迹，请检查妙手登录状态后重试")
	}
	return json.RawMessage(raw.Body), nil
}
