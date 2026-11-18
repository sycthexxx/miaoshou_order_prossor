package packing

import (
	"errors"
	"fmt"
)

type Selection struct {
	PackageID string `json:"packageId"`
	ItemID    string `json:"itemId"`
}
type SKUImage struct {
	OrderItemID string `json:"opOrderItemId"`
	Title       string `json:"title,omitempty"`
	Thumbnail   string `json:"picUrlThumb,omitempty"`
	URL         string `json:"picUrl,omitempty"`
	Original    string `json:"originalPicUrl,omitempty"`
}
type Candidate struct {
	Image *SKUImage `json:"image,omitempty"`
	Selection
	PackageNo string `json:"packageNo"`
	OrderNo   string `json:"orderNo,omitempty"`
	Spec      string `json:"spec"`
	SKU       string `json:"sku"`
	Quantity  int64  `json:"quantity"`
	Gift      bool   `json:"gift"`
}
type BatchInput struct {
	Name            string      `json:"name"`
	SourceKey       string      `json:"sourceKey"`
	Gifts           bool        `json:"gifts"`
	IntervalSeconds int         `json:"intervalSeconds"`
	Selections      []Selection `json:"selections"`
	Info            Item        `json:"info"`
}
type JobItem struct {
	ID      int64       `json:"id"`
	JobID   int64       `json:"jobId"`
	Account string      `json:"-"`
	Request Request     `json:"request"`
	Lines   []Candidate `json:"lines"`
	State   string      `json:"state"`
	Result  *Result     `json:"result,omitempty"`
}
type Job struct {
	ID              int64          `json:"id"`
	Name            string         `json:"name"`
	SourceKey       string         `json:"sourceKey"`
	State           string         `json:"state"`
	IntervalSeconds int            `json:"intervalSeconds"`
	CreatedAt       string         `json:"createdAt"`
	UpdatedAt       string         `json:"updatedAt"`
	Total           int            `json:"total"`
	Counts          map[string]int `json:"counts"`
	Items           []JobItem      `json:"items,omitempty"`
	PackingInfos    []Item         `json:"packingInfos,omitempty"`
}

// Freeze exactly the selected source lines and combine items in the same package.
func PrepareBatch(in BatchInput, candidates []Candidate) ([]JobItem, error) {
	if in.IntervalSeconds < 3 || in.IntervalSeconds > 300 {
		return nil, errors.New("提交间隔应为 3 至 300 秒")
	}
	if len(in.Selections) == 0 {
		return nil, errors.New("请勾选要应用的订单商品")
	}
	available := map[Selection]Candidate{}
	for _, line := range candidates {
		available[line.Selection] = line
	}
	seen := map[Selection]bool{}
	positions := map[string]int{}
	items := []JobItem{}
	for _, selection := range in.Selections {
		if seen[selection] {
			return nil, errors.New("订单商品重复勾选")
		}
		seen[selection] = true
		line, ok := available[selection]
		if !ok {
			return nil, fmt.Errorf("包裹 %s 的商品不再属于当前选择范围的采购需求，请刷新后重选", selection.PackageID)
		}
		index, ok := positions[selection.PackageID]
		if !ok {
			index = len(items)
			positions[selection.PackageID] = index
			items = append(items, JobItem{Request: Request{PackageID: selection.PackageID}, Lines: []Candidate{}, State: "pending"})
		}
		item := in.Info
		item.OrderItemID = selection.ItemID
		items[index].Request.Items = append(items[index].Request.Items, item)
		items[index].Lines = append(items[index].Lines, line)
	}
	for _, item := range items {
		if err := item.Request.Validate(); err != nil {
			return nil, err
		}
	}
	return items, nil
}
