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
	mu          sync.Mutex
	data        map[string]any
	revision    int64
	subscribers map[chan map[string]any]struct{}
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
	at := time.Now().UnixMilli()
	data["createdAt"] = at
	if at <= entry.revision {
		at = entry.revision + 1
	}
	entry.revision = at
	data["revision"] = at
	entry.data = data
	entry.publishLocked(data)
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
	var sourceStatus any = map[string]any{"state": "unknown", "reason": "", "checkedAt": "", "source": ""}
	groups, _ := data["groups"].([]map[string]any)
	for _, g := range groups {
		if reason, _ := g["reason"].(string); reason != "" || sourceOfferID(g) != offer {
			continue
		}
		if status, ok := g["sourceStatus"]; ok {
			sourceStatus = status
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
	return map[string]any{"version": 1, "offerId": offer, "url": "https://detail.1688.com/offer/" + offer + ".html", "account": account, "title": title, "createdAt": data["createdAt"], "variants": out, "sourceStatus": sourceStatus}, true
}

// Only immutable complete snapshots enter these queues. Slow consumers retain
// the latest version; no network IO runs while cache locks are held.
func (e *procurementCacheEntry) publishLocked(data map[string]any) {
	for ch := range e.subscribers {
		select {
		case ch <- data:
		default:
			select {
			case <-ch:
			default:
			}
			ch <- data
		}
	}
}
func (c *procurementCache) subscribe(account string, gifts bool) (<-chan map[string]any, func(), error) {
	if _, err := c.get(account, gifts, false); err != nil {
		return nil, nil, err
	}
	c.mu.Lock()
	e := c.entries[procurementCacheKey{account, gifts}]
	c.mu.Unlock()
	e.mu.Lock()
	// Invalidation may happen between the first get and registration. Register
	// first and regenerate below if necessary, so no refresh can be missed.
	if e.subscribers == nil {
		e.subscribers = map[chan map[string]any]struct{}{}
	}
	ch := make(chan map[string]any, 1)
	e.subscribers[ch] = struct{}{}
	data := e.data
	if data != nil {
		ch <- data
	}
	e.mu.Unlock()
	cancel := func() { e.mu.Lock(); delete(e.subscribers, ch); e.mu.Unlock() }
	if data == nil {
		if _, err := c.get(account, gifts, false); err != nil {
			cancel()
			return nil, nil, err
		}
	}
	return ch, cancel, nil
}
func (c *procurementCache) refreshAccount(account string) error {
	c.mu.Lock()
	keys := []procurementCacheKey{}
	for key := range c.entries {
		if key.account == account {
			keys = append(keys, key)
		}
	}
	c.mu.Unlock()
	if len(keys) == 0 {
		keys = append(keys, procurementCacheKey{account: account})
	}
	var first error
	for _, key := range keys {
		if _, err := c.get(key.account, key.gifts, true); err != nil && first == nil {
			first = err
		}
	}
	return first
}
