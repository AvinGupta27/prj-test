package reporter

import (
	"fmt"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// STATUS
// ──────────────────────────────────────────────────────────────────────────────

// Status represents the outcome of a single test row.
type Status string

const (
	StatusPass    Status = "pass"
	StatusFail    Status = "fail"
	StatusSkip    Status = "skip"
	StatusWarning Status = "warning" // succeeded but with degraded data
)

func (s Status) IsPass() bool { return s == StatusPass }
func (s Status) IsFail() bool { return s == StatusFail }

// ──────────────────────────────────────────────────────────────────────────────
// COLUMN
// ──────────────────────────────────────────────────────────────────────────────

// Column is a single cell in the HTML result table.
// Mono renders the value in monospace font (good for IDs, JSON).
type Column struct {
	Header string
	Value  string
	Mono   bool
}

// ──────────────────────────────────────────────────────────────────────────────
// DETAIL
// ──────────────────────────────────────────────────────────────────────────────

// Detail is an expandable key-value pair shown when a row is clicked.
type Detail struct {
	Key   string
	Value string
	Mono  bool // render value in monospace (good for JSON bodies)
}

// ──────────────────────────────────────────────────────────────────────────────
// FAILURE DETAIL
// ──────────────────────────────────────────────────────────────────────────────

// FailureDetail captures the full HTTP context of a failed call.
// All fields are optional — populate only what is available.
type FailureDetail struct {
	RequestMethod  string
	RequestURL     string
	RequestHeaders map[string]string
	RequestBody    string
	ResponseStatus int
	ResponseBody   string
	ErrorChain     string    // fmt.Sprintf("%+v", err)
	OccurredAt     time.Time
}

func (f FailureDetail) IsZero() bool {
	return f.RequestURL == "" && f.ErrorChain == ""
}

// ToDetails converts a FailureDetail into []Detail for HTML rendering.
func (f FailureDetail) ToDetails() []Detail {
	if f.IsZero() {
		return nil
	}
	d := []Detail{}
	if f.RequestMethod != "" || f.RequestURL != "" {
		d = append(d, Detail{Key: "Request", Value: f.RequestMethod + " " + f.RequestURL, Mono: true})
	}
	for k, v := range f.RequestHeaders {
		d = append(d, Detail{Key: "Header: " + k, Value: v, Mono: true})
	}
	if f.RequestBody != "" {
		d = append(d, Detail{Key: "Request Body", Value: f.RequestBody, Mono: true})
	}
	if f.ResponseStatus != 0 {
		d = append(d, Detail{Key: "Response Status", Value: statusText(f.ResponseStatus), Mono: false})
	}
	if f.ResponseBody != "" {
		d = append(d, Detail{Key: "Response Body", Value: f.ResponseBody, Mono: true})
	}
	if f.ErrorChain != "" {
		d = append(d, Detail{Key: "Error", Value: f.ErrorChain, Mono: true})
	}
	if !f.OccurredAt.IsZero() {
		d = append(d, Detail{Key: "Occurred At", Value: f.OccurredAt.Format("2006-01-02 15:04:05.000"), Mono: false})
	}
	return d
}

func statusText(code int) string {
	texts := map[int]string{
		200: "200 OK", 201: "201 Created", 400: "400 Bad Request",
		401: "401 Unauthorized", 403: "403 Forbidden", 404: "404 Not Found",
		409: "409 Conflict", 422: "422 Unprocessable Entity",
		429: "429 Too Many Requests", 500: "500 Internal Server Error",
		502: "502 Bad Gateway", 503: "503 Service Unavailable",
	}
	if t, ok := texts[code]; ok {
		return t
	}
	return fmt.Sprintf("%d", code)
}

// ──────────────────────────────────────────────────────────────────────────────
// ROW INTERFACE
// ──────────────────────────────────────────────────────────────────────────────

// Row is the only interface a test result type must satisfy.
// Implement this and the runner handles everything else.
type Row interface {
	// RowStatus returns the outcome of this row.
	RowStatus() Status

	// RowLatency is used for latency statistics (p50/p95/p99, histogram).
	// Return 0 if latency is not applicable.
	RowLatency() time.Duration

	// RowLabel is the primary identifier shown in the table (e.g. pack ID, user email).
	RowLabel() string

	// RowColumns defines the table columns rendered in the HTML report.
	// The headers from the first row are used as column headers.
	RowColumns() []Column

	// RowDetails returns expandable key-value pairs shown when the row is clicked.
	// Return nil for no expandable content.
	RowDetails() []Detail
}
