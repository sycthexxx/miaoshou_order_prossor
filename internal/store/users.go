package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
)

type ManagedUser struct {
	ID       int64    `json:"id"`
	Username string   `json:"username"`
	IsAdmin  bool     `json:"isAdmin"`
	Accounts []string `json:"accounts"`
}

type UserInput struct {
	Username string   `json:"username"`
	Password string   `json:"password"`
	IsAdmin  bool     `json:"isAdmin"`
	Accounts []string `json:"accounts"`
}

func migrateUsers(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err = tx.QueryRow("SELECT count(*) FROM pragma_table_info('accounts') WHERE name='deleted'").Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		if _, err = tx.Exec("ALTER TABLE accounts ADD COLUMN deleted INTEGER NOT NULL DEFAULT 0"); err != nil {
			return err
		}
	}
	// SQLite's INTEGER PRIMARY KEY is a signed, 64-bit row ID.
	_, err = tx.Exec(`CREATE TABLE IF NOT EXISTS app_users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,username TEXT NOT NULL UNIQUE,
		password TEXT NOT NULL, auth_version INTEGER NOT NULL DEFAULT 1, is_admin INTEGER NOT NULL DEFAULT 0 CHECK(is_admin IN(0,1))
	);
	CREATE TABLE IF NOT EXISTS user_accounts (
		user_id INTEGER NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
		account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
		PRIMARY KEY(user_id,account_id)
	);
	INSERT INTO app_users(username,password,is_admin)
	SELECT 'admin','123456',1 WHERE NOT EXISTS(SELECT 1 FROM app_users);`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func Users(ctx context.Context, db *sql.DB) ([]ManagedUser, error) {
	rows, err := db.QueryContext(ctx, `SELECT u.id,u.username,u.is_admin,a.id FROM app_users u
	LEFT JOIN user_accounts ua ON ua.user_id=u.id LEFT JOIN accounts a ON a.id=ua.account_id AND a.deleted=0
	ORDER BY u.id,a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []ManagedUser{}
	for rows.Next() {
		var user ManagedUser
		var account sql.NullInt64
		if err = rows.Scan(&user.ID, &user.Username, &user.IsAdmin, &account); err != nil {
			return nil, err
		}
		if len(users) == 0 || users[len(users)-1].ID != user.ID {
			user.Accounts = []string{}
			users = append(users, user)
		}
		if account.Valid {
			i := len(users) - 1
			users[i].Accounts = append(users[i].Accounts, strconv.FormatInt(account.Int64, 10))
		}
	}
	return users, rows.Err()
}

func checkAdminRemoval(ctx context.Context, tx *sql.Tx, id int64) error {
	var admin bool
	if err := tx.QueryRowContext(ctx, "SELECT is_admin FROM app_users WHERE id=?", id).Scan(&admin); err != nil {
		return err
	}
	if !admin {
		return nil
	}
	var other bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM app_users WHERE is_admin=1 AND id<>?)", id).Scan(&other); err != nil {
		return err
	}
	if !other {
		return errors.New("请至少保留一名管理员")
	}
	return nil
}

func SaveUser(ctx context.Context, db *sql.DB, id int64, in UserInput) (int64, error) {
	in.Username = strings.TrimSpace(in.Username)
	if in.Username == "" || len([]rune(in.Username)) > 60 {
		return 0, errors.New("请输入 1 至 60 字的用户名")
	}
	if id == 0 && in.Password == "" {
		return 0, errors.New("请输入密码")
	}
	purchases := purchasesFor(db)
	purchases.Lock()
	defer purchases.Unlock()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var taken bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM app_users WHERE username=? AND id<>?)", in.Username, id).Scan(&taken); err != nil {
		return 0, err
	}
	if taken {
		return 0, errors.New("用户名已存在")
	}
	if id == 0 {
		result, err := tx.ExecContext(ctx, "INSERT INTO app_users(username,password,is_admin) VALUES(?,?,?)", in.Username, in.Password, in.IsAdmin)
		if err != nil {
			return 0, err
		}
		id, err = result.LastInsertId()
		if err != nil {
			return 0, err
		}
	} else {
		if !in.IsAdmin {
			if err = checkAdminRemoval(ctx, tx, id); err != nil {
				return 0, err
			}
		}
		result, err := tx.ExecContext(ctx, `UPDATE app_users SET auth_version=auth_version+CASE WHEN (?<>'' AND ?<>password) OR ?<>is_admin THEN 1 ELSE 0 END,username=?,is_admin=?,password=COALESCE(NULLIF(?,''),password) WHERE id=?`, in.Password, in.Password, in.IsAdmin, in.Username, in.IsAdmin, in.Password, id)
		if err != nil {
			return 0, err
		}
		if n, _ := result.RowsAffected(); n != 1 {
			return 0, sql.ErrNoRows
		}
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM user_accounts WHERE user_id=?", id); err != nil {
		return 0, err
	}
	granted := make(map[int64]bool)
	if !in.IsAdmin {
		for _, account := range in.Accounts {
			accountID, err := strconv.ParseInt(account, 10, 64)
			if err != nil {
				return 0, errors.New("所选妙手账户不存在")
			}
			granted[accountID] = true
			result, err := tx.ExecContext(ctx, `INSERT INTO user_accounts(user_id,account_id) SELECT ?,id FROM accounts WHERE id=? AND deleted=0`, id, account)
			if err != nil {
				return 0, err
			}
			if n, _ := result.RowsAffected(); n != 1 {
				return 0, errors.New("所选妙手账户不存在")
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	for key, lease := range purchases.leases {
		if lease.UserID != id {
			continue
		}
		if !in.IsAdmin && !granted[key.account] {
			delete(purchases.leases, key)
		} else {
			lease.Username = in.Username
			purchases.leases[key] = lease
		}
	}
	return id, nil
}

func DeleteUser(ctx context.Context, db *sql.DB, id int64) error {
	purchases := purchasesFor(db)
	purchases.Lock()
	defer purchases.Unlock()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = checkAdminRemoval(ctx, tx, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM app_users WHERE id=?", id); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	for key, lease := range purchases.leases {
		if lease.UserID == id {
			delete(purchases.leases, key)
		}
	}
	return nil
}

func RemoveAccount(ctx context.Context, db *sql.DB, id string) error {
	accountID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return err
	}
	purchases := purchasesFor(db)
	purchases.Lock()
	defer purchases.Unlock()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var active bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM packing_jobs WHERE account_id=? AND state IN ('queued','running','paused'))`, id).Scan(&active); err != nil {
		return err
	}
	if active {
		return errors.New("该账户有尚未结束的打包任务，请先完成或取消任务")
	}
	// Preserve all order references and free the original name for a replacement account.
	result, err := tx.ExecContext(ctx, "UPDATE accounts SET deleted=1,name=name || char(0) || id WHERE id=? AND deleted=0", id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return sql.ErrNoRows
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM user_accounts WHERE account_id=?", id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM plugin_query_settings WHERE account_id=?", id); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	for key := range purchases.leases {
		if key.account == accountID {
			delete(purchases.leases, key)
		}
	}
	return nil
}
