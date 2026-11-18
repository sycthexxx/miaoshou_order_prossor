package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"time"
	"weak"
)

var ErrSupplierLeaseLost = errors.New("厂家操作权已结束，请重新点击开始采购")
var ErrSupplierBusy = errors.New("厂家正在采购")

type SupplierLease struct {
	ID           int64  `json:"leaseId"`
	Supplier     int64  `json:"supplierId"`
	SupplierName string `json:"supplierName"`
	Username     string `json:"username"`
	UserID       int64  `json:"-"`
}

type supplierKey struct {
	account  int64
	supplier int64
}

type supplierPurchases struct {
	sync.RWMutex
	leases map[supplierKey]SupplierLease
	nextID int64
}

// Each opened store owns a fresh in-process map. Its entries never reach disk.
// When a protected operation also uses SQLite, it locks this map first and
// keeps the lock through commit, so handover cannot race an in-flight write.
var purchaseMaps sync.Map

func registerPurchaseMap(db *sql.DB) {
	key := weak.Make(db)
	purchaseMaps.Store(key, &supplierPurchases{leases: make(map[supplierKey]SupplierLease), nextID: time.Now().UnixMicro()})
	runtime.AddCleanup(db, func(k weak.Pointer[sql.DB]) { purchaseMaps.Delete(k) }, key)
}

func purchasesFor(db *sql.DB) *supplierPurchases {
	p, _ := purchaseMaps.Load(weak.Make(db))
	runtime.KeepAlive(db)
	return p.(*supplierPurchases)
}

func SupplierLeases(ctx context.Context, db *sql.DB, account string) ([]SupplierLease, error) {
	id, err := strconv.ParseInt(account, 10, 64)
	if err != nil {
		return nil, err
	}
	p := purchasesFor(db)
	p.RLock()
	defer p.RUnlock()
	leases := []SupplierLease{}
	for key, lease := range p.leases {
		if key.account == id {
			leases = append(leases, lease)
		}
	}
	return leases, nil
}

func AcquireSupplier(ctx context.Context, db *sql.DB, account string, lease SupplierLease) (SupplierLease, error) {
	id, err := strconv.ParseInt(account, 10, 64)
	if err != nil {
		return lease, err
	}
	p := purchasesFor(db)
	p.Lock()
	defer p.Unlock()
	err = db.QueryRowContext(ctx, `SELECT u.username FROM app_users u JOIN accounts a ON a.id=? AND a.deleted=0
	WHERE u.id=? AND (u.is_admin=1 OR EXISTS(SELECT 1 FROM user_accounts ua WHERE ua.user_id=u.id AND ua.account_id=a.id))`, id, lease.UserID).Scan(&lease.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return lease, ErrSupplierLeaseLost
	}
	if err != nil {
		return lease, err
	}
	key := supplierKey{id, lease.Supplier}
	current, exists := p.leases[key]
	if lease.ID != 0 && (!exists || current.ID != lease.ID || current.UserID != lease.UserID) {
		return lease, ErrSupplierLeaseLost
	}
	if exists {
		if current.UserID == lease.UserID {
			return current, nil
		}
		return lease, fmt.Errorf("%w：%s", ErrSupplierBusy, current.Username)
	}
	lease.SupplierName = "未指定厂家"
	if lease.Supplier != 0 {
		err = db.QueryRowContext(ctx, "SELECT name FROM suppliers WHERE id=? AND account_id=?", lease.Supplier, id).Scan(&lease.SupplierName)
		if errors.Is(err, sql.ErrNoRows) {
			return lease, errors.New("厂家不属于当前账户")
		}
		if err != nil {
			return lease, err
		}
	}
	p.nextID++
	lease.ID = p.nextID
	p.leases[key] = lease
	return lease, nil
}

func ReleaseSupplier(ctx context.Context, db *sql.DB, account string, lease SupplierLease) error {
	id, err := strconv.ParseInt(account, 10, 64)
	if err != nil {
		return err
	}
	p := purchasesFor(db)
	p.Lock()
	defer p.Unlock()
	key := supplierKey{id, lease.Supplier}
	if _, exists := p.leases[key]; !exists {
		return nil
	}
	if err := p.verify(key, lease); err != nil {
		return err
	}
	delete(p.leases, key)
	return nil
}

func (p *supplierPurchases) verify(key supplierKey, lease SupplierLease) error {
	current, ok := p.leases[key]
	if !ok || current.ID != lease.ID || current.UserID != lease.UserID {
		return ErrSupplierLeaseLost
	}
	return nil
}

func CheckSourceAvailable(db *sql.DB, account string, userID int64, source, product string, site *string, destination int64) error {
	p := purchasesFor(db)
	p.RLock()
	defer p.RUnlock()
	return p.checkSourceAvailable(db, account, userID, source, product, site, destination)
}

// Check both the currently assigned supplier(s) and a proposed destination.
// A 1688 offer can appear on multiple source sites. A nil site checks all of them.
func (p *supplierPurchases) checkSourceAvailable(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, account string, userID int64, source, product string, site *string, destination int64) error {
	id, err := strconv.ParseInt(account, 10, 64)
	if err != nil {
		return err
	}
	check := func(supplier int64) error {
		if lease, ok := p.leases[supplierKey{id, supplier}]; ok && lease.UserID != userID {
			return fmt.Errorf("%w：%s", ErrSupplierBusy, lease.Username)
		}
		return nil
	}
	if err := check(destination); err != nil {
		return err
	}
	if len(p.leases) == 0 {
		return nil
	}
	rows, err := q.Query(`SELECT DISTINCT COALESCE(m.supplier_id,0) FROM shop_products p
		JOIN json_each(p.payload,'$.sourceList') s
		LEFT JOIN source_suppliers m ON m.account_id=p.account_id AND m.source=?
			AND m.site=COALESCE(json_extract(s.value,'$.sourceSite'),'') AND m.product_id=?
		WHERE p.account_id=? AND json_extract(s.value,'$.source')=?
			AND CAST(json_extract(s.value,'$.sourceItemId') AS TEXT)=?
			AND (? IS NULL OR COALESCE(json_extract(s.value,'$.sourceSite'),'')=?)`, source, product, id, source, product, site, site)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var supplier int64
		if err := rows.Scan(&supplier); err != nil {
			return err
		}
		if err := check(supplier); err != nil {
			return err
		}
	}
	return rows.Err()
}
