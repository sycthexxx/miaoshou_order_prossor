package miaoshouopenapi

import (
	"encoding/json"
	"order_processor/internal/domain/orders"
	"testing"
)

func TestPendingBatchRefreshesLocalNote(t *testing.T) {
	for _, note := range []string{`"采购"`, `null`, `""`} {
		snapshot := PendingSnapshot{Packages: []json.RawMessage{json.RawMessage(`{"opOrderPackageId":1,"platform":"tiktok","items":[],"orderInfo":{"platformOrderSn":"587230213318608206","appNote":` + note + `}}`)}}
		batch, err := PendingBatch(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		got, _, _, err := orders.Merge(orders.Fields{"appNote": "旧备注"}, batch.Records[0].Fields, "", batch.Policy)
		if err != nil {
			t.Fatal(err)
		}
		var want any
		json.Unmarshal([]byte(note), &want)
		if got["appNote"] != want {
			t.Fatalf("note=%s got=%v", note, got["appNote"])
		}
	}
}
