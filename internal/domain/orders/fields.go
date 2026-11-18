package orders

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func Decode(raw []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	return d.Decode(v)
}
func Text(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
func Objects(v any) []Fields {
	out := []Fields{}
	switch values := v.(type) {
	case []Fields:
		return values
	case []any:
		for _, v := range values {
			if o, ok := v.(map[string]any); ok {
				out = append(out, o)
			}
		}
	case map[string]any:
		for _, v := range values {
			if o, ok := v.(map[string]any); ok {
				out = append(out, o)
			}
		}
	}
	return out
}
