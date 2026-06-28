package web

import (
	"encoding/json"
	"time"
)

const statsChartDays = 30

// StatsCharts holds time-series data for the /stats analytics graphs.
type StatsCharts struct {
	Labels      []string
	Calls       []int
	FileUploads []int
	Messages    []int
}

func (s *Server) gatherStatsCharts() StatsCharts {
	labels, keys := lastNDayKeys(statsChartDays)
	charts := StatsCharts{
		Labels:      labels,
		Calls:       make([]int, len(keys)),
		FileUploads: make([]int, len(keys)),
		Messages:    make([]int, len(keys)),
	}

	var callCounts map[string]int
	if s.Deps.Callers != nil {
		callCounts = s.Deps.Callers.CountByDay(statsChartDays)
	}
	var fileCounts map[string]int
	if s.Deps.Files != nil {
		if fc, err := s.Deps.Files.CountUploadsByDay(statsChartDays); err == nil {
			fileCounts = fc
		}
	}
	var msgCounts map[string]int
	if s.Deps.Messages != nil {
		if mc, err := s.Deps.Messages.CountPostsByDay(statsChartDays); err == nil {
			msgCounts = mc
		}
	}

	for i, key := range keys {
		if callCounts != nil {
			charts.Calls[i] = callCounts[key]
		}
		if fileCounts != nil {
			charts.FileUploads[i] = fileCounts[key]
		}
		if msgCounts != nil {
			charts.Messages[i] = msgCounts[key]
		}
	}
	return charts
}

func sumInts(v []int) int {
	n := 0
	for _, x := range v {
		n += x
	}
	return n
}

// HasCallsData reports whether the calls series has any recorded activity.
func (c StatsCharts) HasCallsData() bool { return sumInts(c.Calls) > 0 }

// HasMessagesData reports whether the messages series has any recorded activity.
func (c StatsCharts) HasMessagesData() bool { return sumInts(c.Messages) > 0 }

// HasFilesData reports whether the file upload series has any recorded activity.
func (c StatsCharts) HasFilesData() bool { return sumInts(c.FileUploads) > 0 }

// ChartJSON returns labels and datasets as JSON for embedding in stats.html.
func (c StatsCharts) ChartJSON() string {
	payload := map[string]any{
		"labels": c.Labels,
		"datasets": map[string][]int{
			"calls":       c.Calls,
			"fileUploads": c.FileUploads,
			"messages":    c.Messages,
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func lastNDayKeys(days int) (labels, keys []string) {
	labels = make([]string, days)
	keys = make([]string, days)
	for i := 0; i < days; i++ {
		d := time.Now().AddDate(0, 0, -(days - 1 - i))
		keys[i] = d.Format("2006-01-02")
		labels[i] = d.Format("Jan 2")
	}
	return labels, keys
}
