package healthsync

import (
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strings"
)

var datePattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
var metricPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)
var stringPatterns = map[string]*regexp.Regexp{
	"type":      regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 &'()/+\-]{0,63}$`),
	"uuid":      regexp.MustCompile(`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`),
	"timestamp": regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+\-][0-9]{2}:[0-9]{2})$`),
	"source":    regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 .&'()/+\-]{0,63}$`),
	"bundle":    regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.\-]{2,127}$`),
	"timezone":  regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_./+\-]{1,63}$`),
	"unit":      regexp.MustCompile(`^[A-Za-z%°][A-Za-z0-9%°/().*+\-]{0,15}$`),
	"stage":     regexp.MustCompile(`^[a-z_]{1,32}$`),
	"device":    regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 .:&'()/+_,\-]{0,127}$`),
}
var eventTypes = []string{"pause", "resume", "lap", "marker", "motion_paused", "motion_resumed", "segment", "pause_or_resume_request"}
var speedSources = []string{"cross_country_skiing", "cycling", "paddle_sports", "rowing", "running", "stair_ascent", "stair_descent", "walking"}
var distanceSources = []string{"cross_country_skiing", "cycling", "downhill_snow_sports", "paddle_sports", "rowing", "skating_sports", "swimming", "walking_running", "wheelchair"}

func allowedString(path []string, value string) bool {
	if len(value) > 128 {
		return false
	}
	p := strings.Join(path, "/")
	kind := ""
	switch p {
	case "workouts/*/type":
		kind = "type"
	case "workouts/*/id":
		kind = "uuid"
	case "workouts/*/start", "workouts/*/end", "sleep/sessions/*/start", "sleep/sessions/*/end", "sleep/first_sleep_start", "sleep/last_sleep_end":
		kind = "timestamp"
	case "workouts/*/source":
		kind = "source"
	case "workouts/*/source_bundle_id":
		kind = "bundle"
	case "workouts/*/timezone", "sleep/sessions/*/timezone":
		kind = "timezone"
	case "sleep/sessions/*/stage":
		kind = "stage"
	}
	if len(path) == 3 && oneOf(path[0], "body", "heart") && path[2] == "unit" {
		kind = "unit"
	}
	if len(path) == 4 && strings.HasPrefix(p, "workouts/*/device/") && oneOf(path[3], "name", "manufacturer", "model", "hardware_version", "firmware_version", "software_version", "local_identifier", "udi_device_identifier") {
		kind = "device"
	}
	if p == "workouts/*/metadata" || strings.HasPrefix(p, "workouts/*/metadata/") {
		for _, candidate := range []string{"uuid", "timestamp", "timezone", "bundle", "source"} {
			if stringPatterns[candidate].MatchString(value) {
				return true
			}
		}
		return false
	}
	return kind != "" && stringPatterns[kind].MatchString(value)
}
func number(value any) (float64, bool) {
	var n float64
	switch v := value.(type) {
	case json.Number:
		parsed, err := v.Float64()
		if err != nil {
			return 0, false
		}
		n = parsed
	case float64:
		n = v
	case int:
		n = float64(v)
	case int64:
		n = float64(v)
	default:
		return 0, false
	}
	return n, !math.IsNaN(n) && !math.IsInf(n, 0)
}
func integer(value any) (int64, bool) {
	switch v := value.(type) {
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	case int:
		return int64(v), true
	case int64:
		return v, true
	default:
		return 0, false
	}
}
func requiredKeys(m map[string]any, required, optional []string) bool {
	for _, k := range required {
		if _, ok := m[k]; !ok {
			return false
		}
	}
	for k := range m {
		if !oneOf(k, required...) && !oneOf(k, optional...) {
			return false
		}
	}
	return true
}
func validRange(m map[string]any) bool {
	a, okA := integer(m["start_offset_ms"])
	b, okB := integer(m["end_offset_ms"])
	return okA && okB && b >= a
}
func positive(m map[string]any, key string, zero bool) bool {
	n, ok := number(m[key])
	return ok && (n > 0 || (zero && n == 0))
}

