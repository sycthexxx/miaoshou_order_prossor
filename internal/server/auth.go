package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"order_processor/internal/store"
	"strconv"
	"strings"
	"sync"
	"time"
)

type loginUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	IsAdmin  bool   `json:"isAdmin"`
}
type userContextKey struct{}

func currentUser(r *http.Request) loginUser {
	u, _ := r.Context().Value(userContextKey{}).(loginUser)
	return u
}

const sessionCookie = "order_session"
const sessionLifetime = 24 * time.Hour

type loginSession struct {
	userID, version int64
	expires         time.Time
}
type authentication struct {
	db       *sql.DB
	mu       sync.Mutex
	sessions map[string]loginSession
}

func (a *authentication) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Username, Password string }
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&in) != nil {
		send(w, 400, map[string]string{"error": "请输入用户名和密码"})
		return
	}
	var user loginUser
	var version int64
	err := a.db.QueryRowContext(r.Context(), "SELECT id,username,is_admin,auth_version FROM app_users WHERE username=? AND password=?", strings.TrimSpace(in.Username), in.Password).Scan(&user.ID, &user.Username, &user.IsAdmin, &version)
	if errors.Is(err, sql.ErrNoRows) {
		send(w, 401, map[string]string{"error": "用户名或密码不正确"})
		return
	}
	if err != nil {
		send(w, 500, map[string]string{"error": "登录失败，请稍后重试"})
		return
	}
	now := time.Now()
	token := rand.Text()
	a.mu.Lock()
	for key, s := range a.sessions {
		if !now.Before(s.expires) {
			delete(a.sessions, key)
		}
	}
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		delete(a.sessions, cookie.Value)
	}
	a.sessions[token] = loginSession{user.ID, version, now.Add(sessionLifetime)}
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil, MaxAge: int(sessionLifetime.Seconds())})
	send(w, 200, user)
}
func (a *authentication) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		a.mu.Lock()
		delete(a.sessions, c.Value)
		a.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	send(w, 200, map[string]bool{"ok": true})
}
func (a *authentication) user(r *http.Request) (loginUser, bool) {
	var user loginUser
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return user, false
	}
	a.mu.Lock()
	s, ok := a.sessions[c.Value]
	a.mu.Unlock()
	if !ok || !time.Now().Before(s.expires) {
		return user, false
	}
	err = a.db.QueryRowContext(r.Context(), "SELECT id,username,is_admin FROM app_users WHERE id=? AND auth_version=?", s.userID, s.version).Scan(&user.ID, &user.Username, &user.IsAdmin)
	return user, err == nil
}
func (a *authentication) protect(mux *http.ServeMux) http.Handler {
	mux.HandleFunc("POST /api/login", a.login)
	mux.HandleFunc("POST /api/logout", a.logout)
	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, r *http.Request) { send(w, 200, currentUser(r)) })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/desktop/") {
			mux.ServeHTTP(w, r)
			return
		}
		_, pattern := mux.Handler(r)
		if pattern == "POST /api/login" {
			mux.ServeHTTP(w, r)
			return
		}
		user, ok := a.user(r)
		if !ok {
			send(w, 401, map[string]string{"error": "请先登录工作台"})
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), userContextKey{}, user))
		var account string
		if strings.Contains(pattern, "/api/accounts/{id}") {
			parts := strings.Split(r.URL.EscapedPath(), "/")
			if len(parts) < 4 {
				http.NotFound(w, r)
				return
			}
			var err error
			account, err = url.PathUnescape(parts[3])
			numericID, parseErr := strconv.ParseInt(account, 10, 64)
			if err != nil || parseErr != nil || numericID < 1 || account != strconv.FormatInt(numericID, 10) {
				http.NotFound(w, r)
				return
			}
			if user.IsAdmin && !store.Exists(a.db, account) {
				send(w, 404, map[string]string{"error": "账户不存在"})
				return
			}
		}
		if !user.IsAdmin && !a.allowed(r, pattern, account, user.ID) {
			send(w, 403, map[string]string{"error": "当前用户没有此操作权限"})
			return
		}
		mux.ServeHTTP(w, r)
	})
}
func (a *authentication) allowed(r *http.Request, pattern, account string, id int64) bool {
	switch pattern {
	case "POST /api/logout", "GET /api/session", "GET /api/accounts", "GET /api/plugin-query-settings":
		return true
	}
	_, route, _ := strings.Cut(pattern, " ")
	const prefix = "/api/accounts/{id}/"
	if !strings.HasPrefix(route, prefix) {
		return false
	}
	operation := strings.Split(strings.TrimPrefix(route, prefix), "/")[0]
	switch operation {
	case "api-credentials", "login", "restart-container", "close", "archives":
		return false
	}
	var allowed bool
	err := a.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM user_accounts u JOIN accounts a ON a.id=u.account_id WHERE u.user_id=? AND a.id=? AND a.deleted=0)`, id, account).Scan(&allowed)
	return err == nil && allowed
}

func accessibleAccounts(db *sql.DB, user loginUser) ([]store.Account, error) {
	if user.IsAdmin {
		return store.Accounts(db)
	}
	rows, err := db.Query(`SELECT a.id,a.name,a.created_at FROM accounts a JOIN user_accounts u ON a.id=u.account_id WHERE u.user_id=? AND a.deleted=0 ORDER BY a.id`, user.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	accounts := []store.Account{}
	for rows.Next() {
		var account store.Account
		if err := rows.Scan(&account.ID, &account.Name, &account.CreatedAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}
