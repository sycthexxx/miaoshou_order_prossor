package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"order_processor/internal/domain/orders"
	"os"
	"path/filepath"
)

// ReplayArchives handles persistence and idempotency. The supplied adapter owns
// the external response format; the store never knows provider URLs or signatures.
func ReplayArchives(db *sql.DB, dir string, decoder orders.ArchiveDecoder) (int, error) {
	accounts, err := Accounts(db)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, account := range accounts {
		files, err := filepath.Glob(filepath.Join(dir, "raw", account.StorageKey, "*.json"))
		if err != nil {
			return count, err
		}
		for _, file := range files {
			name := filepath.Base(file)
			var n int
			err = db.QueryRow("SELECT 1 FROM imported_archives WHERE account_id=? AND file=? AND parser_version=3", account.ID, name).Scan(&n)
			if err == nil {
				continue
			}
			if err != sql.ErrNoRows {
				return count, err
			}
			raw, err := os.ReadFile(file)
			if err != nil {
				return count, err
			}
			archive, err := decoder.DecodeArchive(raw, account.StorageKey, name)
			if err != nil {
				return count, err
			}
			if archive == nil {
				continue
			}
			if err = importArchive(db, account.ID, name, *archive); err != nil {
				return count, err
			}
			count++
		}
	}
	return count, replayScopes(db, dir, decoder)
}
func importArchive(db *sql.DB, account, file string, archive orders.Archive) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	at := archive.CapturedAt
	for _, record := range archive.Records {
		p, id := record.Fields, record.ID
		switch archive.Kind {
		case orders.Packages:
			err = saveOrderRecord(context.Background(), tx, account, at, file, archive.Source, record)
		case orders.Products:
			err = saveProduct(tx, account, at, file, p)
		case orders.Outbound:
			err = saveOutbound(tx, account, at, file, p)
		case orders.Translations:
			for _, item := range objects(p["items"]) {
				iid := text(item["opOrderItemId"])
				if iid == "" {
					return fmt.Errorf("missing item id")
				}
				raw, e := json.Marshal(item)
				if e != nil {
					return e
				}
				_, err = tx.Exec(`INSERT INTO translations(account_id,package_id,item_id,payload,captured_at) VALUES(?,?,?,?,?) ON CONFLICT(account_id,package_id,item_id) DO UPDATE SET payload=excluded.payload,captured_at=excluded.captured_at WHERE excluded.captured_at>=translations.captured_at`, account, id, iid, string(raw), at)
				if err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unsupported record kind: %s", archive.Kind)
		}
		if err != nil {
			return err
		}
	}
	for _, scope := range archive.Scopes {
		if err = saveOrderScope(tx, account, scope.Name, scope.CapturedAt, scope.Files, scope.PackageIDs); err != nil {
			return err
		}
	}
	_, err = tx.Exec("INSERT INTO imported_archives(account_id,file,parser_version) VALUES(?,?,3) ON CONFLICT(account_id,file) DO UPDATE SET parser_version=3", account, file)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func replayScopes(db *sql.DB, dir string, decoder orders.ArchiveDecoder) error {
	accounts, err := Accounts(db)
	if err != nil {
		return err
	}
	for _, account := range accounts {
		files, err := filepath.Glob(filepath.Join(dir, "snapshots", account.StorageKey, "*.json"))
		if err != nil {
			return err
		}
		for _, file := range files {
			raw, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			// Scope manifests are our own archive metadata. Skip already applied
			// versions before asking the adapter to read all underlying pages.
			var header struct {
				Scope      string `json:"scope"`
				CapturedAt string `json:"capturedAt"`
			}
			if err = json.Unmarshal(raw, &header); err != nil {
				return err
			}
			var current string
			err = db.QueryRow("SELECT captured_at FROM order_scope_snapshots WHERE account_id=? AND scope=?", account.ID, header.Scope).Scan(&current)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
			if current >= header.CapturedAt {
				continue
			}
			scope, err := decoder.DecodeScope(raw, account.StorageKey, func(name string) ([]byte, error) {
				return os.ReadFile(filepath.Join(dir, "raw", account.StorageKey, name))
			})
			if err != nil {
				return err
			}
			if scope == nil {
				continue
			}
			complete := true
			for _, id := range scope.PackageIDs {
				var n int
				if err = db.QueryRow("SELECT 1 FROM packages WHERE account_id=? AND package_id=?", account.ID, id).Scan(&n); err != nil {
					complete = false
					break
				}
			}
			if !complete {
				continue
			}
			tx, err := db.Begin()
			if err != nil {
				return err
			}
			err = saveOrderScope(tx, account.ID, scope.Name, scope.CapturedAt, scope.Files, scope.PackageIDs)
			if err != nil {
				tx.Rollback()
				return err
			}
			if err = tx.Commit(); err != nil {
				return err
			}
		}
	}
	return nil
}
