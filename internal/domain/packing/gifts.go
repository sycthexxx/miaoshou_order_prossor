package packing

// IncludePackageGifts copies the first selected regular item's submission info
// to gifts in the same package. Other regular items are never selected implicitly.
func IncludePackageGifts(job JobItem, candidates []Candidate) JobItem {
	byID := map[string]Candidate{}
	for _, line := range candidates {
		if line.PackageID == job.Request.PackageID {
			byID[line.ItemID] = line
		}
	}
	var info Item
	hasMain := false
	for _, item := range job.Request.Items {
		if line, ok := byID[item.OrderItemID]; ok && !line.Gift {
			info = item
			hasMain = true
			break
		}
	}
	if !hasMain {
		return job
	}
	seen := map[string]bool{}
	for i, item := range job.Request.Items {
		seen[item.OrderItemID] = true
		if byID[item.OrderItemID].Gift {
			copy := info
			copy.OrderItemID = item.OrderItemID
			job.Request.Items[i] = copy
		}
	}
	for _, line := range candidates {
		if line.PackageID != job.Request.PackageID || !line.Gift || seen[line.ItemID] {
			continue
		}
		copy := info
		copy.OrderItemID = line.ItemID
		job.Request.Items = append(job.Request.Items, copy)
		job.Lines = append(job.Lines, line)
		seen[line.ItemID] = true
	}
	return job
}
