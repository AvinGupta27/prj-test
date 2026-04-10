package reporter

import (
	"sort"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// LATENCY STATS
// ──────────────────────────────────────────────────────────────────────────────

// LatencyStats holds computed percentile and histogram data.
type LatencyStats struct {
	Count  int   `json:"count"`
	MinMs  int64 `json:"minMs"`
	MaxMs  int64 `json:"maxMs"`
	AvgMs  int64 `json:"avgMs"`
	P50Ms  int64 `json:"p50Ms"`
	P95Ms  int64 `json:"p95Ms"`
	P99Ms  int64 `json:"p99Ms"`

	Histogram []HistogramBucket `json:"histogram"`
}

// HistogramBucket is one latency range bucket.
type HistogramBucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
	// BarWidth is 0–100, used directly as CSS width percent in the HTML bar.
	BarWidth int `json:"barWidthPct"`
}

var bucketDefs = []struct {
	label string
	max   int64 // exclusive upper bound in ms; -1 = no upper bound
}{
	{"< 100 ms", 100},
	{"100 – 300 ms", 300},
	{"300 – 500 ms", 500},
	{"500 ms – 1 s", 1000},
	{"> 1 s", -1},
}

// ComputeLatencyStats builds LatencyStats from a slice of durations.
// Zero-duration values are included in the count but excluded from percentiles
// when all values are zero.
func ComputeLatencyStats(durations []time.Duration) LatencyStats {
	if len(durations) == 0 {
		return LatencyStats{}
	}

	ms := make([]int64, len(durations))
	var total int64
	for i, d := range durations {
		ms[i] = d.Milliseconds()
		total += ms[i]
	}

	sorted := make([]int64, len(ms))
	copy(sorted, ms)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	n := len(sorted)
	stats := LatencyStats{
		Count: n,
		MinMs: sorted[0],
		MaxMs: sorted[n-1],
		AvgMs: total / int64(n),
		P50Ms: percentile(sorted, 50),
		P95Ms: percentile(sorted, 95),
		P99Ms: percentile(sorted, 99),
	}

	// Build histogram buckets.
	counts := make([]int, len(bucketDefs))
	for _, v := range ms {
		for i, b := range bucketDefs {
			if b.max == -1 || v < b.max {
				counts[i]++
				break
			}
		}
	}

	maxCount := 0
	for _, c := range counts {
		if c > maxCount {
			maxCount = c
		}
	}

	for i, b := range bucketDefs {
		bar := 0
		if maxCount > 0 {
			bar = counts[i] * 100 / maxCount
		}
		stats.Histogram = append(stats.Histogram, HistogramBucket{
			Label:    b.label,
			Count:    counts[i],
			BarWidth: bar,
		})
	}

	return stats
}

// percentile returns the value at the given percentile (0–100) from a sorted slice.
func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (p * len(sorted)) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
