package store

import (
	"database/sql"
	"fmt"
	"order_processor/internal/domain/connections"
	"strings"
)

func migrateAPICredentials(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS api_credentials (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 account_id INTEGER NOT NULL REFERENCES accounts(id),
 name TEXT NOT NULL, api_id TEXT NOT NULL, api_key TEXT NOT NULL,
 sub_account_id TEXT NOT NULL DEFAULT '', is_current INTEGER NOT NULL DEFAULT 0,
 UNIQUE(account_id,name));
 CREATE UNIQUE INDEX IF NOT EXISTS api_credentials_current ON api_credentials(account_id) WHERE is_current=1;`)
	return err
}
func APICredentials(db *sql.DB, account string) ([]connections.Credential, error) {
	rows, err := db.Query(`SELECT id,name,api_id,sub_account_id,is_current FROM api_credentials WHERE account_id=? ORDER BY is_current DESC,id`, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []connections.Credential{}
	for rows.Next() {
		var c connections.Credential
		if err = rows.Scan(&c.ID, &c.Name, &c.APIID, &c.SubAccountID, &c.Current); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func CurrentAPICredential(db *sql.DB, account string) (connections.Credential, error) {
	var c connections.Credential
	err := db.QueryRow(`SELECT id,name,api_id,api_key,sub_account_id,is_current FROM api_credentials WHERE account_id=? AND is_current=1`, account).Scan(&c.ID, &c.Name, &c.APIID, &c.Key, &c.SubAccountID, &c.Current)
	return c, err
}
func APICredential(db *sql.DB, account string, id int64) (connections.Credential, error) {
	var c connections.Credential
	err := db.QueryRow(`SELECT id,name,api_id,api_key,sub_account_id,is_current FROM api_credentials WHERE account_id=? AND id=?`, account, id).Scan(&c.ID, &c.Name, &c.APIID, &c.Key, &c.SubAccountID, &c.Current)
	return c, err
}
func SaveAPICredential(db *sql.DB, account string, id int64, in connections.Input) (int64, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.APIID = strings.TrimSpace(in.APIID)
	in.Key = strings.TrimSpace(in.Key)
	in.SubAccountID = strings.TrimSpace(in.SubAccountID)
	if in.Name == "" || in.APIID == "" {
		return 0, fmt.Errorf("请填写名称和 API ID")
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if id != 0 {
		var oldKey string
		if err = tx.QueryRow("SELECT api_key FROM api_credentials WHERE account_id=? AND id=?", account, id).Scan(&oldKey); err != nil {
			return 0, fmt.Errorf("API 凭证不存在")
		}
		if in.Key == "" {
			in.Key = oldKey
		}
	}
	if in.Key == "" {
		return 0, fmt.Errorf("请填写 API Key")
	}
	if id == 0 {
		r, e := tx.Exec(`INSERT INTO api_credentials(account_id,name,api_id,api_key,sub_account_id) VALUES(?,?,?,?,?)`, account, in.Name, in.APIID, in.Key, in.SubAccountID)
		if e != nil {
			return 0, fmt.Errorf("保存失败，请检查名称是否重复")
		}
		id, err = r.LastInsertId()
	} else {
		_, err = tx.Exec(`UPDATE api_credentials SET name=?,api_id=?,api_key=?,sub_account_id=? WHERE account_id=? AND id=?`, in.Name, in.APIID, in.Key, in.SubAccountID, account, id)
	}
	if err != nil {
		return 0, fmt.Errorf("保存失败，请检查名称是否重复")
	}
	if in.Current {
		if err = setCurrentAPICredential(tx, account, id); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}
func setCurrentAPICredential(tx *sql.Tx, account string, id int64) error {
	var exists int
	if err := tx.QueryRow("SELECT 1 FROM api_credentials WHERE account_id=? AND id=?", account, id).Scan(&exists); err != nil {
		return fmt.Errorf("API 凭证不存在")
	}
	if _, err := tx.Exec("UPDATE api_credentials SET is_current=0 WHERE account_id=? AND is_current=1", account); err != nil {
		return err
	}
	_, err := tx.Exec("UPDATE api_credentials SET is_current=1 WHERE account_id=? AND id=?", account, id)
	return err
}
func SetCurrentAPICredential(db *sql.DB, account string, id int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = setCurrentAPICredential(tx, account, id); err != nil {
		return err
	}
	return tx.Commit()
}
func DeleteAPICredential(db *sql.DB, account string, id int64) error {
	r, err := db.Exec("DELETE FROM api_credentials WHERE account_id=? AND id=?", account, id)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("API 凭证不存在")
	}
	return nil
}
