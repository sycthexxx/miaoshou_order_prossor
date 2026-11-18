package web

import (
	"context"
	"encoding/json"
	"errors"
)

func (s PackingSubmitter) LogisticsCompany(ctx context.Context, account, number string) (string, error) {
	raw, err := s.request(ctx, account, "logistics-company", map[string]string{"number": number})
	if err != nil {
		return "", err
	}
	var result struct {
		Company string `json:"company"`
	}
	if raw.Status != 200 || raw.Error != "" || json.Unmarshal([]byte(raw.Body), &result) != nil {
		return "", errors.New("物流公司查询失败，请确认账户已登录妙手或手动填写")
	}
	return result.Company, nil
}
