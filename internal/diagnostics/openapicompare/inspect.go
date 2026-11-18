package compare

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// FieldObservation inventories all fields. Values are exposed only for state
// candidates outside private subtrees. Paths retain array indices for pairing
// fields within the same package, without outputting buyer or order identifiers.
type FieldObservation struct {
	Path      string `json:"path"`
	Type      string `json:"type"`
	Candidate bool   `json:"stateCandidate,omitempty"`
	Value     any    `json:"value,omitempty"`
}

var stateField = regexp.MustCompile(`(?i)(status|statustext|state|statetext|tab|refundquantity|refundqty)$|(?i)^(isRefund|isCancelled|isCanceled|isShipped|isPurchased)$`)
var privateField = regexp.MustCompile(`(?i)consignee|buyer|recipient|address|phone|email|secret|token|password|credential|auth|note|message`)

// Inspect traverses the entire envelope, including JSON embedded in strings.
// A candidate's name is evidence for investigation, not a semantic mapping.
func Inspect(raw json.RawMessage) ([]FieldObservation, error) {
	p, err := Decode(raw)
	if err != nil {
		return nil, err
	}
	fields := []FieldObservation{}
	var walk func(any, string, string, bool, int)
	walk = func(v any, path, key string, private bool, embeddedDepth int) {
		f := FieldObservation{Path: path, Type: "null"}
		switch x := v.(type) {
		case map[string]any:
			f.Type = "object"
			fields = append(fields, f)
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(x[k], path+"["+strconv.Quote(k)+"]", k, private || privateField.MatchString(k), embeddedDepth)
			}
			return
		case []any:
			f.Type = "array"
			fields = append(fields, f)
			for i, item := range x {
				walk(item, path+"["+strconv.Itoa(i)+"]", key, private, embeddedDepth)
			}
			return
		case string:
			f.Type = "string"
		case json.Number:
			f.Type = "number"
		case bool:
			f.Type = "boolean"
		}
		f.Candidate = stateField.MatchString(key) && !private
		if f.Candidate {
			f.Value = v
		}
		fields = append(fields, f)
		if s, ok := v.(string); ok && embeddedDepth < 8 {
			s = strings.TrimSpace(s)
			if strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
				var decoded any
				decoder := json.NewDecoder(strings.NewReader(s))
				decoder.UseNumber()
				if json.Valid([]byte(s)) && decoder.Decode(&decoded) == nil {
					walk(decoded, path+"::json", key, private, embeddedDepth+1)
				}
			}
		}
	}
	walk(map[string]any(p), "$", "", false, 0)
	return fields, nil
}
