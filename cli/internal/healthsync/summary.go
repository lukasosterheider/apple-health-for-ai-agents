package healthsync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type metricStats struct {
	Count  int     `json:"count"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Avg    float64 `json:"avg"`
	Latest float64 `json:"latest"`
}

// Python's summary renders all metric values, including count, as floats. Keep
// its decimal/exponent notation without changing the aggregation's numeric types.
func summaryNumber(value float64) json.Number {
	format := byte('e')
	if value == 0 || math.Abs(value) >= 1e-4 && math.Abs(value) < 1e16 {
		format = 'f'
	}
	encoded := strconv.FormatFloat(value, format, -1, 64)
	if format == 'f' && !strings.ContainsRune(encoded, '.') {
		encoded += ".0"
	}
	return json.Number(encoded)
}

func (m metricStats) averageForOutput() float64 {
	// Python's sum starts at positive zero when all inputs are zero.
	if m.Avg == 0 && m.Min == 0 && m.Max == 0 {
		return 0
	}
	return m.Avg
}

func (m metricStats) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Count  json.Number `json:"count"`
		Min    json.Number `json:"min"`
		Max    json.Number `json:"max"`
		Avg    json.Number `json:"avg"`
		Latest json.Number `json:"latest"`
	}{summaryNumber(float64(m.Count)), summaryNumber(m.Min), summaryNumber(m.Max), summaryNumber(m.averageForOutput()), summaryNumber(m.Latest)})
}

// Struct field order preserves the Python report's JSON layout.
type summaryReport struct {
	Period  string        `json:"period"`
	Start   string        `json:"start"`
	End     string        `json:"end"`
	Storage string        `json:"storage"`
	Summary summaryTotals `json:"summary"`
}

type summaryTotals struct {
	SampleCount int                     `json:"sample_count"`
	RecordCount int                     `json:"record_count"`
	Metrics     map[string]*metricStats `json:"metrics"`
}

func collectNumbers(value any, path string, stats map[string]*metricStats) {
	if n, ok := number(value); ok {
		if path == "" {
			return
		}
		m, exists := stats[path]
		if !exists {
			stats[path] = &metricStats{1, n, n, n, n}
			return
		}
		m.Count++
		m.Min = math.Min(m.Min, n)
		m.Max = math.Max(m.Max, n)
		m.Avg = m.Avg*(float64(m.Count-1)/float64(m.Count)) + n/float64(m.Count)
		m.Latest = n
		return
	}
	switch v := value.(type) {
	case map[string]any:
		for _, key := range sortedKeys(v) {
			if !metricPattern.MatchString(key) {
				continue
			}
			child := key
			if path != "" {
				child = path + "." + key
			}
			collectNumbers(v[key], child, stats)
		}
	case []any:
		for _, child := range v {
			collectNumbers(child, path+"[]", stats)
		}
	}
}
func (a *app) summary(s *state, o options) error {
	kind, path, err := s.storage(o)
	if err != nil {
		return err
	}
	days := map[string]int{"daily": 1, "weekly": 7, "monthly": 30}[o.period]
	end := a.now().UTC().Truncate(time.Second)
	start := end.Add(-time.Duration(days) * 24 * time.Hour)
	samples, err := loadSamples(kind, path, start.Format("2006-01-02"))
	if err != nil {
		return err
	}
	stats := map[string]*metricStats{}
	records := map[string]bool{}
	for _, sample := range samples {
		records[sample.userID] = true
		collectNumbers(sample.data, "", stats)
	}
	report := summaryReport{
		Period: o.period, Start: start.Format("2006-01-02T15:04:05+00:00"), End: end.Format("2006-01-02T15:04:05+00:00"), Storage: kind,
		Summary: summaryTotals{SampleCount: len(samples), RecordCount: len(records), Metrics: stats},
	}
	var rendered []byte
	if o.output == "json" {
		rendered, err = json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		rendered = append(rendered, '\n')
	} else {
		var out bytes.Buffer
		fmt.Fprintf(&out, "Apple Health Summary (%s)\nWindow: %s -> %s\nSamples: %d\nSource records: %d\n", o.period, report.Start, report.End, len(samples), len(records))
		if len(stats) == 0 {
			fmt.Fprintln(&out, "Numeric metrics: none")
		} else {
			fmt.Fprintln(&out, "Numeric metrics:")
		}
		names := map[string]any{}
		for key := range stats {
			names[key] = nil
		}
		for _, key := range sortedKeys(names) {
			m := stats[key]
			fmt.Fprintf(&out, "- %s: avg=%.2f, min=%.2f, max=%.2f, latest=%.2f, n=%d\n", key, m.averageForOutput(), m.Min, m.Max, m.Latest, m.Count)
		}
		rendered = out.Bytes()
	}
	if o.save != "" {
		destination, e := absolutePath(o.save)
		if e != nil {
			return e
		}
		if e = outputParent(destination, false); e != nil {
			return e
		}
		fmt.Fprintln(a.errOut, "Warning: this report contains sensitive Apple Health information. Keep the destination private and exclude it from shared backups.")
		if e = writeNew(destination, rendered); e != nil {
			return e
		}
		_, err = fmt.Fprintln(a.out, "Report written to: "+destination)
		return err
	}
	_, err = a.out.Write(rendered)
	return err
}
