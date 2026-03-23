package reporter

import (
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"os"
	"path/filepath"
	"time"
)

// -------- DATA TYPES --------

// TestResult holds the outcome of a single API call.
type TestResult struct {
	Index     int    `json:"index"`
	ID        string `json:"packId"`
	Success   bool   `json:"success"`
	Status    int    `json:"httpStatus"`
	LatencyMs int64  `json:"latencyMs"`
	ErrorMsg  string `json:"error,omitempty"`
}

// BuyRevealResult holds the combined outcome of a single buy+reveal flow for
// one user account. It embeds the buy latency, reveal latency, and NFT details.
type BuyRevealResult struct {
	Index        int       `json:"index"`
	Email        string    `json:"email"`
	UserPackIDs  []string  `json:"userPackIds"`
	BuySuccess   bool      `json:"buySuccess"`
	BuyStatus    int       `json:"buyHttpStatus"`
	BuyLatencyMs int64     `json:"buyLatencyMs"`
	BuyError     string    `json:"buyError,omitempty"`
	RevealDone   bool      `json:"revealAttempted"`
	RevSuccess   bool      `json:"revealSuccess"`
	RevStatus    int       `json:"revealHttpStatus"`
	RevLatencyMs int64     `json:"revealLatencyMs"`
	RevError     string    `json:"revealError,omitempty"`
	NFTCount     int       `json:"nftCount"`
	TotalValue   float64   `json:"totalNFTValue"`
	NFTs         []NFTItem `json:"nfts,omitempty"`
}

// NFTItem is an individual card within a revealed pack.
type NFTItem struct {
	NFTTokenID string  `json:"nftTokenId"`
	CardName   string  `json:"cardName"`
	Rarity     string  `json:"rarity"`
	Value      float64 `json:"value"`
}

// Summary holds aggregate metrics for the test run.
type Summary struct {
	TotalRequests int     `json:"totalRequests"`
	SuccessCount  int     `json:"successCount"`
	FailureCount  int     `json:"failureCount"`
	SuccessRate   float64 `json:"successRatePercent"`
	AvgLatencyMs  int64   `json:"avgLatencyMs"`
	MinLatencyMs  int64   `json:"minLatencyMs"`
	MaxLatencyMs  int64   `json:"maxLatencyMs"`
	TotalTimeMs   int64   `json:"totalTimeMs"`
}

// BuyRevealSummary holds aggregate metrics for a buy+reveal run.
type BuyRevealSummary struct {
	TotalUsers         int     `json:"totalUsers"`
	BuySuccessCount    int     `json:"buySuccessCount"`
	BuyFailureCount    int     `json:"buyFailureCount"`
	RevealSuccessCount int     `json:"revealSuccessCount"`
	RevealFailureCount int     `json:"revealFailureCount"`
	TotalNFTs          int     `json:"totalNFTs"`
	TotalNFTValue      float64 `json:"totalNFTValue"`
	AvgNFTValue        float64 `json:"avgNFTValue"`
	AvgBuyLatencyMs    int64   `json:"avgBuyLatencyMs"`
	AvgRevLatencyMs    int64   `json:"avgRevealLatencyMs"`
	TotalTimeMs        int64   `json:"totalTimeMs"`
}

// Report is the top-level report object (used by pack-reveal flow).
type Report struct {
	RunAt       time.Time    `json:"runAt"`
	Environment string       `json:"environment"`
	Endpoint    string       `json:"endpoint"`
	Workers     int          `json:"workers"`
	Summary     Summary      `json:"summary"`
	Results     []TestResult `json:"results"`

	// internal accumulators
	minLatency int64
	maxLatency int64
	totalLat   int64
}

// BuyRevealReport is the top-level report for the buy+reveal flow.
type BuyRevealReport struct {
	RunAt       time.Time         `json:"runAt"`
	Environment string            `json:"environment"`
	PackConfig  BuyPackConfig     `json:"packConfig"`
	Summary     BuyRevealSummary  `json:"summary"`
	Results     []BuyRevealResult `json:"results"`

	// internal accumulators
	totalBuyLat int64
	totalRevLat int64
}

// BuyPackConfig stores the pack parameters used in the run.
type BuyPackConfig struct {
	PackMasterID  string `json:"packMasterId"`
	Quantity      int    `json:"quantity"`
	PriceConfigID string `json:"priceConfigId"`
	SpinnerBFFURL string `json:"spinnerBffUrl"`
}

