package compare

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareNormalizationMissingAndPrivateFields(t *testing.T) {
	local, _ := Decode([]byte(`{"opOrderPackageId":9007199254740993,"appPackageNo":"MS-test","buyerName":"PRIVATE_BUYER","appPurchaseStatus":"0","items":[{"opOrderItemId":"9","platformSkuId":9007199254740995,"skuSubName":"红色， M","quantity":"2","skuSubCnName":"红色,M","picUrl":"https://img.invalid/a"}]}`))
	remote, _ := Decode([]byte(`{"opOrderPackageId":"9007199254740993","appPackageNo":"MS-test","buyerName":"OTHER_PRIVATE_BUYER","items":[{"opOrderItemId":9,"platformSkuId":"9007199254740995","skuSubName":"红色,M","quantity":3,"picUrl":"https://img.invalid/a"}]}`))
	r := Compare(local, remote, "list")
	if !r.Found || r.PackageID != "9007199254740993" {
		t.Fatal(r)
	}
	kinds := map[string]string{}
	for _, d := range r.Differences {
		kinds[d.Field] = d.Kind
	}
	if kinds["items[0].quantity"] != "different" || kinds["items[0].skuSubCnName"] != "missingRemote" || kinds["appPurchaseStatus"] != "missingRemote" || kinds["items[0].skuSubName"] != "" || kinds["items[0].platformSkuId"] != "" {
		t.Fatal(r)
	}
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "BUYER") {
		t.Fatal("private field leaked")
	}
	missing := Compare(local, nil, "detail")
	if missing.Found || missing.Compared != 0 || len(missing.Differences) != 0 {
		t.Fatal("unmatched package incorrectly compared", missing)
	}
}
func TestReadSampleChecksAccountAndLatestValidPending(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "account-test")
	os.Mkdir(dir, 0700)
	write := func(name, account, purchase, body string, valid bool) {
		if !valid {
			body = "invalid JSON"
		}
		b, _ := json.Marshal(map[string]any{"accountId": account, "capturedAt": name, "endpoint": "/api/order/package/render_list/searchOrderPackageList", "status": 200, "requestBody": "source=orderProcess&appPackageTab=waitProcess&purchaseTab=" + purchase, "bodyBase64": base64.StdEncoding.EncodeToString([]byte(body))})
		os.WriteFile(filepath.Join(dir, name+".json"), b, 0600)
	}
	body := `{"result":"success","packageList":[{"opOrderPackageId":9007199254740993,"orderInfo":{"platformOrderSn":"fake-order"}}]}`
	write("01", "account-test", "waitPurchase", body, true)
	write("02", "account-test", "hasPurchase", body, true)
	write("03", "wrong-account", "waitPurchase", body, true)
	write("04", "account-test", "waitPurchase", body, false)
	sample, err := ReadSample(dir, 3)
	if err != nil || sample.CapturedAt != "01" || len(sample.Packages) != 1 {
		t.Fatal(sample, err)
	}
	if _, err := ReadSample(dir, 6); err == nil {
		t.Fatal("unbounded sample accepted")
	}
}

func TestObjectItemsAndSeparateGiftArray(t *testing.T) {
	local, _ := Decode([]byte(`{"opOrderPackageId":"1","shopId":"s","items":{"20":{"opOrderItemId":"20","platformItemId":"p2","platformSkuId":"s2","quantity":"1","isTiktokGift":"1"},"10":{"opOrderItemId":"10","platformItemId":"p1","platformSkuId":"s1","quantity":"2","isTiktokGift":"0"}}}`))
	remote, _ := Decode([]byte(`{"opOrderPackageId":"1","shopId":"s","items":[{"opOrderItemId":10,"platformItemId":"p1","platformSkuId":"s1","quantity":2,"isGift":false}],"giftItems":[{"opOrderItemId":20,"platformItemId":"p2","platformSkuId":"s2","quantity":1}]}`))
	r := Compare(local, remote, "detail")
	if r.Items.Local.Items != 2 || r.Items.Remote.Items != 1 || r.Items.Remote.GiftItems != 1 || r.Items.Matched != 2 || len(r.Items.UnmatchedLocal) != 0 || len(r.Items.UnmatchedRemote) != 0 {
		t.Fatal(r)
	}
	if r.Compared <= 9 {
		t.Fatal("item fields were not compared", r.Compared)
	}
	found := false
	for _, f := range r.Fields {
		if f.Field == "items[0].quantity" && f.Status == "equal" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing field coverage")
	}
}
func TestUniqueBusinessFallbackAndAmbiguity(t *testing.T) {
	local, _ := Decode([]byte(`{"shopId":"s","items":[{"opOrderItemId":"old","platformItemId":"p","platformSkuId":"sku","quantity":2,"isGift":false}]}`))
	remote, _ := Decode([]byte(`{"shopId":"s","items":[{"opOrderItemId":"new","platformItemId":"p","platformSkuId":"sku","quantity":2,"isGift":false}]}`))
	r := Compare(local, remote, "list")
	if r.Items.Matched != 1 || r.Items.Matches[0].Method != "uniqueProductSKU" {
		t.Fatal(r)
	}
	remote["shopId"] = "other"
	if Compare(local, remote, "list").Items.Matched != 0 {
		t.Fatal("cross-shop fallback")
	}
	remote["shopId"] = "s"
	row := remote["items"].([]any)[0]
	remote["items"] = []any{row, row}
	r = Compare(local, remote, "list")
	if r.Items.Matched != 0 || len(r.Items.UnmatchedLocal) != 1 || len(r.Items.UnmatchedRemote) != 2 {
		t.Fatal("ambiguous rows matched", r)
	}
	for _, f := range r.Fields {
		if strings.HasPrefix(f.Field, "items[") && f.Status != "notComparedUnmatched" {
			t.Fatal(f)
		}
	}
	remote["items"] = []any{}
	remote["giftItems"] = []any{row}
	if Compare(local, remote, "list").Items.Matched != 0 {
		t.Fatal("gift and regular SKU mixed")
	}
}
