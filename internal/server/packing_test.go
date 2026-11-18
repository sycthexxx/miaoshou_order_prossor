package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"order_processor/internal/adapters/miaoshou/web/browser"
	"order_processor/internal/store"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackingChecksOrderItemMembershipAndLeavesGroupsToSync(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id, _ := store.Create(db, "packing test")
	payload := `{"opOrderPackageId":"77","appPurchaseStatus":0,"items":[{"opOrderItemId":"88","opOrderPackageItemId":"99","quantity":1},{"opOrderItemId":"gift","isGift":true,"quantity":1}]}`
	_, err = db.Exec("INSERT INTO packages(account_id,package_id,payload,captured_at,source_file) VALUES(?,?,?,?,?)", id, "77", payload, "2026-11-17", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/waybill") {
			existingLabelFixture(w, r)
			return
		}
		var body struct {
			List []struct {
				Items map[string]json.RawMessage `json:"opOrderItemIdAndPurchaseOrderInfoMap"`
			} `json:"purchaseOrderInfoList"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.List) != 1 || len(body.List[0].Items) != 2 || string(body.List[0].Items["88"]) != string(body.List[0].Items["gift"]) {
			t.Errorf("single submission gift missing: %+v", body)
		}
		calls++
		var running int
		if err := db.QueryRow(`SELECT count(*) FROM packing_job_items WHERE state='running'`).Scan(&running); err != nil || running != 1 {
			t.Errorf("submission was not recorded before network call: %d %v", running, err)
		}
		json.NewEncoder(w).Encode(map[string]any{"status": 200, "body": `{"result":"success","successCount":1,"failCount":0,"errors":[]}`, "archive": "runtime/1/packing/test"})
	}))
	defer worker.Close()
	u, _ := url.Parse(worker.URL)
	g := &browserGateway{db: db, runtime: fixedRuntime{browser.Target{API: u, Token: "test"}}}
	call := func(iid string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"items":[{"orderItemId":"`+iid+`","waybills":[{"company":"中通快递","number":"001"}]}]}`))
		r.SetPathValue("id", id)
		r.SetPathValue("package", "77")
		w := httptest.NewRecorder()
		g.packing(w, r)
		return w
	}
	if w := call("99"); w.Code != 400 || calls != 0 {
		t.Fatal("package item ID was accepted", w.Code, calls)
	}
	g.gate(id).desktop = true
	if w := call("88"); w.Code != 409 || calls != 0 {
		t.Fatal("desktop wasn't excluded", w.Code, calls)
	}
	g.gate(id).desktop = false
	if w := call("88"); w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"completed"`) || calls != 1 {
		t.Fatal(w.Code, w.Body.String(), calls)
	}
	jobs, err := (store.PackingJobs{DB: db}).List(context.Background(), id)
	if err != nil || len(jobs) != 1 || jobs[0].State != "completed" || jobs[0].Counts["completed"] != 1 {
		t.Fatal("single submission missing from task list", jobs, err)
	}
	detail, err := (store.PackingJobs{DB: db}).Detail(context.Background(), id, jobs[0].ID)
	if err != nil || len(detail.Items) != 1 || len(detail.Items[0].Lines) != 2 || !detail.Items[0].Lines[1].Gift || detail.Items[0].Result.Archive != "runtime/1/packing/test" || detail.Items[0].Result.TaskID != jobs[0].ID {
		t.Fatal("submission details missing", detail, err)
	}
	var stored string
	db.QueryRow("SELECT payload FROM packages WHERE account_id=?", id).Scan(&stored)
	if stored != payload {
		t.Fatal("submission changed local order grouping")
	}
}

// Existing labels keep these packing tests focused on their original behavior.
func existingLabelFixture(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PackageID string `json:"packageId"`
	}
	json.NewDecoder(r.Body).Decode(&in)
	data, _ := json.Marshal(map[string]any{"result": "success", "orderPackageDetailInfo": map[string]string{"opOrderPackageId": in.PackageID, "logisticsNo": "DCS-fixture"}})
	json.NewEncoder(w).Encode(map[string]any{"status": 200, "body": string(data)})
}
