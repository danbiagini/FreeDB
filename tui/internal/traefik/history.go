package traefik

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const maxHistoryDays = 30

// DailySnapshot holds one day's accumulated metrics per service
type DailySnapshot struct {
	Date     string                    `json:"date"` // YYYY-MM-DD
	Services map[string]*DailyMetrics  `json:"services"`
}

// DailyMetrics holds one service's stats for a single day
type DailyMetrics struct {
	Requests float64 `json:"requests"`
	Errors   float64 `json:"errors"`
	BytesIn  float64 `json:"bytes_in"`
	BytesOut float64 `json:"bytes_out"`
}

// MetricsHistory holds the full history and the baseline for computing today's delta
type MetricsHistory struct {
	Snapshots []DailySnapshot          `json:"snapshots"`
	Baseline  map[string]*DailyMetrics `json:"baseline"` // counter values at start of day
	FilePath  string                   `json:"-"`
}

func LoadHistory(path string) (*MetricsHistory, error) {
	h := &MetricsHistory{
		FilePath: path,
		Baseline: make(map[string]*DailyMetrics),
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return h, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, h); err != nil {
		return nil, err
	}
	if h.Baseline == nil {
		h.Baseline = make(map[string]*DailyMetrics)
	}

	return h, nil
}

func (h *MetricsHistory) Save() error {
	dir := filepath.Dir(h.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(h.FilePath, data, 0644)
}

// UpdateBaseline sets or resets the baseline counters for today.
// Call this once on startup (or when the date changes) with the current counter values.
func (h *MetricsHistory) UpdateBaseline(current map[string]*ServiceMetrics) {
	today := time.Now().Format("2006-01-02")

	// Check if we need to roll over to a new day
	if len(h.Snapshots) > 0 {
		lastDate := h.Snapshots[len(h.Snapshots)-1].Date
		if lastDate != today && len(h.Baseline) > 0 {
			// The baseline from the previous day is effectively the "end of day" snapshot
			// We don't need to do anything — the snapshot was already saved
		}
	}

	// Set baseline from current counters
	h.Baseline = make(map[string]*DailyMetrics)
	for name, sm := range current {
		h.Baseline[name] = &DailyMetrics{
			Requests: sm.TotalReqs,
			Errors:   sm.ErrorReqs,
			BytesIn:  sm.BytesIn,
			BytesOut: sm.BytesOut,
		}
	}
}

// RecordSnapshot saves today's accumulated metrics as a daily snapshot.
// Call this periodically or on shutdown.
func (h *MetricsHistory) RecordSnapshot(current map[string]*ServiceMetrics) {
	today := time.Now().Format("2006-01-02")

	services := make(map[string]*DailyMetrics)
	for name, sm := range current {
		baseline, ok := h.Baseline[name]
		if !ok {
			baseline = &DailyMetrics{}
		}
		services[name] = &DailyMetrics{
			Requests: sm.TotalReqs - baseline.Requests,
			Errors:   sm.ErrorReqs - baseline.Errors,
			BytesIn:  sm.BytesIn - baseline.BytesIn,
			BytesOut: sm.BytesOut - baseline.BytesOut,
		}
	}

	// Update or append today's snapshot
	found := false
	for i, snap := range h.Snapshots {
		if snap.Date == today {
			h.Snapshots[i].Services = services
			found = true
			break
		}
	}
	if !found {
		h.Snapshots = append(h.Snapshots, DailySnapshot{
			Date:     today,
			Services: services,
		})
	}

	// Prune old snapshots
	if len(h.Snapshots) > maxHistoryDays {
		h.Snapshots = h.Snapshots[len(h.Snapshots)-maxHistoryDays:]
	}
}

// TodayRequests returns the number of requests today for a service
func (h *MetricsHistory) TodayRequests(service string, current map[string]*ServiceMetrics) float64 {
	sm, ok := current[service]
	if !ok {
		return 0
	}
	baseline, ok := h.Baseline[service]
	if !ok {
		return sm.TotalReqs
	}
	delta := sm.TotalReqs - baseline.Requests
	if delta < 0 {
		return sm.TotalReqs // counter reset
	}
	return delta
}

// AvgDailyRequests returns the average daily requests over the last n days
func (h *MetricsHistory) AvgDailyRequests(service string, days int) float64 {
	if len(h.Snapshots) == 0 {
		return 0
	}

	total := 0.0
	count := 0
	start := len(h.Snapshots) - days
	if start < 0 {
		start = 0
	}

	for _, snap := range h.Snapshots[start:] {
		if dm, ok := snap.Services[service]; ok {
			total += dm.Requests
			count++
		}
	}

	if count == 0 {
		return 0
	}
	return total / float64(count)
}

// ServiceDailyHistory returns per-day metrics for a service over the last n days
func (h *MetricsHistory) ServiceDailyHistory(service string, days int) []DailySnapshot {
	var result []DailySnapshot
	start := len(h.Snapshots) - days
	if start < 0 {
		start = 0
	}

	for _, snap := range h.Snapshots[start:] {
		if dm, ok := snap.Services[service]; ok {
			result = append(result, DailySnapshot{
				Date:     snap.Date,
				Services: map[string]*DailyMetrics{service: dm},
			})
		}
	}
	return result
}
