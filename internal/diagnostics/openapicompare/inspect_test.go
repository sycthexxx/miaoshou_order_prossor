package compare

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInspectNestedAndEmbeddedStates(t *testing.T) {
	raw := json.RawMessage(`{"data":{"extra":{"newStatus":"ready"},"orderPackageList":[{"orderInfo":{"appOrderStatus":"pending","appOrderStatusText":"待处理"},"extInfo":"{\"purchase\":{\"status\":2}}","consigneeInfo":{"state":"PRIVATE_ADDRESS"},"buyerMessage":"PRIVATE_MESSAGE","id":9007199254740993}]}}`)
	fields, err := Inspect(raw)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, f := range fields {
		if f.Candidate {
			got[f.Path] = String(f.Value)
		}
	}
	for p, want := range map[string]string{
		`$["data"]["extra"]["newStatus"]`:                                         "ready",
		`$["data"]["orderPackageList"][0]["orderInfo"]["appOrderStatusText"]`:     "待处理",
		`$["data"]["orderPackageList"][0]["extInfo"]::json["purchase"]["status"]`: "2",
	} {
		if got[p] != want {
			t.Fatalf("%s: %q", p, got[p])
		}
	}
	encoded, _ := json.Marshal(fields)
	if strings.Contains(string(encoded), "PRIVATE_") || strings.Contains(string(encoded), "9007199254740993") {
		t.Fatal("non-state value was exposed")
	}
}
