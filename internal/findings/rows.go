package findings

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

func rowString(row map[string]any, keys ...string) string {
	for _, k := range keys {
		v, ok := row[k]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			if strings.TrimSpace(t) != "" {
				return t
			}
		case float64:
			if t == math.Trunc(t) {
				return strconv.FormatInt(int64(t), 10)
			}
			return strconv.FormatFloat(t, 'f', -1, 64)
		case bool:
			if t {
				return "true"
			}
			return "false"
		default:
			s := strings.TrimSpace(fmt.Sprint(t))
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}

func rowFloat(row map[string]any, keys ...string) float64 {
	for _, k := range keys {
		v, ok := row[k]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case float64:
			return t
		case int:
			return float64(t)
		case string:
			f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
			if err == nil {
				return f
			}
		}
	}
	return 0
}

func rowBool(row map[string]any, key string) bool {
	v, ok := row[key]
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(strings.TrimSpace(t), "true")
	default:
		return false
	}
}

// rowTags reads a Resource Graph tags object.
// The key is absent when the query did not project tags, and the result is nil.
// A present null or empty object is an empty map (tags were loaded, and there are none).
func rowTags(row map[string]any) map[string]string {
	v, ok := row["tags"]
	if !ok {
		return nil
	}
	if v == nil {
		return map[string]string{}
	}
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]string, len(t))
		for k, val := range t {
			if val == nil {
				out[k] = ""
				continue
			}
			out[k] = strings.TrimSpace(fmt.Sprint(val))
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(t))
		for k, val := range t {
			out[k] = val
		}
		return out
	default:
		return nil
	}
}

func rowTime(row map[string]any, keys ...string) time.Time {
	raw := rowString(row, keys...)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z", "2006-01-02"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func stoppedAllocated(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "powerstate/stopped", "stopped", "vm stopped":
		return true
	default:
		return false
	}
}
