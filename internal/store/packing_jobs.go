package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"order_processor/internal/domain/packing"
	"strconv"
	"time"
)

type PackingJobs struct{ DB *sql.DB }

func migratePackingJobs(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS packing_jobs(
 id INTEGER PRIMARY KEY AUTOINCREMENT,account_id INTEGER NOT NULL REFERENCES accounts(id),name TEXT NOT NULL,source_key TEXT NOT NULL,
 state TEXT NOT NULL,interval_seconds INTEGER NOT NULL,next_run_at INTEGER NOT NULL DEFAULT 0,created_at TEXT NOT NULL,updated_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS packing_job_items(
 id INTEGER PRIMARY KEY AUTOINCREMENT,job_id INTEGER NOT NULL REFERENCES packing_jobs(id),package_id TEXT NOT NULL,
 request_json TEXT NOT NULL,lines_json TEXT NOT NULL,state TEXT NOT NULL,result_json TEXT NOT NULL DEFAULT '{}',UNIQUE(job_id,package_id));
 CREATE TABLE IF NOT EXISTS shipping_labels(
 account_id INTEGER NOT NULL REFERENCES accounts(id),package_id TEXT NOT NULL,result_json TEXT NOT NULL,updated_at TEXT NOT NULL,PRIMARY KEY(account_id,package_id));
 DROP TABLE IF EXISTS supplier_leases;
 CREATE INDEX IF NOT EXISTS packing_jobs_account ON packing_jobs(account_id,id);
 CREATE INDEX IF NOT EXISTS packing_items_state ON packing_job_items(job_id,state,id);`)
	return err
}
func (s PackingJobs) Candidates(account, source string, gifts bool) ([]packing.Candidate, error) {
	lines, _, err := s.CandidatesWithSupplier(account, source, gifts)
	return lines, err
}

// Resolve the supplier from the same account-scoped report as the selected lines.
func (s PackingJobs) CandidatesWithSupplier(account, source string, gifts bool) ([]packing.Candidate, int64, error) {
	report, err := Procurement(s.DB, account, gifts)
	if err != nil {
		return nil, 0, err
	}
	out := []packing.Candidate{}
	var supplierID int64
	for _, g := range objects(report["groups"]) {
		supplier, _ := g["supplier"].(object)
		fromSupplier := text(supplier["id"]) != "" && text(supplier["id"]) != "0" && source == "supplier:"+text(supplier["id"])
		if text(g["key"]) != source && !fromSupplier {
			continue
		}
		supplierID, _ = strconv.ParseInt(text(supplier["id"]), 10, 64)
		for _, v := range objects(g["variants"]) {
			for _, l := range objects(v["lines"]) {
				var quantity int64
				fmt.Sscan(text(l["quantity"]), &quantity)
				gift, _ := l["gift"].(bool)
				out = append(out, packing.Candidate{Selection: packing.Selection{PackageID: text(l["packageId"]), ItemID: text(l["itemId"])}, PackageNo: text(l["packageNo"]), OrderNo: text(l["orderNo"]), Spec: text(v["spec"]), SKU: text(l["skuId"]), Quantity: quantity, Gift: gift})
			}
		}
	}
	if len(out) > 0 {
		return out, supplierID, nil
	}
	return nil, 0, errors.New("该货源或厂家已不在采购需求中，请刷新采购汇总")
}
func (s PackingJobs) Create(ctx context.Context, account string, in packing.BatchInput, items []packing.JobItem) (int64, error) {
	return s.create(ctx, account, in, items, "queued", "pending", nil)
}

func (s PackingJobs) CreateWithLease(ctx context.Context, account string, in packing.BatchInput, items []packing.JobItem, lease SupplierLease) (int64, error) {
	return s.create(ctx, account, in, items, "queued", "pending", &lease)
}

func (s PackingJobs) create(ctx context.Context, account string, in packing.BatchInput, items []packing.JobItem, state, itemState string, lease *SupplierLease) (int64, error) {
	if lease != nil {
		accountID, err := strconv.ParseInt(account, 10, 64)
		if err != nil {
			return 0, err
		}
		purchases := purchasesFor(s.DB)
		purchases.RLock()
		defer purchases.RUnlock()
		if err := purchases.verify(supplierKey{accountID, lease.Supplier}, *lease); err != nil {
			return 0, err
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var activeAccount bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM accounts WHERE id=? AND deleted=0)", account).Scan(&activeAccount); err != nil {
		return 0, err
	}
	if !activeAccount {
		return 0, errors.New("账户不存在")
	}

	if state == "running" {
		var blocked int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM packing_job_items WHERE state='running'`).Scan(&blocked); err != nil {
			return 0, err
		}
		if blocked > 0 {
			return 0, errors.New("已有包裹正在提交，请稍后重试")
		}
	}
	// Catch repeated saves while an earlier task still owns the selected lines.
	rows, err := tx.QueryContext(ctx, `SELECT i.lines_json FROM packing_job_items i JOIN packing_jobs j ON j.id=i.job_id WHERE j.account_id=? AND j.state IN ('queued','running','paused') AND i.state NOT IN ('failed','not_sent','cancelled','skipped')`, account)
	if err != nil {
		return 0, err
	}
	existing := map[packing.Selection]bool{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return 0, err
		}
		var lines []packing.Candidate
		if err = json.Unmarshal([]byte(raw), &lines); err != nil {
			rows.Close()
			return 0, err
		}
		for _, line := range lines {
			existing[line.Selection] = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	for _, item := range items {
		for _, line := range item.Lines {
			if existing[line.Selection] {
				return 0, fmt.Errorf("包裹 %s 的所选商品已有未结束的打包任务，请查看任务进度", line.PackageNo)
			}
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	r, err := tx.ExecContext(ctx, `INSERT INTO packing_jobs(account_id,name,source_key,state,interval_seconds,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, account, in.Name, in.SourceKey, state, in.IntervalSeconds, now, now)
	if err != nil {
		return 0, err
	}
	id, err := r.LastInsertId()
	if err != nil {
		return 0, err
	}
	for _, item := range items {
		request, _ := json.Marshal(item.Request)
		lines, _ := json.Marshal(item.Lines)
		if _, err = tx.ExecContext(ctx, `INSERT INTO packing_job_items(job_id,package_id,request_json,lines_json,state) VALUES(?,?,?,?,?)`, id, item.Request.PackageID, string(request), string(lines), itemState); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}
func (s PackingJobs) List(ctx context.Context, account string) ([]packing.Job, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,name,source_key,state,interval_seconds,created_at,updated_at FROM packing_jobs WHERE account_id=? ORDER BY id DESC`, account)
	if err != nil {
		return nil, err
	}
	jobs := []packing.Job{}
	for rows.Next() {
		var j packing.Job
		if err = rows.Scan(&j.ID, &j.Name, &j.SourceKey, &j.State, &j.IntervalSeconds, &j.CreatedAt, &j.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		jobs = append(jobs, j)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range jobs {
		rows, err = s.DB.QueryContext(ctx, `SELECT state,count(*) FROM packing_job_items WHERE job_id=? GROUP BY state`, jobs[i].ID)
		if err != nil {
			return nil, err
		}
		jobs[i].Counts = map[string]int{}
		for rows.Next() {
			var state string
			var n int
			if err = rows.Scan(&state, &n); err != nil {
				rows.Close()
				return nil, err
			}
			jobs[i].Counts[state] = n
			jobs[i].Total += n
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		infos, err := s.packingInfos(ctx, jobs[i].ID)
		if err != nil {
			return nil, err
		}
		jobs[i].PackingInfos = infos
	}
	return jobs, nil
}
func (s PackingJobs) Detail(ctx context.Context, account string, id int64) (packing.Job, error) {
	var j packing.Job
	err := s.DB.QueryRowContext(ctx, `SELECT id,name,source_key,state,interval_seconds,created_at,updated_at FROM packing_jobs WHERE account_id=? AND id=?`, account, id).Scan(&j.ID, &j.Name, &j.SourceKey, &j.State, &j.IntervalSeconds, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return j, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT i.id,i.request_json,i.lines_json,i.state,i.result_json,COALESCE(p.payload,'') FROM packing_job_items i LEFT JOIN packages p ON p.account_id=? AND p.package_id=i.package_id WHERE i.job_id=? ORDER BY i.id`, account, id)
	if err != nil {
		return j, err
	}
	defer rows.Close()
	j.Items = []packing.JobItem{}
	j.Counts = map[string]int{}
	for rows.Next() {
		i := packing.JobItem{JobID: id}
		var request, lines, result, payload string
		if err = rows.Scan(&i.ID, &request, &lines, &i.State, &result, &payload); err != nil {
			return j, err
		}
		if err = json.Unmarshal([]byte(request), &i.Request); err != nil {
			return j, err
		}
		if err = json.Unmarshal([]byte(lines), &i.Lines); err != nil {
			return j, err
		}
		if result != "{}" && i.State != "completed" {
			if err = json.Unmarshal([]byte(result), &i.Result); err != nil {
				return j, err
			}
		}
		if payload != "" {
			current, err := packageCandidates(payload, i.Request.PackageID)
			if err != nil {
				return j, err
			}
			metadata := map[string]packing.Candidate{}
			for _, line := range current {
				metadata[line.ItemID] = line
			}
			for n, line := range i.Lines {
				if v, ok := metadata[line.ItemID]; ok {
					if line.Image == nil {
						i.Lines[n].Image = v.Image
					}
					if line.OrderNo == "" {
						i.Lines[n].OrderNo = v.OrderNo
					}
				}
			}
		}
		j.Items = append(j.Items, i)
		j.Counts[i.State]++
		j.Total++
	}
	return j, rows.Err()
}

// A cancellation stops pending work. If every package was subsequently
// confirmed completed or skipped, the job has no cancelled work left.
const resolvedPackingJobs = `UPDATE packing_jobs SET state='completed'
 WHERE state IN ('cancelled','completed_errors','paused')
 AND EXISTS (SELECT 1 FROM packing_job_items WHERE job_id=packing_jobs.id)
 AND NOT EXISTS (SELECT 1 FROM packing_job_items WHERE job_id=packing_jobs.id AND state NOT IN ('completed','skipped'))`

func (s PackingJobs) Recover(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, _ := json.Marshal(packing.Result{State: "unknown", Message: "服务重启时该包裹正在提交，结果未知。请在妙手核对后处理；其余包裹已保留。"})
	if _, err = tx.ExecContext(ctx, `UPDATE packing_job_items SET state='unknown',result_json=? WHERE state='running'`, string(result)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE packing_jobs SET state=CASE WHEN EXISTS(SELECT 1 FROM packing_job_items WHERE job_id=packing_jobs.id AND state='pending') THEN 'queued' ELSE 'completed_errors' END WHERE state='running'`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, resolvedPackingJobs); err != nil {
		return err
	}
	return tx.Commit()
}
func (s PackingJobs) Claim(ctx context.Context, now time.Time) (*packing.JobItem, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var i packing.JobItem
	var request, lines string
	err = tx.QueryRowContext(ctx, `SELECT i.id,i.job_id,j.account_id,i.request_json,i.lines_json FROM packing_job_items i JOIN packing_jobs j ON i.job_id=j.id WHERE j.state='queued' AND i.state='pending' AND j.next_run_at<=? AND ? >= (SELECT COALESCE(max(next_run_at),0) FROM packing_jobs) AND NOT EXISTS(SELECT 1 FROM packing_job_items WHERE state='running') ORDER BY j.id,(i.result_json != '{}'),i.id LIMIT 1`, now.UnixMilli(), now.UnixMilli()).Scan(&i.ID, &i.JobID, &i.Account, &request, &lines)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(request), &i.Request); err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(lines), &i.Lines); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE packing_job_items SET state='running' WHERE id=?`, i.ID); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE packing_jobs SET state='running',updated_at=? WHERE id=?`, now.UTC().Format(time.RFC3339Nano), i.JobID); err != nil {
		return nil, err
	}
	i.State = "running"
	return &i, tx.Commit()
}
func (s PackingJobs) Finish(ctx context.Context, item packing.JobItem, result packing.Result, now time.Time) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if result.State == "" {
		result.State = "unknown"
		result.Message = "无法确认提交结果，请在妙手核对"
	}
	raw := []byte("{}")
	if result.State != "completed" {
		raw, err = json.Marshal(result)
		if err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE packing_job_items SET state=?,result_json=? WHERE id=? AND state='running'`, result.State, string(raw), item.ID); err != nil {
		return err
	}
	var state string
	var interval, pending, problems int
	if err = tx.QueryRowContext(ctx, `SELECT state,interval_seconds FROM packing_jobs WHERE id=?`, item.JobID).Scan(&state, &interval); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM packing_job_items WHERE job_id=? AND state='pending'`, item.JobID).Scan(&pending); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM packing_job_items WHERE job_id=? AND state NOT IN ('completed','pending','skipped','cancelled')`, item.JobID).Scan(&problems); err != nil {
		return err
	}
	if state != "cancelled" {
		if pending == 0 {
			state = "completed"
			if problems > 0 {
				state = "completed_errors"
			}
		} else if state != "paused" {
			state = "queued"
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE packing_jobs SET state=?,updated_at=?,next_run_at=? WHERE id=?`, state, now.UTC().Format(time.RFC3339Nano), now.Add(time.Duration(interval)*time.Second).UnixMilli(), item.JobID); err != nil {
		return err
	}
	if state == "cancelled" {
		if _, err = tx.ExecContext(ctx, resolvedPackingJobs+" AND id=?", item.JobID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s PackingJobs) Action(ctx context.Context, account string, id int64, action string, itemID int64) error {
	return s.itemAction(ctx, account, id, action, itemID, nil)
}

func (s PackingJobs) RetryEdited(ctx context.Context, account string, id, itemID int64, request packing.Request) error {
	if err := request.Validate(); err != nil {
		return err
	}
	return s.itemAction(ctx, account, id, "retry", itemID, &request)
}

func (s PackingJobs) itemAction(ctx context.Context, account string, id int64, action string, itemID int64, edited *packing.Request) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var state string
	if err = tx.QueryRowContext(ctx, `SELECT state FROM packing_jobs WHERE id=? AND account_id=?`, id, account).Scan(&state); err != nil {
		return err
	}
	switch action {
	case "pause":
		if state != "queued" && state != "running" {
			return errors.New("该任务当前没有执行")
		}
		state = "paused"
	case "resume":
		if state != "paused" {
			return errors.New("只能继续已暂停的任务")
		}
		var unresolved int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM packing_job_items WHERE job_id=? AND state='running'`, id).Scan(&unresolved); err != nil {
			return err
		}
		if unresolved > 0 {
			return errors.New("请等待当前正在提交的包裹返回")
		}
		var pending int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM packing_job_items WHERE job_id=? AND state='pending'`, id).Scan(&pending); err != nil {
			return err
		}
		if pending == 0 {
			return errors.New("没有剩余包裹，请先处理失败项")
		}
		state = "queued"
	case "cancel":
		if state != "queued" && state != "running" && state != "paused" {
			return errors.New("任务已结束")
		}
		state = "cancelled"
		if _, err = tx.ExecContext(ctx, `UPDATE packing_job_items SET state='cancelled' WHERE job_id=? AND state='pending'`, id); err != nil {
			return err
		}
	case "retry", "confirm-completed", "skip":
		if state == "cancelled" && action == "retry" {
			return errors.New("已取消的任务不能重新排队")
		}
		var itemState, saved string
		if err = tx.QueryRowContext(ctx, `SELECT state,request_json FROM packing_job_items WHERE id=? AND job_id=?`, itemID, id).Scan(&itemState, &saved); err != nil {
			return err
		}
		if itemState != "failed" && itemState != "not_sent" && itemState != "unknown" && itemState != "partial" && !(action == "retry" && itemState == "skipped") {
			return errors.New("该包裹无需处理")
		}
		next := "pending"
		if action == "confirm-completed" {
			next = "completed"
		}
		if action == "skip" {
			next = "skipped"
		}
		if edited != nil {
			var original packing.Request
			if err = json.Unmarshal([]byte(saved), &original); err != nil {
				return err
			}
			if original.PackageID != edited.PackageID || len(original.Items) != len(edited.Items) {
				return errors.New("包裹或商品范围已变化")
			}
			ids := map[string]bool{}
			for _, i := range original.Items {
				ids[i.OrderItemID] = true
			}
			for _, i := range edited.Items {
				if !ids[i.OrderItemID] {
					return errors.New("包含原包裹之外的商品")
				}
			}
			raw, err := json.Marshal(edited)
			if err != nil {
				return err
			}
			saved = string(raw)
		}
		if _, err = tx.ExecContext(ctx, `UPDATE packing_job_items SET state=?,request_json=? WHERE id=? AND job_id=?`, next, saved, itemID, id); err != nil {
			return err
		}
		if action == "retry" && (state == "completed_errors" || state == "completed") {
			state = "queued"
		}

	default:
		return errors.New("任务操作不存在")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE packing_jobs SET state=?,updated_at=? WHERE id=?`, state, time.Now().UTC().Format(time.RFC3339Nano), id); err != nil {
		return err
	}
	if action == "skip" || action == "confirm-completed" {
		if _, err = tx.ExecContext(ctx, resolvedPackingJobs+" AND id=?", id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s PackingJobs) packingInfos(ctx context.Context, jobID int64) ([]packing.Item, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT request_json FROM packing_job_items WHERE job_id=? ORDER BY id`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	infos := []packing.Item{}
	seen := map[string]bool{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var r packing.Request
		if err = json.Unmarshal([]byte(raw), &r); err != nil {
			return nil, err
		}
		for _, info := range r.Items {
			info.OrderItemID = ""
			key, _ := json.Marshal(info)
			if !seen[string(key)] {
				seen[string(key)] = true
				infos = append(infos, info)
			}
		}
	}
	return infos, rows.Err()
}
