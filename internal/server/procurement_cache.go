package server

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type procurementCacheKey struct {
	account string
	gifts   bool
}
type procurementCacheEntry struct {
	mu   sync.Mutex
	data map[string]any
}
type procurementCache struct {
	mu      sync.Mutex
	entries map[procurementCacheKey]*procurementCacheEntry
	load    func(string, bool) (map[string]any, error)
}

func (c *procurementCache) get(account string, gifts, refresh bool) (map[string]any, error) {
	key := procurementCacheKey{account, gifts}
	c.mu.Lock()
	if c.entries == nil {
		c.entries = map[procurementCacheKey]*procurementCacheEntry{}
	}
	entry := c.entries[key]
	if entry == nil {
		entry = &procurementCacheEntry{}
		c.entries[key] = entry
	}
	c.mu.Unlock()
	// Serialize both initial reads and explicit refreshes for this scope. A slow
	// older calculation can never overwrite a newer generation.
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.data != nil && !refresh {
		return entry.data, nil
	}
	data, err := c.load(account, gifts)
	if err != nil {
		return nil, err
	}
	data["createdAt"] = time.Now().UnixMilli()
	entry.data = data
	return data, nil
}

var offerPath = regexp.MustCompile(`^/offer/([0-9]+)\.html$`)
var offerNumber = regexp.MustCompile(`^[0-9]{1,40}$`)

func sourceOfferID(group map[string]any) string {
	raw, _ := group["url"].(string)
	u, err := url.Parse(raw)
	if err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() == "detail.1688.com" {
		if match := offerPath.FindStringSubmatch(u.Path); len(match) == 2 {
			return match[1]
		}
	}
	source, _ := group["source"].(map[string]any)
	id := fmt.Sprint(source["sourceItemId"])
	if source["source"] == "1688" && offerNumber.MatchString(id) {
		return id
	}
	return ""
}
func summaryOfferSnapshot(data map[string]any, account, offer string) (map[string]any, bool) {
	variants := map[string]map[string]any{}
	title := ""
	groups, _ := data["groups"].([]map[string]any)
	for _, g := range groups {
		if reason, _ := g["reason"].(string); reason != "" || sourceOfferID(g) != offer {
			continue
		}
		if title == "" {
			title, _ = g["title"].(string)
		}
		rows, _ := g["variants"].([]map[string]any)
		for _, v := range rows {
			n, err := strconv.ParseInt(fmt.Sprint(v["quantity"]), 10, 64)
			if err != nil || n <= 0 || n > 9007199254740991 {
				continue
			}
			spec, _ := v["spec"].(string)
			current := variants[spec]
			if current == nil {
				current = map[string]any{"spec": spec, "quantity": int64(0)}
				variants[spec] = current
			}
			if current["quantity"].(int64) > 9007199254740991-n {
				continue
			}
			current["quantity"] = current["quantity"].(int64) + n
			images, _ := current["images"].([]string)
			incoming, _ := v["images"].([]string)
			for _, image := range incoming {
				u, err := url.Parse(image)
				if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
					continue
				}
				found := false
				for _, existing := range images {
					if existing == image {
						found = true
						break
					}
				}
				if !found && len(images) < 6 {
					images = append(images, image)
				}
			}
			if len(images) > 0 {
				current["images"] = images
			}
		}
	}
	if len(variants) == 0 {
		return nil, false
	}
	if strings.TrimSpace(title) == "" {
		title = "待采购商品"
	}
	specs := []string{}
	for spec := range variants {
		specs = append(specs, spec)
	}
	sort.Strings(specs)
	out := []map[string]any{}
	for _, spec := range specs {
		out = append(out, variants[spec])
	}
	// Deliberately construct a narrow DTO: no order lines, buyer fields, supplier
	// assignments, or original product payload crosses into the extension.
	return map[string]any{"version": 1, "offerId": offer, "url": "https://detail.1688.com/offer/" + offer + ".html", "account": account, "title": title, "createdAt": data["createdAt"], "variants": out}, true
}

func (c *procurementCache) invalidate(account string) {
	c.mu.Lock()
	entries := []*procurementCacheEntry{}
	for key, entry := range c.entries {
		if key.account == account {
			entries = append(entries, entry)
		}
	}
	c.mu.Unlock()
	for _, entry := range entries {
		entry.mu.Lock()
		entry.data = nil
		entry.mu.Unlock()
	}
}
