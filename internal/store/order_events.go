package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"order_processor/internal/domain/orders"
	"sort"
	"time"
)

// OrderEventFile is atomically published by the account browser after a fresh
// exact order-number lookup. The notification itself is never used as order state.
type OrderEventFile struct {
	Version    int      `json:"version"`
	AccountKey string   `json:"accountId"`
	MessageID  string   `json:"messageId"`
	OrderNo    string   `json:"orderNumber"`
	Kind       string   `json:"kind"`
	CapturedAt string   `json:"capturedAt"`
	Packages   []object `json:"packages"`
}

type OrderEvent struct {
	ID        int64    `json:"id"`
	OrderNo   string   `json:"orderNumber"`
	Kind      string   `json:"kind"`
	Suppliers []string `json:"suppliers,omitempty"`
}

func migrateOrderEvents(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS order_events (
	id INTEGER PRIMARY KEY AUTOINCREMENT, account_id INTEGER NOT NULL REFERENCES accounts(id),
	message_id TEXT NOT NULL, order_number TEXT NOT NULL, kind TEXT NOT NULL, captured_at TEXT NOT NULL, suppliers TEXT NOT NULL DEFAULT '[]',
	UNIQUE(account_id,message_id,order_number,kind));
	CREATE INDEX IF NOT EXISTS order_events_account_cursor ON order_events(account_id,id);
	CREATE TABLE IF NOT EXISTS order_event_packages (
	account_id INTEGER NOT NULL, package_id TEXT NOT NULL, captured_at TEXT NOT NULL,
	PRIMARY KEY(account_id,package_id),
	FOREIGN KEY(account_id,package_id) REFERENCES packages(account_id,package_id) ON DELETE CASCADE);`)
	if err != nil {
		return err
	}
	var exists bool
	if err = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pragma_table_info('order_events') WHERE name='suppliers')`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		_, err = db.Exec(`ALTER TABLE order_events ADD COLUMN suppliers TEXT NOT NULL DEFAULT '[]'`)
	}
	return err
}

func eventOrderParts(p object) []object {
	parts := []object{p}
	for _, key := range []string{"items", "orderList", "mergePackageList", "mergedPackageList", "subPackageList"} {
		parts = append(parts, objects(p[key])...)
	}
	return parts
}

func eventOrderMatches(p object, number string) bool {
	for _, part := range eventOrderParts(p) {
		info, _ := part["orderInfo"].(map[string]any)
		if text(part["platformOrderSn"]) == number || text(info["platformOrderSn"]) == number {
			return true
		}
	}
	return false
}