// Workout series are validated atomically and have their own higher limits.
// They must never fall through to the generic list truncation at 512 items.
func workoutField(field string, value any) (any, bool) {
	if field == "workout_activities" {
		return nil, false
	}
	if field == "workout_timing" {
		m, ok := value.(map[string]any)
		if !ok || !requiredKeys(m, []string{"elapsed_duration_ms", "active_duration_ms", "paused_duration_ms"}, nil) {
			return nil, false
		}
		e, okE := integer(m["elapsed_duration_ms"])
		a, okA := integer(m["active_duration_ms"])
		p, okP := integer(m["paused_duration_ms"])
		return m, okE && okA && okP && e >= 0 && a >= 0 && p >= 0 && a <= e && p == e-a
	}
	items, ok := value.([]any)
	limit := 65536
	if field == "workout_events" {
		limit = 4096
	}
	if !ok || len(items) > limit {
		return nil, false
	}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		switch field {
		case "heart_rate_samples":
			if !requiredKeys(m, []string{"start_offset_ms", "end_offset_ms", "bpm"}, nil) || !validRange(m) || !positive(m, "bpm", false) {
				return nil, false
			}
		case "workout_events":
			if !requiredKeys(m, []string{"type", "start_offset_ms", "end_offset_ms"}, nil) || !validRange(m) || !oneOf(rawString(m, "type"), eventTypes...) {
				return nil, false
			}
		case "speed_samples":
			if !requiredKeys(m, []string{"source", "start_offset_ms", "end_offset_ms", "speed_meters_per_second", "pace_seconds_per_kilometer"}, nil) || !validRange(m) || !oneOf(rawString(m, "source"), speedSources...) || !positive(m, "speed_meters_per_second", false) || !positive(m, "pace_seconds_per_kilometer", false) {
				return nil, false
			}
		case "distance_intervals":
			if !requiredKeys(m, []string{"source", "start_offset_ms", "end_offset_ms", "duration_ms", "distance_meters"}, []string{"speed_meters_per_second", "pace_seconds_per_kilometer"}) || !validRange(m) || !oneOf(rawString(m, "source"), distanceSources...) || !positive(m, "distance_meters", true) {
				return nil, false
			}
			d, ok := integer(m["duration_ms"])
			if !ok || d < 0 {
				return nil, false
			}
			_, speed := m["speed_meters_per_second"]
			_, pace := m["pace_seconds_per_kilometer"]
			if speed != pace || speed && (!positive(m, "speed_meters_per_second", false) || !positive(m, "pace_seconds_per_kilometer", false)) {
				return nil, false
			}
		case "route_points":
			optional := []string{"altitude_meters", "horizontal_accuracy_meters", "vertical_accuracy_meters", "speed_meters_per_second", "speed_accuracy_meters_per_second", "course_degrees", "course_accuracy_degrees"}
			if !requiredKeys(m, []string{"timestamp_offset_ms", "latitude", "longitude"}, optional) {
				return nil, false
			}
			_, okT := integer(m["timestamp_offset_ms"])
			lat, okLat := number(m["latitude"])
			lon, okLon := number(m["longitude"])
			if !okT || !okLat || !okLon || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
				return nil, false
			}
			for _, key := range optional {
				if value, exists := m[key]; exists {
					n, valid := number(value)
					if !valid || key != "altitude_meters" && n < 0 || key == "course_degrees" && n > 360 {
						return nil, false
					}
				}
			}
		default:
			return nil, false
		}
	}
	return items, true
}
func sanitizeValue(value any, depth int, nodes *int, path []string) (any, bool) {
	if depth > 12 {
		return nil, false
	}
	if len(path) == 3 && path[0] == "workouts" && path[1] == "*" && oneOf(path[2], "heart_rate_samples", "workout_timing", "workout_events", "workout_activities", "speed_samples", "distance_intervals", "route_points") {
		return workoutField(path[2], value)
	}
	*nodes++
	if *nodes > 20000 {
		return nil, false
	}
	switch v := value.(type) {
	case nil, bool:
		return v, true
	case json.Number, float64, int, int64:
		_, valid := number(v)
		return v, valid
	case string:
		return v, allowedString(path, v)
	case []any:
		out := make([]any, 0)
		for i, item := range v {
			if i >= 512 {
				break
			}
			if clean, ok := sanitizeValue(item, depth+1, nodes, appendPath(path, "*")); ok {
				out = append(out, clean)
			}
		}
		return out, true
	case map[string]any:
		out := map[string]any{}
		keys := sortedKeys(v)
		for _, key := range keys {
			if len(out) >= 256 {
				break
			}
			cleanKey := strings.TrimSpace(key)
			if !metricPattern.MatchString(cleanKey) {
				continue
			}
			if clean, ok := sanitizeValue(v[key], depth+1, nodes, appendPath(path, cleanKey)); ok {
				out[cleanKey] = clean
			}
		}
		return out, true
	default:
		return nil, false
	}
}
func appendPath(path []string, key string) []string {
	out := make([]string, len(path)+1)
	copy(out, path)
	out[len(path)] = key
	return out
}
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
func sanitizePayload(raw map[string]any) (map[string]any, map[string]int) {
	out := map[string]any{}
	stats := map[string]int{"raw_days": len(raw), "stored_days": 0, "dropped_days": 0}
	for _, day := range sortedKeys(raw) {
		if !datePattern.MatchString(day) {
			continue
		}
		nodes := 0
		clean, ok := sanitizeValue(raw[day], 0, &nodes, nil)
		if !ok {
			continue
		}
		if v, isMap := clean.(map[string]any); isMap && len(v) == 0 {
			continue
		}
		if v, isList := clean.([]any); isList && len(v) == 0 {
			continue
		}
		encoded, err := canonicalJSON(clean)
		if err != nil || len(encoded) > 4000000 {
			continue
		}
		out[day] = clean
	}
	stats["stored_days"] = len(out)
	stats["dropped_days"] = len(raw) - len(out)
	return out, stats
}
