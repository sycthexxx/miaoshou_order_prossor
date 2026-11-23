package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

type TrackingRule struct {
	Status   string   `json:"status"`
	Keywords []string `json:"keywords"`
	Exclude  []string `json:"exclude"`
}

func defaultTrackingRules() []TrackingRule {
	return []TrackingRule{{
		Status:   "已送达",
		Keywords: []string{"签收", "已送达", "送达成功", "成功送达", "妥投", "本人取件", "已由+收取", "已由+取件"},
		Exclude: []string{"未签收", "待签收", "取消签收", "签收失败", "未送达", "待送达", "预计送达",
			"即将送达", "正在送达", "送达失败", "无法送达", "未能送达",
			"请本人取件", "待本人取件", "请签收", "请及时签收"},
	}}
}

func DomesticTrackingRules(ctx context.Context, db *sql.DB, account string) ([]TrackingRule, error) {
	var data string
	err := db.QueryRowContext(ctx, `SELECT rules FROM domestic_tracking_rules WHERE account_id=?`, account).Scan(&data)
	if err == sql.ErrNoRows {
		return defaultTrackingRules(), nil
	}
	if err != nil {
		return nil, err
	}
	var rules []TrackingRule
	err = json.Unmarshal([]byte(data), &rules)
	return rules, err
}

func NormalizeTrackingRules(rules []TrackingRule) ([]TrackingRule, error) {
	if rules == nil || len(rules) > 32 {
		return nil, fmt.Errorf("最多可配置 32 条状态规则")
	}
	for i := range rules {
		rule := &rules[i]
		rule.Status = strings.TrimSpace(rule.Status)
		if rule.Status == "" || utf8.RuneCountInString(rule.Status) > 24 || len(rule.Keywords) == 0 ||
			len(rule.Keywords) > 64 || len(rule.Exclude) > 64 {
			return nil, fmt.Errorf("第 %d 条规则需填写状态和至少一个关键词", i+1)
		}
		for j, pattern := range rule.Keywords {
			parts := strings.Split(pattern, "+")
			for k, part := range parts {
				parts[k] = strings.TrimSpace(part)
				if parts[k] == "" || utf8.RuneCountInString(parts[k]) > 64 {
					return nil, fmt.Errorf("第 %d 条规则含无效关键词", i+1)
				}
			}
			rule.Keywords[j] = strings.Join(parts, "+")
		}
		for j, phrase := range rule.Exclude {
			rule.Exclude[j] = strings.TrimSpace(phrase)
			if rule.Exclude[j] == "" || utf8.RuneCountInString(rule.Exclude[j]) > 64 {
				return nil, fmt.Errorf("第 %d 条规则含无效排除词", i+1)
			}
		}
	}
	return rules, nil
}

func SaveDomesticTrackingRules(ctx context.Context, db *sql.DB, account string, rules []TrackingRule) error {
	body, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO domestic_tracking_rules(account_id,rules) VALUES(?,?)
		ON CONFLICT(account_id) DO UPDATE SET rules=excluded.rules`, account, string(body))
	return err
}

func trackingRuleStatus(detail string, rules []TrackingRule) string {
	for _, rule := range rules {
		match := detail
		for _, phrase := range rule.Exclude {
			match = strings.ReplaceAll(match, phrase, " ")
		}
		for _, pattern := range rule.Keywords {
			tail := match
			matched := true
			for _, part := range strings.Split(pattern, "+") {
				_, after, found := strings.Cut(tail, part)
				if !found {
					matched = false
					break
				}
				tail = after
			}
			if matched {
				return rule.Status
			}
		}
	}
	return ""
}
