package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

type DomesticTracking struct {
	Status   string     `json:"status"`
	Time     string     `json:"time,omitempty"`
	SyncedAt *time.Time `json:"syncedAt,omitempty"`
}

type TrackingStep struct {
	Time        string `json:"time"`
	Description string `json:"description"`
	Detail      string `json:"detail"`
}

func trackingEventTime(value string) int64 {
	if date, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return date.UnixMilli()
	}
	location := time.FixedZone("CST", 8*60*60)
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if date, err := time.ParseInLocation(layout, value, location); err == nil {
			return date.UnixMilli()
		}
	}
	return 0
}

func currentDomesticTracking(steps []TrackingStep, rules []TrackingRule) (DomesticTracking, int64) {
	if len(steps) == 0 {
		return DomesticTracking{Status: "已打包"}, 0
	}
	latest := steps[0]
	eventAt := trackingEventTime(latest.Time)
	for _, step := range steps[1:] {
		if at := trackingEventTime(step.Time); at > eventAt {
			latest, eventAt = step, at
		}
	}
	status := trackingRuleStatus(latest.Description+" "+latest.Detail, rules)
	if status == "" {
		status = "已揽收"
		if len(steps) > 1 {
			status = "运输中"
		}
	}
	return DomesticTracking{Status: status, Time: latest.Time}, eventAt
}

// A trace belongs to the domestic waybill, so packages sharing it see the same snapshot.
func SaveDomesticTracking(ctx context.Context, db *sql.DB, account, number string, requestedAt time.Time, steps []TrackingStep) (DomesticTracking, error) {
	rules, err := DomesticTrackingRules(ctx, db, account)
	if err != nil {
		return DomesticTracking{}, err
	}
	state, eventAt := currentDomesticTracking(steps, rules)
	_, err = db.ExecContext(ctx, `INSERT INTO domestic_tracking(account_id,number,status,event_time,event_at,requested_at)
		VALUES(?,?,?,?,?,?) ON CONFLICT(account_id,number) DO UPDATE SET status=excluded.status,
		event_time=excluded.event_time,event_at=excluded.event_at,requested_at=excluded.requested_at
		WHERE excluded.event_at>domestic_tracking.event_at OR
		(excluded.event_at=domestic_tracking.event_at AND excluded.requested_at>=domestic_tracking.requested_at)`,
		account, number, state.Status, state.Time, eventAt, requestedAt.UnixNano())
	if err != nil {
		return DomesticTracking{}, err
	}
	var synced int64
	err = db.QueryRowContext(ctx, `SELECT status,event_time,requested_at FROM domestic_tracking WHERE account_id=? AND number=?`, account, number).
		Scan(&state.Status, &state.Time, &synced)
	if err == nil {
		at := time.Unix(0, synced).UTC()
		state.SyncedAt = &at
	}
	return state, err
}

func domesticTrackingStates(db rowsQuerier, account string) (map[string]DomesticTracking, error) {
	rows, err := db.Query(`SELECT number,status,event_time,requested_at FROM domestic_tracking WHERE account_id=?`, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := map[string]DomesticTracking{}
	for rows.Next() {
		var number string
		var state DomesticTracking
		var synced int64
		if err := rows.Scan(&number, &state.Status, &state.Time, &synced); err != nil {
			return nil, err
		}
		at := time.Unix(0, synced).UTC()
		state.SyncedAt = &at
		states[number] = state
	}
	return states, rows.Err()
}

func withDomesticTracking(raw json.RawMessage, states map[string]DomesticTracking) (json.RawMessage, error) {
	var purchases []firstMilePurchase
	if err := json.Unmarshal(raw, &purchases); err != nil {
		return nil, err
	}
	changed := false
	for i := range purchases {
		for j := range purchases[i].Shipments {
			shipment := &purchases[i].Shipments[j]
			if state, ok := states[shipment.Number]; ok && shipment.Number != purchases[i].Number {
				shipment.Tracking = &state
				changed = true
			}
		}
	}
	if !changed {
		return raw, nil
	}
	return json.Marshal(purchases)
}