// -------- BUILDER API (original reveal report) --------

// New creates an empty Report for the given run context.
func New(env, endpoint string, workers int) *Report {
	return &Report{
		RunAt:       time.Now(),
		Environment: env,
		Endpoint:    endpoint,
		Workers:     workers,
		minLatency:  math.MaxInt64,
	}
}

// AddResult appends a single result and keeps running totals updated.
func (r *Report) AddResult(res TestResult) {
	res.Index = len(r.Results) + 1
	r.Results = append(r.Results, res)

	r.totalLat += res.LatencyMs
	if res.LatencyMs < r.minLatency {
		r.minLatency = res.LatencyMs
	}
	if res.LatencyMs > r.maxLatency {
		r.maxLatency = res.LatencyMs
	}
	if res.Success {
		r.Summary.SuccessCount++
	} else {
		r.Summary.FailureCount++
	}
}

// Finalize computes all derived summary fields. Call once after all results
// have been added and the total wall-clock time is known.
func (r *Report) Finalize(totalTime time.Duration) {
	n := len(r.Results)
	r.Summary.TotalRequests = n
	r.Summary.TotalTimeMs = totalTime.Milliseconds()

	if n > 0 {
		r.Summary.SuccessRate = float64(r.Summary.SuccessCount) / float64(n) * 100
		r.Summary.AvgLatencyMs = r.totalLat / int64(n)
		r.Summary.MinLatencyMs = r.minLatency
		r.Summary.MaxLatencyMs = r.maxLatency
	}
	if r.minLatency == math.MaxInt64 {
		r.Summary.MinLatencyMs = 0
	}
}

// -------- BUILDER API (buy+reveal report) --------

// NewBuyReveal creates an empty BuyRevealReport.
func NewBuyReveal(env string, cfg BuyPackConfig) *BuyRevealReport {
	return &BuyRevealReport{
		RunAt:       time.Now(),
		Environment: env,
		PackConfig:  cfg,
	}
}

// AddBuyRevealResult appends a result and updates running totals.
func (r *BuyRevealReport) AddBuyRevealResult(res BuyRevealResult) {
	res.Index = len(r.Results) + 1
	r.Results = append(r.Results, res)

	if res.BuySuccess {
		r.Summary.BuySuccessCount++
	} else {
		r.Summary.BuyFailureCount++
	}
	r.totalBuyLat += res.BuyLatencyMs

	if res.RevealDone {
		if res.RevSuccess {
			r.Summary.RevealSuccessCount++
		} else {
			r.Summary.RevealFailureCount++
		}
		r.totalRevLat += res.RevLatencyMs
	}

	r.Summary.TotalNFTs += res.NFTCount
	r.Summary.TotalNFTValue += res.TotalValue
}

// FinalizeBuyReveal computes derived summary fields.
func (r *BuyRevealReport) FinalizeBuyReveal(totalTime time.Duration) {
	r.Summary.TotalUsers = len(r.Results)
	r.Summary.TotalTimeMs = totalTime.Milliseconds()

	if r.Summary.BuySuccessCount > 0 {
		r.Summary.AvgBuyLatencyMs = r.totalBuyLat / int64(r.Summary.BuySuccessCount+r.Summary.BuyFailureCount)
	}
	revTotal := r.Summary.RevealSuccessCount + r.Summary.RevealFailureCount
	if revTotal > 0 {
		r.Summary.AvgRevLatencyMs = r.totalRevLat / int64(revTotal)
	}
	if r.Summary.TotalNFTs > 0 {
		r.Summary.AvgNFTValue = r.Summary.TotalNFTValue / float64(r.Summary.TotalNFTs)
	}
}

// -------- WRITERS (original reveal report) --------

// WriteJSON saves the report as a formatted JSON file.
func (r *Report) WriteJSON(outDir string) (string, error) {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", fmt.Errorf("reporter: create dir: %w", err)
	}

	filename := fmt.Sprintf("reveal_%s.json", r.RunAt.Format("2006-01-02_15-04-05"))
	path := filepath.Join(outDir, filename)

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("reporter: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("reporter: write json: %w", err)
	}
	return path, nil
}

