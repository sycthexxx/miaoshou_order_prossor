package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestUserAdministration(t *testing.T) {
	ctx := context.Background()
	filename := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := Users(ctx, db)
	if err != nil || len(admin) != 1 || admin[0].Username != "admin" || !admin[0].IsAdmin {
		t.Fatal(admin, err)
	}
	aid := admin[0].ID
	if _, err = SaveUser(ctx, db, aid, UserInput{Username: "admin"}); err == nil {
		t.Fatal("last admin demoted")
	}
	if err = DeleteUser(ctx, db, aid); err == nil {
		t.Fatal("last admin deleted")
	}
	a, err := Create(db, "采购")
	if err != nil {
		t.Fatal(err)
	}
	id, err := SaveUser(ctx, db, 0, UserInput{Username: "员工", Password: "secret", Accounts: []string{a}})
	if err != nil || id <= aid {
		t.Fatal(id, err)
	}
	if _, err = SaveUser(ctx, db, id, UserInput{Username: "员工", Accounts: []string{"999"}}); err == nil {
		t.Fatal("invalid account")
	}
	users, err := Users(ctx, db)
	if err != nil || len(users) != 2 || len(users[1].Accounts) != 1 || users[1].Accounts[0] != a {
		t.Fatal(users, err)
	}
	if _, err = SaveUser(ctx, db, id, UserInput{Username: "代班管理员", IsAdmin: true}); err != nil {
		t.Fatal(err)
	}
	var credential string
	var version int
	if err = db.QueryRow("SELECT password,auth_version FROM app_users WHERE id=?", id).Scan(&credential, &version); err != nil || credential != "secret" || version != 2 {
		t.Fatal(version, err)
	}
	if err = DeleteUser(ctx, db, aid); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	users, err = Users(ctx, db)
	if err != nil || len(users) != 1 || users[0].ID != id {
		t.Fatal(users, err)
	}
}

func TestRemovedAccountIsolatedFromActiveAccounts(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, err := Create(db, "采购")
	if err != nil {
		t.Fatal(err)
	}
	_, err = SaveUser(ctx, db, 0, UserInput{Username: "员工", Password: "123", Accounts: []string{a}})
	if err != nil {
		t.Fatal(err)
	}
	repo := PackingJobs{db}
	job := createPackingFixture(t, repo, a, 1)
	if err = RemoveAccount(ctx, db, a); err == nil {
		t.Fatal("removed account with pending work")
	}
	if err = repo.Action(ctx, a, job, "cancel", 0); err != nil {
		t.Fatal(err)
	}
	if err = RemoveAccount(ctx, db, a); err != nil {
		t.Fatal(err)
	}
	if Exists(db, a) || StorageKey(db, a) != "" {
		t.Fatal("removed account accessible")
	}
	active, err := Accounts(db)
	if err != nil || len(active) != 0 {
		t.Fatal(active, err)
	}
	users, err := Users(ctx, db)
	if err != nil || len(users[1].Accounts) != 0 {
		t.Fatal(users, err)
	}
	next, err := Create(db, "采购")
	if err != nil || next == a {
		t.Fatal(next, err)
	}
}
