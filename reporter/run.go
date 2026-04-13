package reporter

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// RUN REPORT
//
// WriteRunHTML produces ONE combined HTML file for a test run regardless of
// whether 1 or N tests were executed.
//
// Layout:
//   • Run header  — env, branch, commit, timestamp
//   • Overview    — per-test summary cards side by side
//   • Sections    — one collapsible section per test containing:
//                   annotations, latency histogram + stats, full result table
//                   (failed sections auto-expand on load)
// ──────────────────────────────────────────────────────────────────────────────

// RunReport is the envelope written to the combined HTML.
type RunReport struct {
	RunAt   time.Time
	Env     string
	Reports []*Report
}

// WriteRunHTML writes a single combined HTML file for all reports.
// Always writes — even for a single test.
// Returns the path to the written file.
func WriteRunHTML(outDir, env string, reports ...*Report) (string, error) {
	if len(reports) == 0 {
		return "", nil
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", fmt.Errorf("reporter: mkdir: %w", err)
	}

	run := &RunReport{
		RunAt:   time.Now(),
		Env:     env,
		Reports: reports,
	}

	filename := fmt.Sprintf("run_%s.html", run.RunAt.Format("2006-01-02_15-04-05"))
	path := filepath.Join(outDir, filename)

	funcMap := template.FuncMap{
		"overall": func(r *Report) Status {
			if r.Summary.FailCount > 0 {
				return StatusFail
			}
			if r.Summary.WarnCount > 0 {
				return StatusWarning
			}
			return StatusPass
		},
		"statusClass": func(s Status) string {
			switch s {
			case StatusPass:
				return "pass"
			case StatusFail:
				return "fail"
			case StatusWarning:
				return "warn"
			default:
				return "skip"
			}
		},
		"statusLabel": func(s Status) string {
			switch s {
			case StatusPass:
				return "PASS"
			case StatusFail:
				return "FAIL"
			case StatusWarning:
				return "WARN"
			default:
				return "SKIP"
			}
		},
		"hasDetails": func(row RowRecord) bool {
			return len(row.Details) > 0
		},
		"hasFails": func(r *Report) bool {
			return r.Summary.FailCount > 0
		},
		"inc": func(i int) int { return i + 1 },
		"printf": fmt.Sprintf,
		"totalItems": func(reports []*Report) int {
			n := 0
			for _, r := range reports {
				n += r.Summary.Total
			}
			return n
		},
		"totalFails": func(reports []*Report) int {
			n := 0
			for _, r := range reports {
				n += r.Summary.FailCount
			}
			return n
		},
		"overallStatus": func(reports []*Report) Status {
			for _, r := range reports {
				if r.Summary.FailCount > 0 {
					return StatusFail
				}
			}
			for _, r := range reports {
				if r.Summary.WarnCount > 0 {
					return StatusWarning
				}
			}
			return StatusPass
		},
	}

	tmpl, err := template.New("run").Funcs(funcMap).Parse(runHTMLTemplate)
	if err != nil {
		return "", fmt.Errorf("reporter: parse run template: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("reporter: create run html: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, run); err != nil {
		return "", fmt.Errorf("reporter: render run template: %w", err)
	}
	return path, nil
}

var runHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8"/>
  <meta name="viewport" content="width=device-width,initial-scale=1"/>
  <title>Run Report · {{.Env}} · {{.RunAt.Format "2006-01-02 15:04"}}</title>
  <style>
    *, *::before, *::after { box-sizing:border-box; margin:0; padding:0; }
    :root {
      --bg:#0d0f14; --surface:#151820; --surface2:#1a1f2e; --border:#1e2330;
      --text:#e2e8f0; --muted:#64748b; --mono:'JetBrains Mono',monospace;
      --pass:#22c55e; --fail:#ef4444; --warn:#f59e0b; --skip:#64748b;
      --blue:#3b82f6; --purple:#a855f7; --radius:10px;
    }
    body { font-family:'Inter',system-ui,sans-serif; background:var(--bg); color:var(--text); padding:32px 20px; min-height:100vh; }
    .wrap { max-width:1300px; margin:0 auto; }

    /* ── RUN HEADER ── */
    .run-header { margin-bottom:28px; }
    .run-header h1 { font-size:26px; font-weight:700;
      background:linear-gradient(135deg,#60a5fa,#a78bfa);
      -webkit-background-clip:text; -webkit-text-fill-color:transparent; margin-bottom:8px; }
    .run-meta { display:flex; flex-wrap:wrap; gap:8px; font-size:12px; }
    .run-meta span { background:var(--surface2); border:1px solid var(--border);
      padding:3px 10px; border-radius:999px; color:var(--muted); }
    .run-badge { display:inline-flex; align-items:center; gap:5px; padding:4px 14px;
      border-radius:999px; font-size:12px; font-weight:700; margin-left:auto; }
    .run-badge::before { content:'●'; font-size:9px; }
    .run-badge.pass { background:rgba(34,197,94,.12); color:var(--pass); border:1px solid rgba(34,197,94,.3); }
    .run-badge.fail { background:rgba(239,68,68,.12); color:var(--fail); border:1px solid rgba(239,68,68,.3); }
    .run-badge.warn { background:rgba(245,158,11,.12); color:var(--warn); border:1px solid rgba(245,158,11,.3); }

    /* ── OVERVIEW CARDS ── */
    .overview { display:grid; grid-template-columns:repeat(auto-fit,minmax(160px,1fr)); gap:14px; margin-bottom:32px; }
    .card { background:var(--surface); border:1px solid var(--border); border-radius:var(--radius); padding:18px 16px; }
    .card-label { font-size:10px; color:var(--muted); text-transform:uppercase; letter-spacing:.8px; margin-bottom:8px; }
    .card-value { font-size:28px; font-weight:700; }
    .c-pass{color:var(--pass);} .c-fail{color:var(--fail);} .c-warn{color:var(--warn);}
    .c-blue{color:var(--blue);} .c-purple{color:var(--purple);} .c-skip{color:var(--skip);}

    /* ── TEST SUMMARY STRIP ── */
    .test-strip { display:grid; grid-template-columns:repeat(auto-fit,minmax(280px,1fr)); gap:14px; margin-bottom:36px; }
    .test-thumb { background:var(--surface); border:1px solid var(--border); border-radius:var(--radius); padding:16px 18px; }
    .test-thumb.fail { border-color:rgba(239,68,68,.5); background:rgba(239,68,68,.04); }
    .test-thumb.pass { border-color:rgba(34,197,94,.25); }
    .test-thumb.warn { border-color:rgba(245,158,11,.5); background:rgba(245,158,11,.04); }
    .thumb-name { font-size:14px; font-weight:600; display:flex; align-items:center; justify-content:space-between; margin-bottom:10px; }
    .badge { display:inline-flex; align-items:center; gap:4px; padding:2px 10px;
      border-radius:999px; font-size:11px; font-weight:700; white-space:nowrap; }
    .badge::before { content:'●'; font-size:8px; }
    .badge.pass { background:rgba(34,197,94,.12); color:var(--pass); border:1px solid rgba(34,197,94,.25); }
    .badge.fail { background:rgba(239,68,68,.12); color:var(--fail); border:1px solid rgba(239,68,68,.25); }
    .badge.warn { background:rgba(245,158,11,.12); color:var(--warn); border:1px solid rgba(245,158,11,.25); }
    .badge.skip { background:rgba(100,116,139,.12); color:var(--skip); border:1px solid rgba(100,116,139,.25); }
    .thumb-stats { display:grid; grid-template-columns:1fr 1fr; gap:4px 12px; font-size:12px; }
    .thumb-stat { display:flex; justify-content:space-between; padding:3px 0; border-bottom:1px solid var(--border); }
    .thumb-stat:last-child { border-bottom:none; }
    .ts-label { color:var(--muted); }
    .ts-val { font-family:var(--mono); }
    .thumb-link { display:block; margin-top:10px; font-size:11px; color:var(--blue); text-decoration:none; }
    .thumb-link:hover { text-decoration:underline; }
    .ann-chips { margin-top:10px; display:flex; flex-wrap:wrap; gap:5px; }
    .ann-chip { background:var(--surface2); border:1px solid var(--border); border-radius:6px;
      padding:2px 8px; font-size:11px; }
    .ann-k { color:var(--muted); margin-right:4px; }
    .ann-v { font-family:var(--mono); }

    /* ── SECTION (per test detail) ── */
    .section { border:1px solid var(--border); border-radius:var(--radius); margin-bottom:24px; overflow:hidden; }
    .section.fail { border-color:rgba(239,68,68,.4); }
    .section.pass { border-color:rgba(34,197,94,.2); }
    .section-header { display:flex; align-items:center; justify-content:space-between;
      padding:14px 20px; background:var(--surface2); cursor:pointer; user-select:none; }
    .section-header:hover { background:#1e2540; }
    .section-title { font-size:15px; font-weight:600; display:flex; align-items:center; gap:10px; }
    .section-chevron { color:var(--muted); font-size:12px; transition:transform .2s; }
    .section-chevron.open { transform:rotate(180deg); }
    .section-body { display:none; padding:20px; }
    .section-body.open { display:block; }

    /* ── ANNOTATIONS ── */
    .annotations { background:var(--surface2); border:1px solid var(--border); border-radius:8px;
      padding:14px 18px; margin-bottom:18px; }
    .annotations h4 { font-size:10px; color:var(--muted); text-transform:uppercase; letter-spacing:.8px; margin-bottom:10px; }
    .ann-grid { display:flex; flex-wrap:wrap; gap:7px; }

    /* ── HISTOGRAM ── */
    .histogram { background:var(--surface2); border:1px solid var(--border); border-radius:8px;
      padding:14px 18px; margin-bottom:18px; }
    .histogram h4 { font-size:10px; color:var(--muted); text-transform:uppercase; letter-spacing:.8px; margin-bottom:12px; }
    .hist-row { display:flex; align-items:center; gap:10px; margin-bottom:7px; font-size:12px; }
    .hist-label { width:120px; color:var(--muted); flex-shrink:0; }
    .hist-bar-wrap { flex:1; background:var(--bg); border-radius:4px; height:12px; }
    .hist-bar { height:100%; border-radius:4px; background:linear-gradient(90deg,#3b82f6,#a78bfa); }
    .hist-count { width:36px; text-align:right; font-family:var(--mono); font-size:11px; }
    .lat-row { display:flex; flex-wrap:wrap; gap:8px; margin-bottom:18px; }
    .lat-pill { background:var(--surface2); border:1px solid var(--border); border-radius:6px;
      padding:6px 14px; font-size:12px; }
    .lp-label { color:var(--muted); margin-right:5px; }
    .lp-val { font-family:var(--mono); color:var(--blue); font-weight:600; }

    /* ── TABLE ── */
    .table-wrap { overflow-x:auto; border-radius:8px; border:1px solid var(--border); }
    table { width:100%; border-collapse:collapse; font-size:12px; background:var(--surface); }
    thead th { background:var(--surface2); color:var(--muted); font-weight:600;
      text-transform:uppercase; letter-spacing:.5px; font-size:10px;
      padding:10px 12px; text-align:left; white-space:nowrap; }
    tbody tr { border-top:1px solid var(--border); cursor:pointer; transition:background .12s; }
    tbody tr:hover { background:rgba(255,255,255,.02); }
    tbody tr.expanded { background:rgba(59,130,246,.05); }
    td { padding:9px 12px; vertical-align:middle; }
    td.mono { font-family:var(--mono); font-size:11px; word-break:break-all; }
    .expander { color:var(--muted); font-size:10px; margin-right:4px; }

    /* ── DETAIL ROW ── */
    .detail-row { display:none; }
    .detail-row.open { display:table-row; }
    .detail-row td { padding:0; border-top:none; background:var(--surface2); }
    .detail-box { padding:14px 18px; border-bottom:1px solid var(--border); }
    .detail-box h5 { font-size:10px; color:var(--muted); text-transform:uppercase; letter-spacing:.8px; margin-bottom:10px; }
    .detail-grid { display:grid; grid-template-columns:160px 1fr; gap:5px 14px; font-size:12px; }
    .dk { color:var(--muted); padding-top:2px; }
    .dv { color:var(--text); word-break:break-all; }
    .dv.mono { font-family:var(--mono); font-size:11px; white-space:pre-wrap; }

    /* ── FOOTER ── */
    .footer { text-align:center; color:var(--muted); font-size:11px; margin-top:32px; }
  </style>
</head>
<body>
<div class="wrap">

  <!-- RUN HEADER -->
  <div class="run-header">
    <div style="display:flex;align-items:flex-start;justify-content:space-between;flex-wrap:wrap;gap:12px;">
      <h1>Run Report</h1>
      {{$os := overallStatus .Reports}}
      <span class="run-badge {{statusClass $os}}">{{statusLabel $os}}</span>
    </div>
    <div class="run-meta" style="margin-top:10px;">
      <span>env: {{.Env}}</span>
      {{with (index .Reports 0)}}
        {{if .Meta.GitBranch}}<span>branch: {{.Meta.GitBranch}}</span>{{end}}
        {{if .Meta.GitCommit}}<span>commit: {{.Meta.GitCommit}}</span>{{end}}
      {{end}}
      <span>{{.RunAt.Format "2006-01-02 15:04:05"}}</span>
      <span>{{len .Reports}} test(s)</span>
    </div>
  </div>

  <!-- OVERVIEW CARDS -->
  <div class="overview">
    <div class="card"><div class="card-label">Tests Run</div><div class="card-value c-blue">{{len .Reports}}</div></div>
    <div class="card"><div class="card-label">Total Items</div><div class="card-value c-blue">{{totalItems .Reports}}</div></div>
    <div class="card">
      <div class="card-label">Total Failures</div>
      <div class="card-value {{if gt (totalFails .Reports) 0}}c-fail{{else}}c-pass{{end}}">{{totalFails .Reports}}</div>
    </div>
  </div>

  <!-- TEST SUMMARY STRIP -->
  <div class="test-strip">
    {{range $i, $r := .Reports}}
    {{$ov := overall $r}}
    <div class="test-thumb {{statusClass $ov}}">
      <div class="thumb-name">
        {{$r.Name}}
        <span class="badge {{statusClass $ov}}">{{statusLabel $ov}}</span>
      </div>
      <div class="thumb-stats">
        <div class="thumb-stat"><span class="ts-label">Total</span><span class="ts-val">{{$r.Summary.Total}}</span></div>
        <div class="thumb-stat"><span class="ts-label">Pass</span><span class="ts-val c-pass">{{$r.Summary.PassCount}}</span></div>
        <div class="thumb-stat"><span class="ts-label">Fail</span><span class="ts-val {{if gt $r.Summary.FailCount 0}}c-fail{{else}}c-pass{{end}}">{{$r.Summary.FailCount}}</span></div>
        <div class="thumb-stat"><span class="ts-label">Rate</span><span class="ts-val">{{printf "%.1f" $r.Summary.SuccessRate}}%</span></div>
        <div class="thumb-stat"><span class="ts-label">p95</span><span class="ts-val">{{$r.Summary.Latency.P95Ms}} ms</span></div>
        <div class="thumb-stat"><span class="ts-label">wall</span><span class="ts-val">{{$r.Summary.WallTimeMs}} ms</span></div>
      </div>
      {{if $r.Annotations}}
      <div class="ann-chips">
        {{range $k,$v := $r.Annotations}}
        <div class="ann-chip"><span class="ann-k">{{$k}}</span><span class="ann-v">{{$v}}</span></div>
        {{end}}
      </div>
      {{end}}
      <a class="thumb-link" href="#section-{{$i}}">↓ Jump to details</a>
    </div>
    {{end}}
  </div>

  <!-- DETAIL SECTIONS (one per test) -->
  {{range $i, $r := .Reports}}
  {{$ov := overall $r}}
  <div class="section {{statusClass $ov}}" id="section-{{$i}}">
    <div class="section-header" onclick="toggleSection({{$i}})">
      <div class="section-title">
        <span class="badge {{statusClass $ov}}">{{statusLabel $ov}}</span>
        {{$r.Name}}
        <span style="font-size:12px;color:var(--muted);font-weight:400;">
          {{$r.Summary.Total}} items &nbsp;·&nbsp; {{$r.Summary.PassCount}} pass &nbsp;·&nbsp; {{$r.Summary.FailCount}} fail &nbsp;·&nbsp; {{$r.Summary.WallTimeMs}} ms
        </span>
      </div>
      <span class="section-chevron {{if hasFails $r}}open{{end}}" id="chev-{{$i}}">▼</span>
    </div>

    <div class="section-body {{if hasFails $r}}open{{end}}" id="body-{{$i}}">

      <!-- ANNOTATIONS -->
      {{if $r.Annotations}}
      <div class="annotations">
        <h4>Annotations</h4>
        <div class="ann-grid">
          {{range $k,$v := $r.Annotations}}
          <div class="ann-chip"><span class="ann-k">{{$k}}</span><span class="ann-v">{{$v}}</span></div>
          {{end}}
        </div>
      </div>
      {{end}}

      <!-- LATENCY HISTOGRAM -->
      {{if gt $r.Summary.Latency.Count 0}}
      <div class="histogram">
        <h4>Latency Distribution</h4>
        {{range $r.Summary.Latency.Histogram}}
        <div class="hist-row">
          <div class="hist-label">{{.Label}}</div>
          <div class="hist-bar-wrap"><div class="hist-bar" style="width:{{.BarWidth}}%"></div></div>
          <div class="hist-count">{{.Count}}</div>
        </div>
        {{end}}
      </div>
      <div class="lat-row">
        <div class="lat-pill"><span class="lp-label">min</span><span class="lp-val">{{$r.Summary.Latency.MinMs}} ms</span></div>
        <div class="lat-pill"><span class="lp-label">avg</span><span class="lp-val">{{$r.Summary.Latency.AvgMs}} ms</span></div>
        <div class="lat-pill"><span class="lp-label">p50</span><span class="lp-val">{{$r.Summary.Latency.P50Ms}} ms</span></div>
        <div class="lat-pill"><span class="lp-label">p95</span><span class="lp-val">{{$r.Summary.Latency.P95Ms}} ms</span></div>
        <div class="lat-pill"><span class="lp-label">p99</span><span class="lp-val">{{$r.Summary.Latency.P99Ms}} ms</span></div>
        <div class="lat-pill"><span class="lp-label">max</span><span class="lp-val">{{$r.Summary.Latency.MaxMs}} ms</span></div>
      </div>
      {{end}}

      <!-- RESULT TABLE -->
      {{if $r.Rows}}
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>#</th>
              <th>Label</th>
              {{range (index $r.Rows 0).Columns}}<th>{{.Header}}</th>{{end}}
              <th>Latency</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            {{range $r.Rows}}
            {{$rid := printf "r%d-%d" $i .Index}}
            <tr onclick="toggleRow('{{$rid}}')" id="row-{{$rid}}"
              {{if eq .Status "fail"}}style="background:rgba(239,68,68,.05)"{{end}}>
              <td class="mono">{{.Index}}</td>
              <td class="mono">
                {{if hasDetails .}}<span class="expander" id="exp-{{$rid}}">▶</span>{{end}}
                {{.Label}}
              </td>
              {{range .Columns}}<td {{if .Mono}}class="mono"{{end}}>{{.Value}}</td>{{end}}
              <td class="mono">{{.LatencyMs}} ms</td>
              <td><span class="badge {{statusClass .Status}}">{{statusLabel .Status}}</span></td>
            </tr>
            {{if hasDetails .}}
            <tr class="detail-row {{if eq .Status "fail"}}open{{end}}" id="detail-{{$rid}}">
              <td colspan="100">
                <div class="detail-box">
                  <h5>Details — {{.Label}}</h5>
                  <div class="detail-grid">
                    {{range .Details}}
                    <div class="dk">{{.Key}}</div>
                    <div class="dv {{if .Mono}}mono{{end}}">{{.Value}}</div>
                    {{end}}
                  </div>
                </div>
              </td>
            </tr>
            {{end}}
            {{end}}
          </tbody>
        </table>
      </div>
      {{end}}

    </div>
  </div>
  {{end}}

  <div class="footer">
    Generated by code-go-automation &nbsp;·&nbsp; {{.RunAt.Format "2006-01-02 15:04:05"}}
    {{with (index .Reports 0)}}{{if .Meta.GitCommit}}&nbsp;·&nbsp; commit {{.Meta.GitCommit}}{{end}}{{end}}
  </div>
</div>

<script>
  function toggleSection(i) {
    const body = document.getElementById('body-' + i);
    const chev = document.getElementById('chev-' + i);
    const open = body.classList.toggle('open');
    chev.classList.toggle('open', open);
  }
  function toggleRow(id) {
    const detail = document.getElementById('detail-' + id);
    const row    = document.getElementById('row-'    + id);
    const exp    = document.getElementById('exp-'    + id);
    if (!detail) return;
    const open = detail.classList.toggle('open');
    row.classList.toggle('expanded', open);
    if (exp) exp.textContent = open ? '▼' : '▶';
  }
</script>
</body>
</html>`