// WriteHTML saves the report as a self-contained HTML file.
func (r *Report) WriteHTML(outDir string) (string, error) {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", fmt.Errorf("reporter: create dir: %w", err)
	}

	filename := fmt.Sprintf("reveal_%s.html", r.RunAt.Format("2006-01-02_15-04-05"))
	path := filepath.Join(outDir, filename)

	tmpl, err := template.New("report").Parse(htmlTemplate)
	if err != nil {
		return "", fmt.Errorf("reporter: parse template: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("reporter: create html: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, r); err != nil {
		return "", fmt.Errorf("reporter: render template: %w", err)
	}
	return path, nil
}

// -------- WRITERS (buy+reveal report) --------

// WriteBuyRevealJSON saves the buy+reveal report as JSON.
func (r *BuyRevealReport) WriteBuyRevealJSON(outDir string) (string, error) {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", fmt.Errorf("reporter: create dir: %w", err)
	}
	filename := fmt.Sprintf("buy_reveal_%s.json", r.RunAt.Format("2006-01-02_15-04-05"))
	path := filepath.Join(outDir, filename)

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("reporter: marshal buy_reveal: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("reporter: write json: %w", err)
	}
	return path, nil
}

// WriteBuyRevealHTML saves the buy+reveal report as self-contained HTML.
func (r *BuyRevealReport) WriteBuyRevealHTML(outDir string) (string, error) {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", fmt.Errorf("reporter: create dir: %w", err)
	}
	filename := fmt.Sprintf("buy_reveal_%s.html", r.RunAt.Format("2006-01-02_15-04-05"))
	path := filepath.Join(outDir, filename)

	tmpl, err := template.New("buy_reveal_report").Funcs(template.FuncMap{
		"printf": fmt.Sprintf,
	}).Parse(buyRevealHTMLTemplate)
	if err != nil {
		return "", fmt.Errorf("reporter: parse buy_reveal template: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("reporter: create html: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, r); err != nil {
		return "", fmt.Errorf("reporter: render buy_reveal template: %w", err)
	}
	return path, nil
}

// -------- HTML TEMPLATE (original reveal report) --------

var htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0"/>
  <title>Pack Reveal Report · {{.Environment}}</title>
  <style>
    @import url('https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&family=JetBrains+Mono:wght@400;500&display=swap');

    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

    :root {
      --bg:        #0d0f14;
      --surface:   #151820;
      --border:    #1e2330;
      --text:      #e2e8f0;
      --muted:     #64748b;
      --green:     #22c55e;
      --red:       #ef4444;
      --blue:      #3b82f6;
      --yellow:    #f59e0b;
      --purple:    #a855f7;
      --radius:    12px;
    }

    body {
      font-family: 'Inter', sans-serif;
      background: var(--bg);
      color: var(--text);
      min-height: 100vh;
      padding: 40px 24px;
    }

    /* ── HEADER ── */
    .header {
      max-width: 1100px;
      margin: 0 auto 36px;
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      gap: 16px;
      flex-wrap: wrap;
    }
    .header-left h1 {
      font-size: 26px;
      font-weight: 700;
      background: linear-gradient(135deg, #60a5fa, #a78bfa);
      -webkit-background-clip: text;
      -webkit-text-fill-color: transparent;
      margin-bottom: 6px;
    }
    .header-left .meta {
      font-size: 13px;
      color: var(--muted);
      font-family: 'JetBrains Mono', monospace;
    }
    .badge {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 4px 12px;
      border-radius: 999px;
      font-size: 12px;
      font-weight: 600;
      letter-spacing: .4px;
    }
    .badge-pass { background: rgba(34,197,94,.15); color: var(--green); border: 1px solid rgba(34,197,94,.3); }
    .badge-fail { background: rgba(239,68,68,.15);  color: var(--red);   border: 1px solid rgba(239,68,68,.3); }

    /* ── CARDS ── */
    .cards {
      max-width: 1100px;
      margin: 0 auto 28px;
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
      gap: 14px;
    }
    .card {
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: var(--radius);
      padding: 18px 20px;
      position: relative;
      overflow: hidden;
    }
    .card::before {
      content: '';
      position: absolute;
      inset: 0;
      background: var(--card-glow, transparent);
      opacity: .08;
      pointer-events: none;
    }
    .card-label {
      font-size: 11px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: .8px;
      color: var(--muted);
      margin-bottom: 10px;
    }
    .card-value {
      font-size: 28px;
      font-weight: 700;
      line-height: 1;
    }
    .card-sub {
      font-size: 11px;
      color: var(--muted);
      margin-top: 4px;
    }
    .card-total    { --card-glow: linear-gradient(135deg, #3b82f6, #2563eb); }
    .card-pass     { --card-glow: linear-gradient(135deg, #22c55e, #16a34a); }
    .card-fail     { --card-glow: linear-gradient(135deg, #ef4444, #dc2626); }
    .card-rate     { --card-glow: linear-gradient(135deg, #a855f7, #7c3aed); }
    .card-latency  { --card-glow: linear-gradient(135deg, #f59e0b, #d97706); }
    .card-time     { --card-glow: linear-gradient(135deg, #06b6d4, #0891b2); }

    .val-green  { color: var(--green); }
    .val-red    { color: var(--red);   }
    .val-blue   { color: var(--blue);  }
    .val-purple { color: var(--purple);}
    .val-yellow { color: var(--yellow);}
    .val-cyan   { color: #22d3ee; }

    /* ── PROGRESS BAR ── */
    .progress-wrap {
      max-width: 1100px;
      margin: 0 auto 28px;
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: var(--radius);
      padding: 16px 20px;
    }
    .progress-header {
      display: flex;
      justify-content: space-between;
      font-size: 12px;
      color: var(--muted);
      margin-bottom: 10px;
    }
    .progress-track {
      height: 8px;
      background: var(--border);
      border-radius: 999px;
      overflow: hidden;
    }
    .progress-fill {
      height: 100%;
      border-radius: 999px;
      background: linear-gradient(90deg, #22c55e, #86efac);
      transition: width .6s ease;
    }
    .progress-fill.partial {
      background: linear-gradient(90deg, #f59e0b, #fcd34d);
    }
    .progress-fill.fail {
      background: linear-gradient(90deg, #ef4444, #fca5a5);
    }

    /* ── ENDPOINT ── */
    .endpoint-wrap {
      max-width: 1100px;
      margin: 0 auto 28px;
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: var(--radius);
      padding: 14px 20px;
      display: flex;
      align-items: center;
      gap: 10px;
    }
    .endpoint-label {
      font-size: 11px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: .8px;
      color: var(--muted);
      white-space: nowrap;
    }
    .endpoint-url {
      font-family: 'JetBrains Mono', monospace;
      font-size: 13px;
      color: #60a5fa;
      word-break: break-all;
    }

    /* ── TABLE ── */
    .table-wrap {
      max-width: 1100px;
      margin: 0 auto;
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: var(--radius);
      overflow: hidden;
    }
    .table-header {
      padding: 16px 20px 12px;
      border-bottom: 1px solid var(--border);
      display: flex;
      align-items: center;
      justify-content: space-between;
    }
    .table-header h2 {
      font-size: 14px;
      font-weight: 600;
      color: var(--text);
    }
    table {
      width: 100%;
      border-collapse: collapse;
    }
    thead th {
      padding: 12px 16px;
      font-size: 11px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: .8px;
      color: var(--muted);
      border-bottom: 1px solid var(--border);
      text-align: left;
    }
    tbody tr {
      border-bottom: 1px solid var(--border);
      transition: background .15s;
    }
    tbody tr:last-child { border-bottom: none; }
    tbody tr:hover { background: rgba(255,255,255,.03); }
    td {
      padding: 11px 16px;
      font-size: 13px;
      vertical-align: middle;
    }
    .td-index { color: var(--muted); font-family: 'JetBrains Mono', monospace; font-size: 12px; }
    .td-id    { font-family: 'JetBrains Mono', monospace; font-size: 12px; color: #94a3b8; }
    .td-lat   { font-family: 'JetBrains Mono', monospace; font-size: 12px; }
    .td-code  { font-family: 'JetBrains Mono', monospace; font-size: 12px; }
    .td-err   { font-size: 12px; color: var(--red); font-family: 'JetBrains Mono', monospace; max-width: 300px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

    .status-dot {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      font-weight: 500;
      font-size: 12px;
    }
    .dot { width: 7px; height: 7px; border-radius: 50%; }
    .dot-green { background: var(--green); box-shadow: 0 0 6px var(--green); }
    .dot-red   { background: var(--red);   box-shadow: 0 0 6px var(--red); }

    /* ── FOOTER ── */
    .footer {
      max-width: 1100px;
      margin: 28px auto 0;
      text-align: center;
      font-size: 12px;
      color: var(--muted);
    }
  </style>
</head>
<body>

<!-- HEADER -->
<div class="header">
  <div class="header-left">
    <h1>🎴 Pack Reveal Report</h1>
    <div class="meta">
      {{.RunAt.Format "Mon, 02 Jan 2006 · 15:04:05 MST"}} &nbsp;·&nbsp;
      ENV: <strong>{{.Environment}}</strong> &nbsp;·&nbsp;
      Workers: <strong>{{.Workers}}</strong>
    </div>
  </div>
  {{if eq .Summary.FailureCount 0}}
    <span class="badge badge-pass">✓ All Passed</span>
  {{else}}
    <span class="badge badge-fail">✗ {{.Summary.FailureCount}} Failed</span>
  {{end}}
</div>

<!-- METRIC CARDS -->
<div class="cards">
  <div class="card card-total">
    <div class="card-label">Total Packs</div>
    <div class="card-value val-blue">{{.Summary.TotalRequests}}</div>
    <div class="card-sub">requests sent</div>
  </div>
  <div class="card card-pass">
    <div class="card-label">Passed</div>
    <div class="card-value val-green">{{.Summary.SuccessCount}}</div>
    <div class="card-sub">revealed successfully</div>
  </div>
  <div class="card card-fail">
    <div class="card-label">Failed</div>
    <div class="card-value val-red">{{.Summary.FailureCount}}</div>
    <div class="card-sub">reveal errors</div>
  </div>
  <div class="card card-rate">
    <div class="card-label">Success Rate</div>
    <div class="card-value val-purple">{{printf "%.1f" .Summary.SuccessRate}}%</div>
    <div class="card-sub">pass rate</div>
  </div>
  <div class="card card-latency">
    <div class="card-label">Avg Latency</div>
    <div class="card-value val-yellow">{{.Summary.AvgLatencyMs}}<span style="font-size:14px;font-weight:400"> ms</span></div>
    <div class="card-sub">min {{.Summary.MinLatencyMs}}ms · max {{.Summary.MaxLatencyMs}}ms</div>
  </div>
  <div class="card card-time">
    <div class="card-label">Total Time</div>
    <div class="card-value val-cyan">{{.Summary.TotalTimeMs}}<span style="font-size:14px;font-weight:400"> ms</span></div>
    <div class="card-sub">wall-clock duration</div>
  </div>
</div>

<!-- PROGRESS BAR -->
<div class="progress-wrap">
  <div class="progress-header">
    <span>Pass Rate</span>
    <span>{{.Summary.SuccessCount}} / {{.Summary.TotalRequests}}</span>
  </div>
  <div class="progress-track">
    <div class="progress-fill{{if eq .Summary.FailureCount 0}}{{else if lt .Summary.SuccessRate 50.0}} fail{{else}} partial{{end}}"
         style="width: {{printf "%.1f" .Summary.SuccessRate}}%"></div>
  </div>
</div>

<!-- ENDPOINT -->
<div class="endpoint-wrap">
  <span class="endpoint-label">Endpoint</span>
  <span class="endpoint-url">{{.Endpoint}}</span>
</div>

<!-- RESULTS TABLE -->
<div class="table-wrap">
  <div class="table-header">
    <h2>Individual Results</h2>
    <span class="badge {{if eq .Summary.FailureCount 0}}badge-pass{{else}}badge-fail{{end}}">
      {{.Summary.TotalRequests}} records
    </span>
  </div>
  <table>
    <thead>
      <tr>
        <th>#</th>
        <th>Pack ID</th>
        <th>Status</th>
        <th>HTTP</th>
        <th>Latency</th>
        <th>Error</th>
      </tr>
    </thead>
    <tbody>
      {{range .Results}}
      <tr>
        <td class="td-index">{{.Index}}</td>
        <td class="td-id">{{.ID}}</td>
        <td>
          {{if .Success}}
            <span class="status-dot"><span class="dot dot-green"></span> Pass</span>
          {{else}}
            <span class="status-dot"><span class="dot dot-red"></span> Fail</span>
          {{end}}
        </td>
        <td class="td-code" style="color: {{if .Success}}#22c55e{{else}}#ef4444{{end}}">
          {{if eq .Status 0}}—{{else}}{{.Status}}{{end}}
        </td>
        <td class="td-lat" style="color: {{if gt .LatencyMs 2000}}#ef4444{{else if gt .LatencyMs 1000}}#f59e0b{{else}}#94a3b8{{end}}">
          {{.LatencyMs}} ms
        </td>
        <td class="td-err">{{if .ErrorMsg}}{{.ErrorMsg}}{{else}}—{{end}}</td>
      </tr>
      {{end}}
    </tbody>
  </table>
</div>

<div class="footer">
  Generated by code-go-automation &nbsp;·&nbsp; {{.RunAt.Format "2006-01-02 15:04:05"}}
</div>

</body>
</html>`

// -------- HTML TEMPLATE (buy+reveal report) --------

var buyRevealHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8"/>
  <meta name="viewport" content="width=device-width, initial-scale=1.0"/>
  <title>Buy &amp; Reveal Report · {{.Environment}}</title>
  <style>
    @import url('https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&family=JetBrains+Mono:wght@400;500&display=swap');
    *,*::before,*::after{box-sizing:border-box;margin:0;padding:0;}
    :root{
      --bg:#0d0f14;--surface:#151820;--border:#1e2330;
      --text:#e2e8f0;--muted:#64748b;
      --green:#22c55e;--red:#ef4444;--blue:#3b82f6;
      --yellow:#f59e0b;--purple:#a855f7;--cyan:#22d3ee;
      --radius:12px;
    }
    body{font-family:'Inter',sans-serif;background:var(--bg);color:var(--text);min-height:100vh;padding:40px 24px;}

    /* HEADER */
    .header{max-width:1200px;margin:0 auto 36px;display:flex;align-items:flex-start;justify-content:space-between;gap:16px;flex-wrap:wrap;}
    .header-left h1{font-size:26px;font-weight:700;background:linear-gradient(135deg,#f59e0b,#ec4899);-webkit-background-clip:text;-webkit-text-fill-color:transparent;margin-bottom:6px;}
    .header-left .meta{font-size:13px;color:var(--muted);font-family:'JetBrains Mono',monospace;}
    .badge{display:inline-flex;align-items:center;gap:6px;padding:4px 12px;border-radius:999px;font-size:12px;font-weight:600;letter-spacing:.4px;}
    .badge-pass{background:rgba(34,197,94,.15);color:var(--green);border:1px solid rgba(34,197,94,.3);}
    .badge-fail{background:rgba(239,68,68,.15);color:var(--red);border:1px solid rgba(239,68,68,.3);}
    .badge-info{background:rgba(59,130,246,.15);color:var(--blue);border:1px solid rgba(59,130,246,.3);}

    /* CARDS */
    .cards{max-width:1200px;margin:0 auto 28px;display:grid;grid-template-columns:repeat(auto-fit,minmax(170px,1fr));gap:14px;}
    .card{background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);padding:18px 20px;position:relative;overflow:hidden;}
    .card::before{content:'';position:absolute;inset:0;background:var(--card-glow,transparent);opacity:.08;pointer-events:none;}
    .card-label{font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:.8px;color:var(--muted);margin-bottom:10px;}
    .card-value{font-size:28px;font-weight:700;line-height:1;}
    .card-sub{font-size:11px;color:var(--muted);margin-top:4px;}
    .c-users  {--card-glow:linear-gradient(135deg,#3b82f6,#2563eb);}
    .c-buy    {--card-glow:linear-gradient(135deg,#22c55e,#16a34a);}
    .c-reveal {--card-glow:linear-gradient(135deg,#a855f7,#7c3aed);}
    .c-nfts   {--card-glow:linear-gradient(135deg,#f59e0b,#d97706);}
    .c-value  {--card-glow:linear-gradient(135deg,#ec4899,#db2777);}
    .c-time   {--card-glow:linear-gradient(135deg,#06b6d4,#0891b2);}
    .val-blue{color:var(--blue);}.val-green{color:var(--green);}.val-purple{color:var(--purple);}
    .val-yellow{color:var(--yellow);}.val-pink{color:#f472b6;}.val-cyan{color:var(--cyan);}

    /* PACK CONFIG */
    .config-wrap{max-width:1200px;margin:0 auto 28px;background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);padding:16px 20px;display:flex;flex-wrap:wrap;gap:24px;}
    .config-item{display:flex;flex-direction:column;gap:4px;}
    .config-label{font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:.8px;color:var(--muted);}
    .config-val{font-family:'JetBrains Mono',monospace;font-size:13px;color:#60a5fa;}

    /* TABLE */
    .table-wrap{max-width:1200px;margin:0 auto 28px;background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);overflow:hidden;}
    .table-header{padding:16px 20px 12px;border-bottom:1px solid var(--border);display:flex;align-items:center;justify-content:space-between;}
    .table-header h2{font-size:14px;font-weight:600;}
    table{width:100%;border-collapse:collapse;}
    thead th{padding:12px 16px;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:.8px;color:var(--muted);border-bottom:1px solid var(--border);text-align:left;}
    tbody tr{border-bottom:1px solid var(--border);transition:background .15s;}
    tbody tr:last-child{border-bottom:none;}
    tbody tr:hover{background:rgba(255,255,255,.03);}
    td{padding:11px 16px;font-size:13px;vertical-align:middle;}
    .mono{font-family:'JetBrains Mono',monospace;font-size:12px;}
    .muted{color:var(--muted);}
    .status-dot{display:inline-flex;align-items:center;gap:6px;font-weight:500;font-size:12px;}
    .dot{width:7px;height:7px;border-radius:50%;}
    .dot-green{background:var(--green);box-shadow:0 0 6px var(--green);}
    .dot-red{background:var(--red);box-shadow:0 0 6px var(--red);}
    .dot-grey{background:var(--muted);}

    /* NFT PILL LIST */
    .nft-pills{display:flex;flex-wrap:wrap;gap:4px;max-width:320px;}
    .nft-pill{display:inline-flex;align-items:center;gap:4px;padding:2px 8px;border-radius:999px;font-size:10px;font-weight:600;font-family:'JetBrains Mono',monospace;background:rgba(168,85,247,.12);border:1px solid rgba(168,85,247,.25);color:#c084fc;}
    .nft-pill.common{background:rgba(100,116,139,.12);border-color:rgba(100,116,139,.25);color:#94a3b8;}
    .nft-pill.rare{background:rgba(59,130,246,.12);border-color:rgba(59,130,246,.25);color:#60a5fa;}
    .nft-pill.epic{background:rgba(168,85,247,.12);border-color:rgba(168,85,247,.25);color:#c084fc;}
    .nft-pill.legendary{background:rgba(245,158,11,.12);border-color:rgba(245,158,11,.25);color:#fbbf24;}

    .footer{max-width:1200px;margin:28px auto 0;text-align:center;font-size:12px;color:var(--muted);}
  </style>
</head>
<body>

<!-- HEADER -->
<div class="header">
  <div class="header-left">
    <h1>🛍️ Buy &amp; Reveal Report</h1>
    <div class="meta">
      {{.RunAt.Format "Mon, 02 Jan 2006 · 15:04:05 MST"}} &nbsp;·&nbsp;
      ENV: <strong>{{.Environment}}</strong>
    </div>
  </div>
  {{if eq .Summary.BuyFailureCount 0}}
    <span class="badge badge-pass">✓ All Buys Succeeded</span>
  {{else}}
    <span class="badge badge-fail">✗ {{.Summary.BuyFailureCount}} Buy(s) Failed</span>
  {{end}}
</div>

<!-- PACK CONFIG -->
<div class="config-wrap">
  <div class="config-item">
    <span class="config-label">Pack Master ID</span>
    <span class="config-val">{{.PackConfig.PackMasterID}}</span>
  </div>
  <div class="config-item">
    <span class="config-label">Quantity</span>
    <span class="config-val">{{.PackConfig.Quantity}}</span>
  </div>
  <div class="config-item">
    <span class="config-label">Price Config ID</span>
    <span class="config-val">{{.PackConfig.PriceConfigID}}</span>
  </div>
  <div class="config-item">
    <span class="config-label">Spinner BFF URL</span>
    <span class="config-val">{{.PackConfig.SpinnerBFFURL}}</span>
  </div>
</div>

<!-- METRIC CARDS -->
<div class="cards">
  <div class="card c-users">
    <div class="card-label">Total Users</div>
    <div class="card-value val-blue">{{.Summary.TotalUsers}}</div>
    <div class="card-sub">concurrent goroutines</div>
  </div>
  <div class="card c-buy">
    <div class="card-label">Buys OK</div>
    <div class="card-value val-green">{{.Summary.BuySuccessCount}}</div>
    <div class="card-sub">{{.Summary.BuyFailureCount}} failed</div>
  </div>
  <div class="card c-reveal">
    <div class="card-label">Reveals OK</div>
    <div class="card-value val-purple">{{.Summary.RevealSuccessCount}}</div>
    <div class="card-sub">{{.Summary.RevealFailureCount}} failed</div>
  </div>
  <div class="card c-nfts">
    <div class="card-label">Total NFTs</div>
    <div class="card-value val-yellow">{{.Summary.TotalNFTs}}</div>
    <div class="card-sub">across all packs</div>
  </div>
  <div class="card c-value">
    <div class="card-label">Total Value</div>
    <div class="card-value val-pink">{{printf "%.2f" .Summary.TotalNFTValue}}</div>
    <div class="card-sub">avg {{printf "%.2f" .Summary.AvgNFTValue}} per NFT</div>
  </div>
  <div class="card c-time">
    <div class="card-label">Total Time</div>
    <div class="card-value val-cyan">{{.Summary.TotalTimeMs}}<span style="font-size:14px;font-weight:400"> ms</span></div>
    <div class="card-sub">avg buy {{.Summary.AvgBuyLatencyMs}}ms · reveal {{.Summary.AvgRevLatencyMs}}ms</div>
  </div>
</div>

<!-- RESULTS TABLE -->
<div class="table-wrap">
  <div class="table-header">
    <h2>Per-User Results</h2>
    <span class="badge badge-info">{{.Summary.TotalUsers}} users</span>
  </div>
  <table>
    <thead>
      <tr>
        <th>#</th>
        <th>User</th>
        <th>Buy</th>
        <th>Buy ms</th>
        <th>User Pack ID</th>
        <th>Reveal</th>
        <th>Rev ms</th>
        <th>NFTs</th>
        <th>Value</th>
        <th>Cards</th>
      </tr>
    </thead>
    <tbody>
      {{range .Results}}
      <tr>
        <td class="mono muted">{{.Index}}</td>
        <td class="mono" style="color:#94a3b8">{{.Email}}</td>
        <td>
          {{if .BuySuccess}}
            <span class="status-dot"><span class="dot dot-green"></span> OK</span>
          {{else}}
            <span class="status-dot"><span class="dot dot-red"></span> FAIL</span>
          {{end}}
          {{if .BuyError}}<div class="mono" style="color:var(--red);font-size:10px;margin-top:2px">{{.BuyError}}</div>{{end}}
        </td>
        <td class="mono" style="color:{{if gt .BuyLatencyMs 3000}}#ef4444{{else if gt .BuyLatencyMs 1500}}#f59e0b{{else}}#94a3b8{{end}}">{{.BuyLatencyMs}}</td>
        <td class="mono" style="color:#60a5fa;font-size:11px">{{if .UserPackIDs}}{{range $i,$id := .UserPackIDs}}{{if $i}}<br/>{{end}}{{$id}}{{end}}{{else}}—{{end}}</td>
        <td>
          {{if .RevealDone}}
            {{if .RevSuccess}}
              <span class="status-dot"><span class="dot dot-green"></span> OK</span>
            {{else}}
              <span class="status-dot"><span class="dot dot-red"></span> FAIL</span>
            {{end}}
            {{if .RevError}}<div class="mono" style="color:var(--red);font-size:10px;margin-top:2px">{{.RevError}}</div>{{end}}
          {{else}}
            <span class="status-dot"><span class="dot dot-grey"></span> Skipped</span>
          {{end}}
        </td>
        <td class="mono" style="color:{{if gt .RevLatencyMs 5000}}#ef4444{{else if gt .RevLatencyMs 2000}}#f59e0b{{else}}#94a3b8{{end}}">{{if .RevealDone}}{{.RevLatencyMs}}{{else}}—{{end}}</td>
        <td class="mono val-yellow">{{.NFTCount}}</td>
        <td class="mono val-pink">{{printf "%.2f" .TotalValue}}</td>
        <td>
          <div class="nft-pills">
            {{range .NFTs}}
            <span class="nft-pill {{.Rarity}}" title="{{.CardName}} · {{printf "%.2f" .Value}}">{{.CardName}}</span>
            {{end}}
          </div>
        </td>
      </tr>
      {{end}}
    </tbody>
  </table>
</div>

<div class="footer">
  Generated by code-go-automation &nbsp;·&nbsp; {{.RunAt.Format "2006-01-02 15:04:05"}}
</div>
</body>
</html>`
