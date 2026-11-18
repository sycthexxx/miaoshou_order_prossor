package orders

type Scopes map[string]map[string]bool

func (s Scopes) Membership(p Fields) ([]string, string) {
	id := Text(p["opOrderPackageId"])
	original := Tabs(p)
	tabs := map[string]bool{}
	for _, t := range original {
		tabs[t] = true
	}
	purchase := ""
	if tabs["waitProcess"] {
		if Text(p["appPurchaseStatus"]) == "0" {
			purchase = "waitPurchase"
		} else if v := Text(p["appPurchaseStatus"]); v == "1" || v == "2" {
			purchase = "hasPurchase"
		}
	}
	// Remove only memberships whose scope has a complete replacement.
	if purchase != "" {
		if _, known := s[purchase]; known {
			purchase = ""
		}
	}
	for _, scope := range []string{"waitPurchase", "hasPurchase"} {
		if ids, known := s[scope]; known && ids[id] {
			purchase = scope
		}
	}
	if len(s) > 0 {
		_, waitKnown := s["waitPurchase"]
		_, hasKnown := s["hasPurchase"]
		if waitKnown || hasKnown {
			tabs["waitProcess"] = purchase != ""
		}
	}
	for scope, ids := range s {
		if scope != "waitPurchase" && scope != "hasPurchase" {
			tabs[scope] = ids[id]
		}
	}
	out := []string{}
	for _, t := range []string{"all", "waitProcess", "waitShip", "submitPlatform", "waitReceiverConfirm", "isolation", "closed"} {
		if tabs[t] {
			out = append(out, t)
		}
	}
	return out, purchase
}
