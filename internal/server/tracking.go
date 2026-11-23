package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"order_processor/internal/adapters/miaoshou/web"
	"order_processor/internal/domain/orders"
	"order_processor/internal/store"
	"strings"
	"time"
)

func trackingPayload(kind, number string, p orders.Fields, purchases string) map[string]string {
	if kind == "platform" && strings.EqualFold(orders.Text(p["platform"]), "tiktok") {
		if tracking := orders.Text(p["logisticsNo"]); tracking != "" && tracking != "0" && tracking != "--" {
			return map[string]string{"kind": kind, "number": tracking}
		}
	}
	if kind != "domestic" || number == "" || number == "0" || number == "--" {
		return nil
	}
	var records []struct {
		Number    string
		Shipments []struct{ Number, Company, TraceID string }
	}
	if json.Unmarshal([]byte(purchases), &records) != nil {
		return nil
	}
	for _, purchase := range records {
		if number == purchase.Number {
			continue
		}
		for _, shipment := range purchase.Shipments {
			if shipment.Number == number && shipment.TraceID != "" && shipment.TraceID != "0" {
				return map[string]string{"kind": kind, "number": number, "traceId": shipment.TraceID, "company": shipment.Company}
			}
		}
	}
	return nil
}

func (g *browserGateway) tracking(w http.ResponseWriter, r *http.Request) {
	account, packageID := r.PathValue("id"), r.PathValue("package")
	var payload, purchases string
	err := g.db.QueryRowContext(r.Context(), `SELECT p.payload,COALESCE(l.purchases,'[]')
		FROM packages p LEFT JOIN package_purchase_logistics l ON l.account_id=p.account_id AND l.package_id=p.package_id
		WHERE p.account_id=? AND p.package_id=?`, account, packageID).Scan(&payload, &purchases)
	if errors.Is(err, sql.ErrNoRows) {
		send(w, 404, map[string]string{"error": "当前账户中没有该包裹"})
		return
	}
	var p orders.Fields
	if err != nil || orders.Decode([]byte(payload), &p) != nil {
		send(w, 500, map[string]string{"error": "读取包裹物流信息失败"})
		return
	}
	query := trackingPayload(r.URL.Query().Get("kind"), r.URL.Query().Get("number"), p, purchases)
	if query == nil {
		send(w, 422, map[string]string{"error": "妙手暂未提供这个运单的查询信息"})
		return
	}
	query["packageId"] = packageID
	if query["kind"] == "domestic" {
		trace, status, err := g.syncDomesticTracking(r.Context(), account, query)
		if err != nil {
			send(w, status, map[string]string{"error": err.Error()})
			return
		}
		send(w, 200, trace)
		return
	}
	data, err := g.trackingAdapter().Tracking(r.Context(), account, query)
	if err != nil {
		send(w, 502, map[string]string{"error": err.Error()})
		return
	}
	send(w, 200, data)
}

type domesticTrace struct {
	Number string                 `json:"number"`
	Steps  []store.TrackingStep   `json:"steps"`
	Latest store.DomesticTracking `json:"latest"`
}

func (g *browserGateway) trackingAdapter() web.PackingSubmitter {
	return web.PackingSubmitter{Runtime: g.runtime, Client: g.client, StorageKey: func(id string) string { return store.StorageKey(g.db, id) }}
}

func (g *browserGateway) syncDomesticTracking(ctx context.Context, account string, query map[string]string) (domesticTrace, int, error) {
	requestedAt := time.Now()
	var trace domesticTrace
	data, err := g.trackingAdapter().Tracking(ctx, account, query)
	if err != nil {
		return trace, 502, err
	}
	if err := json.Unmarshal(data, &trace); err != nil || trace.Number != query["number"] || trace.Steps == nil {
		return trace, 502, errors.New("妙手未返回有效的国内物流轨迹")
	}
	trace.Latest, err = store.SaveDomesticTracking(ctx, g.db, account, trace.Number, requestedAt, trace.Steps)
	if err != nil {
		return trace, 500, errors.New("保存头程物流状态失败")
	}
	return trace, 200, nil
}

func registerTrackingRules(mux *http.ServeMux, db *sql.DB) {
	const route = "/api/accounts/{id}/first-mile/tracking-rules"
	mux.HandleFunc("GET "+route, func(w http.ResponseWriter, r *http.Request) {
		rules, err := store.DomesticTrackingRules(r.Context(), db, r.PathValue("id"))
		if err != nil {
			send(w, 500, map[string]string{"error": "读取物流判定规则失败"})
			return
		}
		send(w, 200, rules)
	})
	mux.HandleFunc("PUT "+route, func(w http.ResponseWriter, r *http.Request) {
		if !currentUser(r).IsAdmin {
			send(w, 403, map[string]string{"error": "只有管理员可以修改物流判定规则"})
			return
		}
		var input []store.TrackingRule
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 262144)).Decode(&input) != nil {
			send(w, 400, map[string]string{"error": "物流判定规则格式无效"})
			return
		}
		rules, err := store.NormalizeTrackingRules(input)
		if err != nil {
			send(w, 400, map[string]string{"error": err.Error()})
			return
		}
		if err := store.SaveDomesticTrackingRules(r.Context(), db, r.PathValue("id"), rules); err != nil {
			send(w, 500, map[string]string{"error": "保存物流判定规则失败"})
			return
		}
		send(w, 200, rules)
	})
}
