package store

import (
	"database/sql"
	"encoding/json"
	"order_processor/internal/domain/connections"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPICredentialsSelectionAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orders.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	account, _ := Create(db, "a")
	other, _ := Create(db, "b")
	first, err := SaveAPICredential(db, account, 0, connections.Input{Name: "First", APIID: "api-one", Key: "secret-one", Current: true})
	if err != nil {
		t.Fatal(err)
	}
	second, err := SaveAPICredential(db, account, 0, connections.Input{Name: "Second", APIID: "api-two", Key: "secret-two"})
	if err != nil {
		t.Fatal(err)
	}
	if err = SetCurrentAPICredential(db, account, second); err != nil {
		t.Fatal(err)
	}
	// An invalid or cross-account selection must leave the current choice intact.
	if err = SetCurrentAPICredential(db, other, second); err == nil {
		t.Fatal("cross-account selection accepted")
	}
	if err = SetCurrentAPICredential(db, account, 999); err == nil {
		t.Fatal("missing selection accepted")
	}
	if _, err = SaveAPICredential(db, account, second, connections.Input{Name: "Renamed", APIID: "api-two"}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	active, err := CurrentAPICredential(db, account)
	if err != nil || active.ID != second || active.Name != "Renamed" || active.Key != "secret-two" {
		t.Fatal("saved choice/key did not survive reopening")
	}
	list, err := APICredentials(db, account)
	if err != nil || len(list) != 2 {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(list)
	if strings.Contains(string(raw), "secret-") {
		t.Fatal("secret in listing")
	}
	if _, err = SaveAPICredential(db, other, first, connections.Input{Name: "wrong", APIID: "wrong", Key: "wrong"}); err == nil {
		t.Fatal("cross-account edit accepted")
	}
	if err = DeleteAPICredential(db, other, first); err == nil {
		t.Fatal("cross-account delete accepted")
	}
	if err = DeleteAPICredential(db, account, second); err != nil {
		t.Fatal(err)
	}
	if _, err = CurrentAPICredential(db, account); err != sql.ErrNoRows {
		t.Fatal("deleting current must leave no active credential", err)
	}
	if err = SetCurrentAPICredential(db, account, first); err != nil {
		t.Fatal(err)
	}
}
