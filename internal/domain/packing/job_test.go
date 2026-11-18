package packing

import "testing"

func TestPrepareBatchFreezesSelectedLinesAndGroupsPackages(t *testing.T) {
	lines := []Candidate{{Selection: Selection{"p1", "a"}, Spec: "黑色,M", Quantity: 2}, {Selection: Selection{"p1", "gift"}, Gift: true, Quantity: 1}, {Selection: Selection{"p2", "b"}, Spec: "红色,L", Quantity: 1}, {Selection: Selection{"p1", "c"}, Spec: "黑色,L", Quantity: 1}}
	in := BatchInput{IntervalSeconds: 5, Selections: []Selection{{"p1", "a"}, {"p2", "b"}, {"p1", "c"}}, Info: Item{Waybills: []Waybill{{Company: "中通快递", Number: "00123"}}}}
	items, err := PrepareBatch(in, lines)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || len(items[0].Request.Items) != 2 || items[0].Request.Items[1].OrderItemID != "c" || items[0].Lines[0].Quantity != 2 {
		t.Fatal(items)
	}
	for _, i := range items {
		for _, line := range i.Lines {
			if line.Gift {
				t.Fatal("added gift without selection")
			}
		}
	}
	in.Selections = append(in.Selections, Selection{"p1", "a"})
	if _, err = PrepareBatch(in, lines); err == nil {
		t.Fatal("duplicate accepted")
	}
	in.Selections = []Selection{{"other", "a"}}
	if _, err = PrepareBatch(in, lines); err == nil {
		t.Fatal("stale selection accepted")
	}
}
