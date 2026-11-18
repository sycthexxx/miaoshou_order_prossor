package store

// Optional integration test against private captured JSON and a disposable DB
// backup. Private fixtures are never checked into source control.
import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"order_processor/internal/adapters/miaoshou/openapi"
)

func TestOpenAPICapturedPopulationParity(t *testing.T) {
	fixture, dbPath := os.Getenv("OPENAPI_PARITY_FIXTURES"), os.Getenv("OPENAPI_PARITY_DB")
	if fixture == "" || dbPath == "" {
		t.Skip("requires private captured fixtures and a disposable DB backup")
	}
	// Prevent accidental use against the normal live database.
	if filepath.Base(dbPath) != "openapi-parity.db" {
		t.Fatal("expected disposable openapi-parity.db")
	}
	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	accounts, err := Accounts(db)
	if err != nil || len(accounts) != 1 {
		t.Fatal("expected one baseline account", err)
	}
	account := accounts[0].ID
	webFiles, err := filepath.Glob(filepath.Join(fixture, "population-searchOrderPackageList-*.json"))
	if err != nil || len(webFiles) != 3 {
		t.Fatal("missing web fixtures", err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	scopes := map[string][]string{"waitPurchase": {}, "hasPurchase": {}}
	for _, file := range webFiles {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var env struct {
			ResponseBody string `json:"responseBody"`
		}
		if err = json.Unmarshal(b, &env); err != nil {
			t.Fatal(err)
		}
		var data object
		if err = decode([]byte(env.ResponseBody), &data); err != nil {
			t.Fatal(err)
		}
		for _, p := range objects(data["packageList"]) {
			id := text(p["opOrderPackageId"])
			raw, _ := json.Marshal(p)
			_, err = tx.Exec(`INSERT INTO packages(account_id,package_id,payload,captured_at,source_file) VALUES(?,?,?,?,?) ON CONFLICT(account_id,package_id) DO UPDATE SET payload=excluded.payload,captured_at=excluded.captured_at,source_file=excluded.source_file`, account, id, string(raw), "2080-01-01T00:00:00.000Z", "web-fixture")
			if err != nil {
				t.Fatal(err)
			}
			if err = saveItems(tx, account, id, "2080-01-01T00:00:00.000Z", p); err != nil {
				t.Fatal(err)
			}
			scope := "waitPurchase"
			if text(p["appPurchaseStatus"]) == "2" {
				scope = "hasPurchase"
			}
			scopes[scope] = append(scopes[scope], id)
		}
	}
	for scope, ids := range scopes {
		if err = saveOrderScope(tx, account, scope, "2080-01-01T00:00:00.000Z", []string{"web-fixture"}, ids); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	before := map[bool]object{}
	for _, gifts := range []bool{false, true} {
		before[gifts], err = Procurement(db, account, gifts)
		if err != nil {
			t.Fatal(err)
		}
	}
	snapshot := miaoshouopenapi.PendingSnapshot{}
	files, _ := filepath.Glob(filepath.Join(fixture, "*-official-*.json"))
	for _, file := range files {
		name := filepath.Base(file)
		isPackages := strings.Contains(name, "-waitProcess-") && !strings.Contains(name, "head-check")
		isPurchases := strings.Contains(name, "-purchase-population-")
		if !isPackages && !isPurchases {
			continue
		}
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var env struct {
			Response struct {
				Data struct {
					Packages  []json.RawMessage `json:"orderPackageList"`
					Purchases []json.RawMessage `json:"list"`
				} `json:"data"`
			} `json:"response"`
		}
		if err = json.Unmarshal(b, &env); err != nil {
			t.Fatal(err)
		}
		snapshot.Packages = append(snapshot.Packages, env.Response.Data.Packages...)
		snapshot.Purchases = append(snapshot.Purchases, env.Response.Data.Purchases...)
	}
	result, err := importPendingFixture(context.Background(), db, account, "2081-01-01T00:00:00.000Z", "openapi-fixture", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if result.WaitPurchase != 305 || result.HasPurchase != 192 || result.Packages != 497 || result.MissingRefundLines != 0 {
		t.Fatal(result)
	}
	for scope, ids := range scopes {
		got, err := Orders(db, account, "", 1, 1000, "waitProcess", scope)
		if err != nil {
			t.Fatal(err)
		}
		actual := []string{}
		for _, p := range got["items"].([]object) {
			actual = append(actual, text(p["opOrderPackageId"]))
		}
		sort.Strings(actual)
		sort.Strings(ids)
		if !reflect.DeepEqual(actual, ids) {
			t.Fatalf("%s memberships differ", scope)
		}
	}
	for _, gifts := range []bool{false, true} {
		after, err := Procurement(db, account, gifts)
		if err != nil {
			t.Fatal(err)
		}
		left, right := procurementBusinessResult(before[gifts]), procurementBusinessResult(after)
		if !reflect.DeepEqual(left, right) {
			os.WriteFile(filepath.Join(filepath.Dir(dbPath), "before.json"), []byte(left), 0600)
			os.WriteFile(filepath.Join(filepath.Dir(dbPath), "after.json"), []byte(right), 0600)
			t.Fatalf("procurement differs (gifts=%v); diagnostic JSON saved beside disposable DB", gifts)
		}
		variants := 0
		for _, g := range after["groups"].([]object) {
			variants += len(g["variants"].([]object))
		}
		t.Logf("gifts=%v: %v packages, %v units, %v sources, %d variants; identical quantities/specifications/order lines/supplier assignments", gifts, after["packages"], after["quantity"], after["sources"], variants)
	}
	t.Logf("groups matched: %d pending + %d purchased, %d supplemented item rows", result.WaitPurchase, result.HasPurchase, result.SupplementedLines)
}
func procurementBusinessResult(report object) string {
	groups := []object{}
	for _, g := range report["groups"].([]object) {
		variants := []object{}
		for _, v := range g["variants"].([]object) {
			lines := []string{}
			for _, line := range v["lines"].([]object) {
				b, _ := json.Marshal(line)
				lines = append(lines, string(b))
			}
			sort.Strings(lines)
			variants = append(variants, object{"spec": v["spec"], "quantity": v["quantity"], "lines": lines})
		}
		groups = append(groups, object{"key": g["key"], "quantity": g["quantity"], "products": g["products"], "variants": variants, "supplier": g["supplier"]})
	}
	b, _ := json.Marshal(object{"quantity": report["quantity"], "unresolvedQuantity": report["unresolvedQuantity"], "packages": report["packages"], "groups": groups, "pendingRefundLines": report["pendingRefundLines"]})
	return string(b)
}
