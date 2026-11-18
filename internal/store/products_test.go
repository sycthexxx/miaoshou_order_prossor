package store

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestProductReplayAndOrderSourceJoin(t *testing.T) {
	dir := t.TempDir()
	db, e := Open(filepath.Join(dir, "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	account, _ := Create(db, "a")
	other, _ := Create(db, "b")
	write := func(name, at, body string) {
		t.Helper()
		raw, _ := json.Marshal(object{"accountId": account, "endpoint": productEndpoint, "capturedAt": at, "status": 200, "bodyBase64": base64.StdEncoding.EncodeToString([]byte(body))})
		p := filepath.Join(dir, "raw", account)
		os.MkdirAll(p, 0700)
		if e := os.WriteFile(filepath.Join(p, name), raw, 0600); e != nil {
			t.Fatal(e)
		}
		if _, e := Replay(db, dir); e != nil {
			t.Fatal(e)
		}
	}
	body := `{"result":"success","itemList":[{"platform":"tiktok","shopId":"shop1","platformItemId":"1737815173072323602","sourceList":[{"source":"1688","sourceItemId":"1067544205340","sourceItemUrl":"http://detail.1688.com/offer/1067544205340.html"}]}]}`
	write("1.json", "2026-11-16T01:00:00Z", body)
	var id string
	db.QueryRow("SELECT id FROM shop_products").Scan(&id)
	for _, a := range []string{account, other} {
		for _, shop := range []string{"shop1", "shop2"} {
			p := object{"opOrderPackageId": shop, "platform": "tiktok", "shopId": shop, "items": []object{{"opOrderItemId": "i1", "platformItemId": "1737815173072323602"}}}
			raw, _ := json.Marshal(p)
			if _, e := db.Exec(`INSERT INTO packages(account_id,package_id,payload,captured_at,source_file) VALUES(?,?,?,'2026','test')`, a, shop, string(raw)); e != nil {
				t.Fatal(e)
			}
		}
	}
	d, e := Orders(db, account, "", 1, 20)
	if e != nil {
		t.Fatal(e)
	}
	matched := 0
	for _, p := range d["items"].([]object) {
		i := p["items"].([]object)[0]
		if p["shopId"] == "shop1" {
			if i["sourceState"] != "available" {
				t.Fatal(i)
			}
			matched++
		} else if i["sourceState"] != "not_synced" {
			t.Fatal(i)
		}
	}
	if matched != 1 {
		t.Fatal(matched)
	}
	d, e = Orders(db, other, "", 1, 20)
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range d["items"].([]object) {
		if p["items"].([]object)[0]["sourceState"] != "not_synced" {
			t.Fatal(p)
		}
	}
	write("2.json", "2026-11-16T02:00:00Z", `{"result":"success","itemList":[{"platform":"tiktok","shopId":"shop1","platformItemId":"1737815173072323602","sourceList":[]}]}`)
	write("3.json", "2026-11-16T00:00:00Z", body)
	var count int
	var current string
	db.QueryRow("SELECT count(*),min(id) FROM shop_products").Scan(&count, &current)
	if count != 1 || current != id {
		t.Fatal(count, current, id)
	}
	products, e := Products(db, account, "", 1)
	if e != nil || products["items"].([]object)[0]["sourceState"] != "empty" {
		t.Fatal(products, e)
	}
	if n, e := Replay(db, dir); n != 0 || e != nil {
		t.Fatal(n, e)
	}
}

func TestProductStatusFilterCountsAndPagination(t *testing.T) {
	db, e := Open(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	account, _ := Create(db, "filter")
	other, _ := Create(db, "other")
	tx, e := db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 24; i++ {
		status := "onsale"
		if i >= 21 {
			status = "forsale"
		}
		a := account
		if i == 23 {
			a = other
		}
		p := object{"platform": "tiktok", "shopId": "1", "platformItemId": fmt.Sprint(i), "status": status, "title": "test"}
		if e := saveProduct(tx, a, "2026", "test", p); e != nil {
			t.Fatal(e)
		}
	}
	if e := tx.Commit(); e != nil {
		t.Fatal(e)
	}
	d, e := Products(db, account, "", 2, "onsale")
	if e != nil {
		t.Fatal(e)
	}
	if d["total"] != 21 || len(d["items"].([]object)) != 1 {
		t.Fatal(d)
	}
	counts := d["counts"].(map[string]int)
	if counts["onsale"] != 21 || counts["forsale"] != 2 || counts["draft"] != 0 {
		t.Fatal(counts)
	}
	d, e = Products(db, account, "absent", 1, "forsale")
	if e != nil || d["total"] != 0 || d["counts"].(map[string]int)["forsale"] != 2 {
		t.Fatal(d, e)
	}
	d, e = Products(db, account, "", 1, "forsale")
	if e != nil || d["total"] != 2 {
		t.Fatal(d, e)
	}
	for _, p := range d["items"].([]object) {
		if p["status"] != "forsale" {
			t.Fatal(p)
		}
	}
}
