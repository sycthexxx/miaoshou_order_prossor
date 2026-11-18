package packing

import (
	"reflect"
	"testing"
)

func TestIncludePackageGiftsCopiesInfoWithoutAddingOtherProducts(t *testing.T) {
	candidates := []Candidate{{Selection: Selection{"p", "main"}, Quantity: 1}, {Selection: Selection{"p", "gift"}, Gift: true, Quantity: 1}, {Selection: Selection{"p", "other"}, Quantity: 2}, {Selection: Selection{"other-package", "foreign-gift"}, Gift: true, Quantity: 1}}
	for _, info := range []Item{{Purchase: &Purchase{Platform: "1688", OrderNumber: "order", Buyer: "buyer", Seller: "seller"}, AutoSyncAmount: true}, {Waybills: []Waybill{{Company: "company", Number: "number"}}, Amount: "2"}} {
		info.OrderItemID = "main"
		job := JobItem{Request: Request{PackageID: "p", Items: []Item{info}}, Lines: candidates[:1]}
		result := IncludePackageGifts(job, candidates)
		if len(result.Request.Items) != 2 || len(result.Lines) != 2 || !result.Lines[1].Gift {
			t.Fatal(result)
		}
		want := info
		want.OrderItemID = "gift"
		if !reflect.DeepEqual(result.Request.Items[1], want) {
			t.Fatal(result.Request.Items[1], want)
		}
		repeated := IncludePackageGifts(result, candidates)
		if len(repeated.Request.Items) != 2 || len(repeated.Lines) != 2 {
			t.Fatal("duplicate gift", repeated)
		}
	}
	giftOnly := JobItem{Request: Request{PackageID: "p", Items: []Item{{OrderItemID: "gift"}}}}
	if got := IncludePackageGifts(giftOnly, candidates); len(got.Request.Items) != 1 {
		t.Fatal(got)
	}
}
