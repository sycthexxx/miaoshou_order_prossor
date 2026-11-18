package store

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"strconv"
	"time"
)

type Account struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	CreatedAt  string `json:"createdAt"`
	StorageKey string `json:"-"`
}

func Open(path string) (*sql.DB, error) {
	db, e := sql.Open("sqlite3", path+"?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000")
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	e = migrateAccounts(db)
	if e != nil {
		db.Close()
		return nil, e
	}
	if _, e = db.Exec(`CREATE TABLE IF NOT EXISTS plugin_query_settings (
        id INTEGER PRIMARY KEY CHECK(id=1),
        account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
        include_gifts INTEGER NOT NULL DEFAULT 0
    )`); e != nil {
		db.Close()
		return nil, e
	}
	if e = migrateOrders(db); e != nil {
		db.Close()
		return nil, e
	}
	if e = migrateOrderSnapshots(db); e != nil {
		db.Close()
		return nil, e
	}
	if e = migrateItems(db); e != nil {
		db.Close()
		return nil, e
	}
	// Retire the removed manual classification feature.
	if _, e = db.Exec("DROP TABLE IF EXISTS sku_classifications"); e != nil {
		db.Close()
		return nil, e
	}
	if e = migrateProducts(db); e != nil {
		db.Close()
		return nil, e
	}
	if e = migrateSuppliers(db); e != nil {
		db.Close()
		return nil, e
	}
	if e = migrateProcurementMappings(db); e != nil {
		db.Close()
		return nil, e
	}
	if e = migrateAPICredentials(db); e != nil {
		db.Close()
		return nil, e
	}
	if e = migratePackingJobs(db); e != nil {
		db.Close()
		return nil, e
	}
	return db, nil
}
func Accounts(db *sql.DB) ([]Account, error) {
	r, e := db.Query("SELECT id,name,created_at,storage_key FROM accounts ORDER BY id")
	if e != nil {
		return nil, e
	}
	defer r.Close()
	a := []Account{}
	for r.Next() {
		var v Account
		if e = r.Scan(&v.ID, &v.Name, &v.CreatedAt, &v.StorageKey); e != nil {
			return nil, e
		}
		a = append(a, v)
	}
	return a, r.Err()
}
func Create(db *sql.DB, name string) (string, error) {
	tx, e := db.Begin()
	if e != nil {
		return "", e
	}
	defer tx.Rollback()
	r, e := tx.Exec("INSERT INTO accounts(name,created_at,storage_key) VALUES(?,?,NULL)", name, time.Now().UTC().Format(time.RFC3339))
	if e != nil {
		return "", e
	}
	id, e := r.LastInsertId()
	if e != nil {
		return "", e
	}
	key := strconv.FormatInt(id, 10)
	if _, e = tx.Exec("UPDATE accounts SET storage_key=? WHERE id=?", key, id); e != nil {
		return "", e
	}
	return key, tx.Commit()
}
func StorageKey(db *sql.DB, id string) string {
	var key string
	db.QueryRow("SELECT storage_key FROM accounts WHERE id=?", id).Scan(&key)
	return key
}
func migrateAccounts(db *sql.DB) error {
	var schema string
	e := db.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name='accounts'").Scan(&schema)
	if e == sql.ErrNoRows {
		_, e = db.Exec(`CREATE TABLE accounts(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL UNIQUE,created_at TEXT NOT NULL,storage_key TEXT UNIQUE)`)
		return e
	}
	if e != nil {
		return e
	}
	var n int
	if db.QueryRow("SELECT count(*) FROM pragma_table_info('accounts') WHERE name='storage_key'").Scan(&n); n > 0 {
		return nil
	}
	tx, e := db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	_, e = tx.Exec(`ALTER TABLE accounts RENAME TO accounts_legacy;
 CREATE TABLE accounts(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL UNIQUE,created_at TEXT NOT NULL,storage_key TEXT UNIQUE);
 INSERT INTO accounts(name,created_at,storage_key) SELECT name,created_at,id FROM accounts_legacy ORDER BY created_at,id;
 DROP TABLE accounts_legacy;`)
	if e != nil {
		return e
	}
	for _, table := range []string{"packages", "translations", "imported_archives"} {
		var exists int
		tx.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&exists)
		if exists > 0 {
			if _, e = tx.Exec("UPDATE " + table + " SET account_id=(SELECT id FROM accounts WHERE storage_key=" + table + ".account_id)"); e != nil {
				return e
			}
		}
	}
	return tx.Commit()
}
func Exists(db *sql.DB, id string) bool {
	var n int
	return db.QueryRow("SELECT 1 FROM accounts WHERE id=?", id).Scan(&n) == nil
}
