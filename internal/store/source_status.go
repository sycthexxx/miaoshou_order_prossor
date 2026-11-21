package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

var ErrSourceStatusInput = errors.New("货源状态参数不正确")

type SourceStatus struct {
	State     string `json:"state"`
	Reason    string `json:"reason"`
	CheckedAt string `json:"checkedAt"`
	Source    string `json:"source"`
}
type SourceStatusInput struct {
	State    string `json:"state"`
	Reason   string `json:"reason"`
	Source   string `json:"source"`
	Evidence string `json:"evidence,omitempty"`
}
type SourceStatusResult struct {
	Status         SourceStatus `json:"sourceStatus"`
	Changed        bool         `json:"changed"`
	RetainedManual bool         `json:"retainedManual,omitempty"`
}

func unknownSourceStatus() SourceStatus { return SourceStatus{State: "unknown"} }
func migrateSourceStatuses(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS source_statuses(id INTEGER PRIMARY KEY AUTOINCREMENT,account_id INTEGER NOT NULL REFERENCES accounts(id),platform TEXT NOT NULL,offer_id TEXT NOT NULL,state TEXT NOT NULL CHECK(state IN('unknown','valid','invalid')),reason TEXT NOT NULL,checked_at TEXT NOT NULL,origin TEXT NOT NULL CHECK(origin IN('plugin','manual')),UNIQUE(account_id,platform,offer_id))`)
	return err
}
func readSourceStatus(q mappingQuerier, account, offer string) (SourceStatus, error) {
	s := unknownSourceStatus()
	err := q.QueryRow(`SELECT state,reason,checked_at,origin FROM source_statuses WHERE account_id=? AND platform='1688' AND offer_id=?`, account, offer).Scan(&s.State, &s.Reason, &s.CheckedAt, &s.Source)
	if err == sql.ErrNoRows {
		return unknownSourceStatus(), nil
	}
	return s, err
}
func GetSourceStatus(db *sql.DB, account, offer string) (SourceStatus, error) {
	if err := requireProcurementSource(db, account, offer); err != nil {
		return SourceStatus{}, err
	}
	return readSourceStatus(db, account, offer)
}
func SetSourceStatus(db *sql.DB, account, offer string, in SourceStatusInput) (SourceStatusResult, error) {
	result := SourceStatusResult{}
	in.Reason = strings.TrimSpace(in.Reason)
	if (in.State != "unknown" && in.State != "valid" && in.State != "invalid") || (in.Source != "plugin" && in.Source != "manual") || len([]rune(in.Reason)) > 200 {
		return result, ErrSourceStatusInput
	}
	if in.Source == "plugin" && in.State == "invalid" && in.Evidence != "offer_removed" && in.Evidence != "offer_not_found" {
		return result, ErrSourceStatusInput
	}
	tx, err := db.Begin()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if err = requireProcurementSource(tx, account, offer); err != nil {
		return result, err
	}
	previous, err := readSourceStatus(tx, account, offer)
	if err != nil {
		return result, err
	}
	result.Status = previous
	// Unknown means an inconclusive page read, not evidence that a known source
	// changed. A human invalidation can only be changed by another manual action.
	if in.Source == "plugin" && (in.State == "unknown" || (previous.State == "invalid" && previous.Source == "manual")) {
		result.RetainedManual = previous.State == "invalid" && previous.Source == "manual"
		return result, nil
	}
	if in.State == "invalid" && in.Reason == "" {
		if in.Source == "manual" {
			in.Reason = "人工标记无效"
		} else {
			in.Reason = in.Evidence
		}
	}
	result.Changed = previous.State != in.State || previous.Reason != in.Reason || previous.Source != in.Source
	result.Status = SourceStatus{State: in.State, Reason: in.Reason, Source: in.Source, CheckedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	_, err = tx.Exec(`INSERT INTO source_statuses(account_id,platform,offer_id,state,reason,checked_at,origin) VALUES(?,'1688',?,?,?,?,?) ON CONFLICT(account_id,platform,offer_id) DO UPDATE SET state=excluded.state,reason=excluded.reason,checked_at=excluded.checked_at,origin=excluded.origin`, account, offer, result.Status.State, result.Status.Reason, result.Status.CheckedAt, result.Status.Source)
	if err != nil {
		return SourceStatusResult{}, err
	}
	if err = tx.Commit(); err != nil {
		return SourceStatusResult{}, err
	}
	return result, nil
}
func attachSourceStatuses(db rowsQuerier, account string, report object) error {
	rows, err := db.Query(`SELECT platform,offer_id,state,reason,checked_at,origin FROM source_statuses WHERE account_id=?`, account)
	if err != nil {
		return err
	}
	defer rows.Close()
	statuses := map[string]SourceStatus{}
	for rows.Next() {
		var platform, offer string
		var s SourceStatus
		if err = rows.Scan(&platform, &offer, &s.State, &s.Reason, &s.CheckedAt, &s.Source); err != nil {
			return err
		}
		statuses[platform+":"+offer] = s
	}
	if err = rows.Err(); err != nil {
		return err
	}
	for _, g := range report["groups"].([]object) {
		source, _ := g["source"].(object)
		status, ok := statuses[text(source["source"])+":"+text(source["sourceItemId"])]
		if !ok {
			status = unknownSourceStatus()
		}
		g["sourceStatus"] = status
	}
	return nil
}
