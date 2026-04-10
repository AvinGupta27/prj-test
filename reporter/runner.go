package reporter

import "time"

// ──────────────────────────────────────────────────────────────────────────────
// SERIALISABLE ROW
// ──────────────────────────────────────────────────────────────────────────────

// RowRecord is the serialisable representation of one Row stored inside Report.
type RowRecord struct {
	Index     int      `json:"index"`
	Label     string   `json:"label"`
	Status    Status   `json:"status"`
	LatencyMs int64    `json:"latencyMs"`
	Columns   []Column `json:"columns"`
	Details   []Detail `json:"details,omitempty"`
}

// ──────────────────────────────────────────────────────────────────────────────
// SUMMARY
// ──────────────────────────────────────────────────────────────────────────────

// Summary is the aggregate view of a completed run.
type Summary struct {
	Total       int          `json:"total"`
	PassCount   int          `json:"pass"`
	FailCount   int          `json:"fail"`
	SkipCount   int          `json:"skip"`
	WarnCount   int          `json:"warning"`
	SuccessRate float64      `json:"successRatePct"`
	WallTimeMs  int64        `json:"wallTimeMs"`
	Latency     LatencyStats `json:"latency"`
}

// ──────────────────────────────────────────────────────────────────────────────
// REPORT  (the serialisable payload written to JSON / rendered to HTML)
// ──────────────────────────────────────────────────────────────────────────────

// Report is the complete output of one Runner[R] after Finish() is called.
type Report struct {
	Name        string            `json:"name"`
	Meta        RunMeta           `json:"meta"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Summary     Summary           `json:"summary"`
	Rows        []RowRecord       `json:"rows"`
}

// ──────────────────────────────────────────────────────────────────────────────
// RUNNER
// ──────────────────────────────────────────────────────────────────────────────

// Runner accumulates rows of type R and produces a Report.
// R must implement the Row interface.
//
// Typical usage:
//
//	run := reporter.NewRunner[MyRow]("My Test", reporter.NewMeta(...))
//	run.Add(MyRow{...})
//	run.Annotate("key", "value")
//	report := run.Finish()
//	reporter.WriteAll(report, outDir)
type Runner[R Row] struct {
	name        string
	meta        RunMeta
	annotations map[string]string
	rows        []R
	durations   []time.Duration
}

// NewRunner creates a new Runner for the given test name and run metadata.
func NewRunner[R Row](name string, meta RunMeta) *Runner[R] {
	return &Runner[R]{
		name:        name,
		meta:        meta,
		annotations: make(map[string]string),
	}
}

// Add appends a result row.
func (r *Runner[R]) Add(row R) {
	r.rows = append(r.rows, row)
	r.durations = append(r.durations, row.RowLatency())
}

// Annotate attaches a key-value note to the run (appears in the report header).
func (r *Runner[R]) Annotate(key, value string) {
	r.annotations[key] = value
}

// Finish seals the run, computes statistics, and returns the serialisable Report.
// Call this once after all rows have been added.
func (r *Runner[R]) Finish() *Report {
	r.meta.FinishedAt = time.Now()

	rep := &Report{
		Name:        r.name,
		Meta:        r.meta,
		Annotations: r.annotations,
	}

	// Build RowRecords and tally counts.
	var passC, failC, skipC, warnC int
	var latDurations []time.Duration

	for i, row := range r.rows {
		rec := RowRecord{
			Index:     i + 1,
			Label:     row.RowLabel(),
			Status:    row.RowStatus(),
			LatencyMs: row.RowLatency().Milliseconds(),
			Columns:   row.RowColumns(),
			Details:   row.RowDetails(),
		}
		rep.Rows = append(rep.Rows, rec)

		switch row.RowStatus() {
		case StatusPass:
			passC++
		case StatusFail:
			failC++
		case StatusSkip:
			skipC++
		case StatusWarning:
			warnC++
		}

		if row.RowLatency() > 0 {
			latDurations = append(latDurations, row.RowLatency())
		}
	}

	total := len(r.rows)
	successRate := 0.0
	if total > 0 {
		successRate = float64(passC) / float64(total) * 100
	}

	rep.Summary = Summary{
		Total:       total,
		PassCount:   passC,
		FailCount:   failC,
		SkipCount:   skipC,
		WarnCount:   warnC,
		SuccessRate: successRate,
		WallTimeMs:  r.meta.Duration().Milliseconds(),
		Latency:     ComputeLatencyStats(latDurations),
	}

	return rep
}

// ──────────────────────────────────────────────────────────────────────────────
// WRITE HELPERS  (implementations live in writers.go / suite.go)
// ──────────────────────────────────────────────────────────────────────────────

// WriteJSON writes a JSON report for the given Report and returns the file path.
// HTML is intentionally not written here — the combined run HTML is written
// by WriteRunHTML in suite_test.go TearDownSuite after all tests complete.
func WriteJSON(rep *Report, outDir string) (string, error) {
	return writeJSON(rep, outDir)
}
