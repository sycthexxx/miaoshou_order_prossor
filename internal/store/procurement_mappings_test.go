package store

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func sampleMapping(color string) ProcurementMapping {
	return ProcurementMapping{Version: 2, Colors: map[string]string{"海军蓝": color}, SKUs: map[string]ProcurementSKUMapping{"海军蓝,XL": {Tail: []string{"1XL"}}, "玫瑰,M": {Exception: "粉红色,M"}, "酒红色,L": {LegacyAlias: "深红色,L"}}}
}
func TestProcurementMappingSaveUpdateIsolationAndConflict(t *testing.T) {
	db, account, other := captureFixture(t)
	captureProduct(t, db, account, "100", "", "cn")
	got, e := GetProcurementMapping(db, account, "100")
	if e != nil || got.Revision != 0 || !reflect.DeepEqual(got.Mapping, emptyProcurementMapping()) {
		t.Fatal(got, e)
	}
	if _, e = GetProcurementMapping(db, other, "100"); !errors.Is(e, ErrProcurementMappingSource) {
		t.Fatal(e)
	}
	saved, e := SaveProcurementMapping(db, account, "100", 0, sampleMapping("藏青色"))
	if e != nil || saved.Revision != 1 {
		t.Fatal(saved, e)
	}
	var id int64
	var at string
	if e = db.QueryRow("SELECT id,updated_at FROM procurement_mappings").Scan(&id, &at); e != nil || id <= 0 {
		t.Fatal(id, e)
	}
	stale, e := SaveProcurementMapping(db, account, "100", 0, sampleMapping("错误覆盖"))
	if !errors.Is(e, ErrProcurementMappingConflict) || !reflect.DeepEqual(stale, saved) {
		t.Fatal(stale, e)
	}
	var unchangedAt string
	db.QueryRow("SELECT updated_at FROM procurement_mappings").Scan(&unchangedAt)
	if unchangedAt != at {
		t.Fatal("conflict modified timestamp")
	}
	updated, e := SaveProcurementMapping(db, account, "100", 1, sampleMapping("深蓝色"))
	if e != nil || updated.Revision != 2 {
		t.Fatal(updated, e)
	}
	var updatedID int64
	db.QueryRow("SELECT id FROM procurement_mappings").Scan(&updatedID)
	if updatedID != id || captureCount(t, db, "procurement_mappings") != 1 {
		t.Fatal("save replaced ID or duplicated source sites")
	}
	got, e = GetProcurementMapping(db, account, "100")
	if e != nil || !reflect.DeepEqual(got, updated) {
		t.Fatal(got, e)
	}
	captureProduct(t, db, other, "100", "")
	got, e = GetProcurementMapping(db, other, "100")
	if e != nil || got.Revision != 0 {
		t.Fatal(got, e)
	}
	if _, e = SaveProcurementMapping(db, other, "100", 0, sampleMapping("另一账户颜色")); e != nil {
		t.Fatal(e)
	}
	got, e = GetProcurementMapping(db, account, "100")
	if e != nil || got.Mapping.Colors["海军蓝"] != "深蓝色" {
		t.Fatal("cross-account mapping leaked", got, e)
	}
	if _, e = SaveProcurementMapping(db, account, "999", 0, sampleMapping("未知商品")); !errors.Is(e, ErrProcurementMappingSource) {
		t.Fatal(e)
	}
	if captureCount(t, db, "procurement_mappings") != 2 {
		t.Fatal("unknown product created mapping")
	}
	cleared, e := SaveProcurementMapping(db, account, "100", 2, ProcurementMapping{Version: 2})
	if e != nil || cleared.Revision != 3 || !reflect.DeepEqual(cleared.Mapping, emptyProcurementMapping()) {
		t.Fatal(cleared, e)
	}
}
func TestProcurementMappingConcurrentRevisionDoesNotOverwrite(t *testing.T) {
	db, account, _ := captureFixture(t)
	captureProduct(t, db, account, "100", "")
	if _, e := SaveProcurementMapping(db, account, "100", 0, sampleMapping("原始")); e != nil {
		t.Fatal(e)
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, name := range []string{"电脑A", "电脑B"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			_, e := SaveProcurementMapping(db, account, "100", 1, sampleMapping(name))
			results <- e
		}(name)
	}
	wg.Wait()
	close(results)
	saved, conflicts := 0, 0
	for e := range results {
		if e == nil {
			saved++
		} else if errors.Is(e, ErrProcurementMappingConflict) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if saved != 1 || conflicts != 1 {
		t.Fatal(saved, conflicts)
	}
	got, e := GetProcurementMapping(db, account, "100")
	if e != nil || got.Revision != 2 {
		t.Fatal(got, e)
	}
}
func TestProcurementMappingSurvivesReplayWithoutChangingOriginalPayloads(t *testing.T) {
	dir := t.TempDir()
	db, e := Open(filepath.Join(dir, "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	account, e := Create(db, "mapping replay")
	if e != nil {
		t.Fatal(e)
	}
	captureProduct(t, db, account, "100", "")
	order := `{"opOrderPackageId":"p","items":[{"skuSubCnName":"海军蓝,XL"}]}`
	if _, e = db.Exec(`INSERT INTO packages(account_id,package_id,payload,captured_at,source_file) VALUES(?,'p',?,'2026','fixture')`, account, order); e != nil {
		t.Fatal(e)
	}
	var originalProduct string
	db.QueryRow("SELECT payload FROM shop_products").Scan(&originalProduct)
	saved, e := SaveProcurementMapping(db, account, "100", 0, sampleMapping("藏青色"))
	if e != nil {
		t.Fatal(e)
	}
	var productAfter, orderAfter string
	db.QueryRow("SELECT payload FROM shop_products").Scan(&productAfter)
	db.QueryRow("SELECT payload FROM packages").Scan(&orderAfter)
	if originalProduct != productAfter || order != orderAfter {
		t.Fatal("mapping save mutated synced payloads")
	}
	for i := 1; i <= 2; i++ {
		body := fmt.Sprintf(`{"result":"success","itemList":[{"platform":"tiktok","shopId":"shop","platformItemId":"100","title":"同步版本%d","sourceList":[{"source":"1688","sourceItemId":"100"}]}]}`, i)
		raw, _ := json.Marshal(object{"accountId": account, "endpoint": productEndpoint, "capturedAt": fmt.Sprintf("2027-01-0%dT00:00:00Z", i), "status": 200, "bodyBase64": base64.StdEncoding.EncodeToString([]byte(body))})
		folder := filepath.Join(dir, "raw", account)
		if e = os.MkdirAll(folder, 0700); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(folder, fmt.Sprintf("%d.json", i)), raw, 0600); e != nil {
			t.Fatal(e)
		}
		if _, e = Replay(db, dir); e != nil {
			t.Fatal(e)
		}
		got, e := GetProcurementMapping(db, account, "100")
		if e != nil || !reflect.DeepEqual(got, saved) {
			t.Fatal("replay overwrote purchasing rules", got, e)
		}
	}
	db.QueryRow("SELECT payload FROM packages").Scan(&orderAfter)
	if orderAfter != order {
		t.Fatal("product replay changed order")
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	reopened, e := Open(filepath.Join(dir, "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	got, e := GetProcurementMapping(reopened, account, "100")
	if e != nil || !reflect.DeepEqual(got, saved) {
		t.Fatal("mapping not durable across restart", got, e)
	}
}