// Merged packages can carry several ERP order IDs. Only index explicit ID and
// number pairs from the same order; a SKU/child without an order ID is still
// searchable in the persisted package payload.
func indexEventOrders(tx *sql.Tx, account, at string, p object) error {
	for _, part := range eventOrderParts(p) {
		info, _ := part["orderInfo"].(map[string]any)
		id, no := text(info["opOrderId"]), text(info["platformOrderSn"])
		if id == "" {
			id = text(part["opOrderId"])
		}
		if no == "" {
			no = text(part["platformOrderSn"])
		}
		if id == "" || no == "" {
			continue
		}
		complete := len(info) > 0
		if !complete {
			info = object{"opOrderId": id, "platformOrderSn": no}
		}
		b, err := json.Marshal(info)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO orders(account_id,external_order_id,order_number,payload,captured_at) VALUES(?,?,?,?,?)
		ON CONFLICT(account_id,external_order_id) DO UPDATE SET order_number=excluded.order_number,
		payload=CASE WHEN ? THEN excluded.payload ELSE orders.payload END,captured_at=excluded.captured_at
		WHERE excluded.captured_at>=orders.captured_at`, account, id, no, string(b), at, complete)
		if err != nil {
			return err
		}
	}
	return nil
}

// Resolve suppliers from the same product and source assignments as procurement.
func eventSuppliers(tx *sql.Tx, account string, ev OrderEventFile) ([]string, error) {
	products, err := productMap(tx, account)
	if err != nil {
		return nil, err
	}
	groups := []object{}
	for _, pkg := range ev.Packages {
		items := objects(pkg["items"])
		for _, item := range items {
			info, _ := item["orderInfo"].(map[string]any)
			number := text(item["platformOrderSn"])
			if number == "" {
				number = text(info["platformOrderSn"])
			}
			if number != "" && number != ev.OrderNo {
				continue
			}
			if text(item["isGift"]) == "1" || text(item["isTiktokGift"]) == "1" {
				continue
			}
			platform, shop := text(item["platform"]), text(item["shopId"])
			if platform == "" {
				platform = text(pkg["platform"])
			}
			if shop == "" {
				shop = text(pkg["shopId"])
			}
			p := products[productKey{platform, shop, text(item["platformItemId"])}]
			keys := map[string]bool{}
			for _, source := range objects(p["sourceList"]) {
				if text(source["sourceItemId"]) == "" {
					continue
				}
				key, _ := json.Marshal([]string{text(source["source"]), text(source["sourceSite"]), text(source["sourceItemId"])})
				keys["source:"+string(key)] = true
			}
			group := object{}
			if len(keys) == 1 {
				for key := range keys {
					group["key"] = key
				}
			}
			groups = append(groups, group)
		}
	}
	if err := attachSuppliers(tx, account, object{"groups": groups}); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, group := range groups {
		supplier, _ := group["supplier"].(object)
		name := text(supplier["name"])
		if name == "" {
			name = "未指定厂家"
		}
		seen[name] = true
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func ImportOrderEvent(ctx context.Context, db *sql.DB, account Account, raw []byte) error {
	var ev OrderEventFile
	if err := decode(raw, &ev); err != nil {
		return err
	}
	at, err := time.Parse(time.RFC3339Nano, ev.CapturedAt)
	if err != nil {
		return err
	}
	if ev.Version != 1 || ev.AccountKey != account.StorageKey || ev.MessageID == "" || ev.OrderNo == "" ||
		(ev.Kind != "new_order" && ev.Kind != "cancelled" && ev.Kind != "in_cancel") || len(ev.Packages) == 0 {
		return errors.New("订单提醒格式错误")
	}
	// Check every split package before modifying any of them.
	seen := map[string]bool{}
	for _, p := range ev.Packages {
		id := text(p["opOrderPackageId"])
		if id == "" || seen[id] || !eventOrderMatches(p, ev.OrderNo) || text(p["appPackageStatus"]) == "" || len(objects(p["items"])) == 0 {
			return errors.New("订单提醒查询结果不完整或单号不匹配")
		}
		seen[id] = true
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	captured := at.UTC().Format("2006-01-02T15:04:05.000Z")
	r, err := tx.ExecContext(ctx, `INSERT INTO order_events(account_id,message_id,order_number,kind,captured_at)
	SELECT id,?,?,?,? FROM accounts WHERE id=? AND storage_key=? AND deleted=0
	ON CONFLICT(account_id,message_id,order_number,kind) DO NOTHING`, ev.MessageID, ev.OrderNo, ev.Kind, captured, account.ID, account.StorageKey)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil || n == 0 {
		return err
	}
	for _, p := range ev.Packages {
		id := text(p["opOrderPackageId"])
		if err := saveOrderRecord(ctx, tx, account.ID, captured, "order-event:"+ev.MessageID, "web", orders.Record{ID: id, Fields: p}); err != nil {
			return err
		}
		if err := indexEventOrders(tx, account.ID, captured, p); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO order_event_packages(account_id,package_id,captured_at) VALUES(?,?,?)
		ON CONFLICT(account_id,package_id) DO UPDATE SET captured_at=excluded.captured_at WHERE excluded.captured_at>order_event_packages.captured_at`, account.ID, id, captured)
		if err != nil {
			return err
		}
	}
	names, err := eventSuppliers(tx, account.ID, ev)
	if err != nil {
		return err
	}
	suppliers, err := json.Marshal(names)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE order_events SET suppliers=? WHERE account_id=? AND message_id=? AND order_number=? AND kind=?`, string(suppliers), account.ID, ev.MessageID, ev.OrderNo, ev.Kind); err != nil {
		return err
	}
	return tx.Commit()
}

func OrderEvents(db *sql.DB, account string, after *int64) ([]OrderEvent, int64, error) {
	events := []OrderEvent{}
	var cursor int64
	if after == nil {
		err := db.QueryRow("SELECT COALESCE(MAX(id),0) FROM order_events WHERE account_id=?", account).Scan(&cursor)
		return events, cursor, err
	}
	cursor = *after
	rows, err := db.Query("SELECT id,order_number,kind,suppliers FROM order_events WHERE account_id=? AND id>? ORDER BY id LIMIT 100", account, cursor)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var ev OrderEvent
		var suppliers string
		if err = rows.Scan(&ev.ID, &ev.OrderNo, &ev.Kind, &suppliers); err != nil {
			return nil, 0, err
		}
		if err = json.Unmarshal([]byte(suppliers), &ev.Suppliers); err != nil {
			return nil, 0, err
		}
		events = append(events, ev)
		cursor = ev.ID
	}
	return events, cursor, rows.Err()
}

// A single lookup must not replace an entire historical scope. For each
// complete snapshot, overlay only packages individually checked since it was
// taken. A subsequent complete snapshot wins again for that scope.
func applyEventMembership(db rowsQuerier, account string, scopes orderScopes, times map[string]string) error {
	if len(scopes) == 0 {
		return nil
	}
	var earliest string
	for _, at := range times {
		if earliest == "" || at < earliest {
			earliest = at
		}
	}
	rows, err := db.Query(`SELECT e.package_id,e.captured_at,p.payload FROM order_event_packages e JOIN packages p ON p.account_id=e.account_id AND p.package_id=e.package_id WHERE e.account_id=? AND e.captured_at>?`, account, earliest)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, at, raw string
		if err = rows.Scan(&id, &at, &raw); err != nil {
			return err
		}
		var p object
		if err = decode([]byte(raw), &p); err != nil {
			return fmt.Errorf("读取订单分组: %w", err)
		}
		tabs, purchase := orders.Scopes(nil).Membership(p)
		current := map[string]bool{purchase: true}
		for _, tab := range tabs {
			current[tab] = true
		}
		for scope, members := range scopes {
			if at > times[scope] {
				if current[scope] {
					members[id] = true
				} else {
					delete(members, id)
				}
			}
		}
	}
	return rows.Err()
}
