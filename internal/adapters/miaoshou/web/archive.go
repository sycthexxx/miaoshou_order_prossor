// Package web translates archived browser responses into domain input records.
// Browser automation lives in worker/ and its process/container runtime in browser/.
package web

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"

	"order_processor/internal/domain/orders"
)

const (
	ListEndpoint        = "/api/order/package/render_list/searchOrderPackageList"
	TranslationEndpoint = "/api/order/package/getTranslatedOpOrderPackageList"
	ProductEndpoint     = "/api/platform/tiktok/item/item/searchItemList"
	OutboundEndpoint    = "/api/order/package/getOpOrderPackageIdAndOutboundInfoMap"
)

type Decoder struct{}
type envelope struct {
	RequestBody string `json:"requestBody"`
	AccountID   string `json:"accountId"`
	Endpoint    string `json:"endpoint"`
	CapturedAt  string `json:"capturedAt"`
	Body        string `json:"bodyBase64"`
	Status      int    `json:"status"`
}

func parse(raw []byte, key string) (*envelope, orders.Fields, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, nil, err
	}
	switch env.Endpoint {
	case ListEndpoint, TranslationEndpoint, ProductEndpoint, OutboundEndpoint:
	default:
		return nil, nil, nil
	}
	if env.AccountID != key {
		return nil, nil, fmt.Errorf("archive account mismatch")
	}
	body, err := base64.StdEncoding.DecodeString(env.Body)
	if err != nil {
		return nil, nil, err
	}
	var data orders.Fields
	if err = orders.Decode(body, &data); err != nil {
		return nil, nil, err
	}
	if env.Status != 200 || orders.Text(data["result"]) != "success" {
		return nil, nil, nil
	}
	return &env, data, nil
}
func (Decoder) DecodeArchive(raw []byte, key, file string) (*orders.Archive, error) {
	env, data, err := parse(raw, key)
	if err != nil || env == nil {
		return nil, err
	}
	kinds := map[string]orders.Kind{ListEndpoint: orders.Packages, TranslationEndpoint: orders.Translations, ProductEndpoint: orders.Products, OutboundEndpoint: orders.Outbound}
	result := &orders.Archive{Source: "web", Kind: kinds[env.Endpoint], CapturedAt: env.CapturedAt}
	listKey := "packageList"
	if result.Kind == orders.Products {
		listKey = "itemList"
	}
	rows, ok := data[listKey].([]any)
	if result.Kind == orders.Outbound {
		m, valid := data["opOrderPackageIdAndOutboundInfoMap"].(map[string]any)
		ok = valid
		rows = []any{}
		for _, v := range m {
			rows = append(rows, v)
		}
	}
	if !ok {
		return nil, fmt.Errorf("missing %s: %s", listKey, file)
	}
	for _, v := range rows {
		p, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid record")
		}
		id := orders.Text(p["opOrderPackageId"])
		if (result.Kind == orders.Packages || result.Kind == orders.Translations) && id == "" {
			return nil, fmt.Errorf("missing package id")
		}
		result.Records = append(result.Records, orders.Record{ID: id, Fields: p})
	}
	if result.Kind == orders.Packages {
		if scope := singlePageScope(env.CapturedAt, file, env.RequestBody, data); scope != nil {
			result.Scopes = append(result.Scopes, *scope)
		}
	}
	return result, nil
}

// DecodeScope only accepts a complete, unfiltered pagination pass whose raw
// archives, declared IDs and totals agree. Invalid evidence never clears a scope.
func (Decoder) DecodeScope(raw []byte, key string, read func(string) ([]byte, error)) (*orders.Scope, error) {
	var m struct {
		Version  int      `json:"version"`
		Account  string   `json:"accountId"`
		Scope    string   `json:"scope"`
		At       string   `json:"capturedAt"`
		Total    int      `json:"total"`
		Archives []string `json:"archives"`
		IDs      []string `json:"packageIds"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if m.Version != 1 || m.Account != key || m.Total < 0 || len(m.Archives) == 0 || len(m.IDs) != m.Total {
		return nil, nil
	}
	ids := []string{}
	seen := map[string]bool{}
	size := 0
	for index, name := range m.Archives {
		if filepath.Base(name) != name {
			return nil, nil
		}
		raw, err := read(name)
		if err != nil {
			return nil, nil
		}
		env, data, err := parse(raw, key)
		if err != nil || env == nil || env.Endpoint != ListEndpoint || env.CapturedAt > m.At || RequestScope(env.RequestBody) != m.Scope {
			return nil, nil
		}
		q, _ := url.ParseQuery(env.RequestBody)
		page, _ := strconv.Atoi(q.Get("page"))
		pageSize, _ := strconv.Atoi(q.Get("pageSize"))
		total, err := strconv.Atoi(orders.Text(data["total"]))
		if index == 0 {
			size = pageSize
		}
		if err != nil || page != index+1 || pageSize <= 0 || size != pageSize || total != m.Total {
			return nil, nil
		}
		if page := orders.Text(data["page"]); page != "" && page != strconv.Itoa(index+1) {
			return nil, nil
		}
		list, ok := data["packageList"].([]any)
		expected := min(size, m.Total-index*size)
		if !ok || len(list) != expected {
			return nil, nil
		}
		for _, v := range list {
			p, ok := v.(map[string]any)
			if !ok {
				return nil, nil
			}
			id := orders.Text(p["opOrderPackageId"])
			if id == "" || seen[id] {
				return nil, nil
			}
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) != m.Total {
		return nil, nil
	}
	declared := map[string]bool{}
	for _, id := range m.IDs {
		if !seen[id] || declared[id] {
			return nil, nil
		}
		declared[id] = true
	}
	return &orders.Scope{Name: m.Scope, CapturedAt: m.At, Files: m.Archives, PackageIDs: ids}, nil
}
