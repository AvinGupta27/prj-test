package reporter

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
)

// ──────────────────────────────────────────────────────────────────────────────
// JSON WRITER
// ──────────────────────────────────────────────────────────────────────────────

func writeJSON(rep *Report, outDir string) (string, error) {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", fmt.Errorf("reporter: mkdir: %w", err)
	}
	slug := nameSlug(rep.Name)
	filename := fmt.Sprintf("%s_%s.json", slug, rep.Meta.StartedAt.Format("2006-01-02_15-04-05"))
	path := filepath.Join(outDir, filename)

	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return "", fmt.Errorf("reporter: marshal json: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("reporter: write json: %w", err)
	}
	return path, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// HTML WRITER
// ──────────────────────────────────────────────────────────────────────────────

func writeHTML(rep *Report, outDir string) (string, error) {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", fmt.Errorf("reporter: mkdir: %w", err)
	}
	slug := nameSlug(rep.Name)
	filename := fmt.Sprintf("%s_%s.html", slug, rep.Meta.StartedAt.Format("2006-01-02_15-04-05"))
	path := filepath.Join(outDir, filename)

	funcMap := template.FuncMap{
		"statusClass": func(s Status) string {
			switch s {
			case StatusPass:
				return "pass"
			case StatusFail:
				return "fail"
			case StatusWarning:
				return "warn"
			case StatusSkip:
				return "skip"
			default:
				return ""
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
			case StatusSkip:
				return "SKIP"
			default:
				return string(s)
			}
		},
		"hasDetails": func(row RowRecord) bool {
			return len(row.Details) > 0
		},
		"join": strings.Join,
		"printf": fmt.Sprintf,
	}

	tmpl, err := template.New("report").Funcs(funcMap).Parse(universalHTMLTemplate)
	if err != nil {
		return "", fmt.Errorf("reporter: parse template: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("reporter: create html: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, rep); err != nil {
		return "", fmt.Errorf("reporter: render template: %w", err)
	}
	return path, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// HELPERS
// ──────────────────────────────────────────────────────────────────────────────

// nameSlug converts a test name like "Reveal Packs" → "reveal_packs".
func nameSlug(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "/", "_")
	return s
}

// ──────────────────────────────────────────────────────────────────────────────
// UNIVERSAL HTML TEMPLATE
// One template renders every report regardless of row shape.
// Column headers come from the first row's Columns().
// Each row is clickable to expand its Details.
// ──────────────────────────────────────────────────────────────────────────────

var universalHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8"/>
  <meta name="viewport" content="width=device-width,initial-scale=1"/>
  <title>{{.Name}} · {{.Meta.Env}}</title>
  <style>
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
    :root {
      --bg:#0d0f14; --surface:#151820; --surface2:#1a1f2e; --border:#1e2330;
      --text:#e2e8f0; --muted:#64748b; --mono:'JetBrains Mono',monospace;
      --pass:#22c55e; --fail:#ef4444; --warn:#f59e0b; --skip:#64748b; --blue:#3b82f6;
      --purple:#a855f7; --radius:10px;
    }
    body { font-family:'Inter',system-ui,sans-serif; background:var(--bg); color:var(--text); padding:32px 20px; min-height:100vh; }
    a { color:var(--blue); }

    /* ── LAYOUT ── */
    .wrap { max-width:1300px; margin:0 auto; }

    /* ── HEADER ── */
    .header { margin-bottom:28px; }
    .header h1 { font-size:24px; font-weight:700;
      background:linear-gradient(135deg,#60a5fa,#a78bfa);
      -webkit-background-clip:text; -webkit-text-fill-color:transparent; margin-bottom:6px; }
    .header-meta { display:flex; flex-wrap:wrap; gap:10px; font-size:12px; color:var(--muted); margin-top:8px; }
    .header-meta span { background:var(--surface2); border:1px solid var(--border);
      padding:3px 10px; border-radius:999px; }
    .tags { display:flex; gap:6px; flex-wrap:wrap; margin-top:8px; }
    .tag { background:rgba(168,85,247,.15); color:var(--purple); border:1px solid rgba(168,85,247,.3);
      font-size:11px; font-weight:600; padding:2px 10px; border-radius:999px; }

    /* ── SUMMARY CARDS ── */
    .cards { display:grid; grid-template-columns:repeat(auto-fit,minmax(140px,1fr)); gap:14px; margin-bottom:28px; }
    .card { background:var(--surface); border:1px solid var(--border); border-radius:var(--radius); padding:18px 16px; }
    .card-label { font-size:10px; color:var(--muted); text-transform:uppercase; letter-spacing:.8px; margin-bottom:8px; }
    .card-value { font-size:26px; font-weight:700; }
    .c-pass { color:var(--pass); } .c-fail { color:var(--fail); }
    .c-warn { color:var(--warn); } .c-skip { color:var(--skip); }
    .c-blue { color:var(--blue); } .c-purple { color:var(--purple); }

    /* ── ANNOTATIONS ── */
    .annotations { background:var(--surface); border:1px solid var(--border); border-radius:var(--radius);
      padding:16px 20px; margin-bottom:24px; }
    .annotations h3 { font-size:11px; color:var(--muted); text-transform:uppercase; letter-spacing:.8px; margin-bottom:12px; }
    .ann-grid { display:flex; flex-wrap:wrap; gap:8px; }
    .ann-item { background:var(--surface2); border:1px solid var(--border); border-radius:6px;
      padding:4px 12px; font-size:12px; }
    .ann-key { color:var(--muted); margin-right:6px; }
    .ann-val { color:var(--text); font-family:var(--mono); }

    /* ── LATENCY HISTOGRAM ── */
    .histogram { background:var(--surface); border:1px solid var(--border); border-radius:var(--radius);
      padding:18px 20px; margin-bottom:24px; }
    .histogram h3 { font-size:11px; color:var(--muted); text-transform:uppercase; letter-spacing:.8px; margin-bottom:14px; }
    .hist-row { display:flex; align-items:center; gap:12px; margin-bottom:8px; font-size:12px; }
    .hist-label { width:130px; color:var(--muted); flex-shrink:0; }
    .hist-bar-wrap { flex:1; background:var(--surface2); border-radius:4px; height:14px; }
    .hist-bar { height:100%; border-radius:4px; background:linear-gradient(90deg,#3b82f6,#a78bfa); transition:width .3s; }
    .hist-count { width:40px; text-align:right; color:var(--text); font-family:var(--mono); }

    /* ── LATENCY STATS ROW ── */
    .lat-stats { display:flex; flex-wrap:wrap; gap:10px; margin-bottom:24px; }
    .lat-stat { background:var(--surface); border:1px solid var(--border); border-radius:var(--radius);
      padding:10px 16px; font-size:12px; }
    .lat-stat-label { color:var(--muted); margin-right:6px; }
    .lat-stat-val { font-family:var(--mono); color:var(--blue); font-weight:600; }

    /* ── TABLE ── */
    .table-wrap { overflow-x:auto; border-radius:var(--radius); border:1px solid var(--border); margin-bottom:32px; }
    table { width:100%; border-collapse:collapse; font-size:13px; background:var(--surface); }
    thead th { background:var(--surface2); color:var(--muted); font-weight:600;
      text-transform:uppercase; letter-spacing:.5px; font-size:11px;
      padding:11px 14px; text-align:left; white-space:nowrap; }
    tbody tr { border-top:1px solid var(--border); cursor:pointer; transition:background .15s; }
    tbody tr:hover { background:rgba(255,255,255,.025); }
    tbody tr.expanded { background:rgba(59,130,246,.06); }
    td { padding:10px 14px; vertical-align:middle; }
    td.mono { font-family:var(--mono); font-size:11px; word-break:break-all; }

    /* ── BADGE ── */
    .badge { display:inline-flex; align-items:center; gap:5px; padding:2px 10px;
      border-radius:999px; font-size:11px; font-weight:700; letter-spacing:.4px; white-space:nowrap; }
    .badge::before { content:'●'; font-size:8px; }
    .badge.pass { background:rgba(34,197,94,.12); color:var(--pass); border:1px solid rgba(34,197,94,.25); }
    .badge.fail { background:rgba(239,68,68,.12); color:var(--fail); border:1px solid rgba(239,68,68,.25); }
    .badge.warn { background:rgba(245,158,11,.12); color:var(--warn); border:1px solid rgba(245,158,11,.25); }
    .badge.skip { background:rgba(100,116,139,.12); color:var(--skip); border:1px solid rgba(100,116,139,.25); }

    /* ── EXPAND ROW ── */
    .detail-row { display:none; }
    .detail-row.open { display:table-row; }
    .detail-row td { padding:0; border-top:none; }
    .detail-box { background:var(--surface2); border-bottom:1px solid var(--border); padding:16px 20px; }
    .detail-box h4 { font-size:10px; color:var(--muted); text-transform:uppercase; letter-spacing:.8px; margin-bottom:12px; }
    .detail-grid { display:grid; grid-template-columns:160px 1fr; gap:6px 16px; font-size:12px; }
    .detail-key { color:var(--muted); padding-top:2px; }
    .detail-val { color:var(--text); word-break:break-all; }
    .detail-val.mono { font-family:var(--mono); font-size:11px; white-space:pre-wrap; }

    /* ── EXPAND INDICATOR ── */
    .expander { color:var(--muted); font-size:11px; user-select:none; padding-right:4px; }

    /* ── FOOTER ── */
    .footer { text-align:center; color:var(--muted); font-size:11px; margin-top:16px; }
  </style>
</head>
<body>
<div class="wrap">

  <!-- HEADER -->
  <div class="header">
    <h1>{{.Name}}</h1>
    <div class="header-meta">
      <span>env: {{.Meta.Env}}</span>
      {{if .Meta.BaseURL}}<span>{{.Meta.BaseURL}}</span>{{end}}
      {{if .Meta.GitBranch}}<span>branch: {{.Meta.GitBranch}}</span>{{end}}
      {{if .Meta.GitCommit}}<span>commit: {{.Meta.GitCommit}}</span>{{end}}
      {{if .Meta.ConfigFile}}<span>config: {{.Meta.ConfigFile}}</span>{{end}}
      <span>{{.Meta.StartedAt.Format "2006-01-02 15:04:05"}}</span>
      <span>wall: {{.Summary.WallTimeMs}} ms</span>
    </div>
    {{if .Meta.Tags}}
    <div class="tags">
      {{range .Meta.Tags}}<span class="tag">{{.}}</span>{{end}}
    </div>
    {{end}}
  </div>

  <!-- SUMMARY CARDS -->
  <div class="cards">
    <div class="card"><div class="card-label">Total</div><div class="card-value c-blue">{{.Summary.Total}}</div></div>
    <div class="card"><div class="card-label">Pass</div><div class="card-value c-pass">{{.Summary.PassCount}}</div></div>
    <div class="card"><div class="card-label">Fail</div><div class="card-value {{if gt .Summary.FailCount 0}}c-fail{{else}}c-pass{{end}}">{{.Summary.FailCount}}</div></div>
    {{if gt .Summary.WarnCount 0}}<div class="card"><div class="card-label">Warning</div><div class="card-value c-warn">{{.Summary.WarnCount}}</div></div>{{end}}
    {{if gt .Summary.SkipCount 0}}<div class="card"><div class="card-label">Skip</div><div class="card-value c-skip">{{.Summary.SkipCount}}</div></div>{{end}}
    <div class="card"><div class="card-label">Success Rate</div><div class="card-value {{if ge .Summary.SuccessRate 90.0}}c-pass{{else if ge .Summary.SuccessRate 50.0}}c-warn{{else}}c-fail{{end}}">{{printf "%.1f" .Summary.SuccessRate}}%</div></div>
    <div class="card"><div class="card-label">p50</div><div class="card-value c-blue">{{.Summary.Latency.P50Ms}}ms</div></div>
    <div class="card"><div class="card-label">p95</div><div class="card-value c-purple">{{.Summary.Latency.P95Ms}}ms</div></div>
    <div class="card"><div class="card-label">p99</div><div class="card-value c-purple">{{.Summary.Latency.P99Ms}}ms</div></div>
  </div>

  <!-- ANNOTATIONS -->
  {{if .Annotations}}
  <div class="annotations">
    <h3>Run Annotations</h3>
    <div class="ann-grid">
      {{range $k, $v := .Annotations}}
      <div class="ann-item"><span class="ann-key">{{$k}}</span><span class="ann-val">{{$v}}</span></div>
      {{end}}
    </div>
  </div>
  {{end}}

  <!-- LATENCY HISTOGRAM -->
  {{if gt .Summary.Latency.Count 0}}
  <div class="histogram">
    <h3>Latency Distribution</h3>
    {{range .Summary.Latency.Histogram}}
    <div class="hist-row">
      <div class="hist-label">{{.Label}}</div>
      <div class="hist-bar-wrap"><div class="hist-bar" style="width:{{.BarWidth}}%"></div></div>
      <div class="hist-count">{{.Count}}</div>
    </div>
    {{end}}
  </div>

  <!-- LATENCY STATS -->
  <div class="lat-stats">
    <div class="lat-stat"><span class="lat-stat-label">min</span><span class="lat-stat-val">{{.Summary.Latency.MinMs}} ms</span></div>
    <div class="lat-stat"><span class="lat-stat-label">avg</span><span class="lat-stat-val">{{.Summary.Latency.AvgMs}} ms</span></div>
    <div class="lat-stat"><span class="lat-stat-label">p50</span><span class="lat-stat-val">{{.Summary.Latency.P50Ms}} ms</span></div>
    <div class="lat-stat"><span class="lat-stat-label">p95</span><span class="lat-stat-val">{{.Summary.Latency.P95Ms}} ms</span></div>
    <div class="lat-stat"><span class="lat-stat-label">p99</span><span class="lat-stat-val">{{.Summary.Latency.P99Ms}} ms</span></div>
    <div class="lat-stat"><span class="lat-stat-label">max</span><span class="lat-stat-val">{{.Summary.Latency.MaxMs}} ms</span></div>
  </div>
  {{end}}

  <!-- RESULTS TABLE -->
  {{if .Rows}}
  <div class="table-wrap">
    <table id="results-table">
      <thead>
        <tr>
          <th>#</th>
          <th>Label</th>
          {{range (index .Rows 0).Columns}}<th>{{.Header}}</th>{{end}}
          <th>Latency</th>
          <th>Status</th>
        </tr>
      </thead>
      <tbody>
        {{range .Rows}}
        <tr onclick="toggleDetail({{.Index}})" id="row-{{.Index}}">
          <td class="mono">{{.Index}}</td>
          <td class="mono">
            {{if hasDetails .}}<span class="expander" id="exp-{{.Index}}">▶</span>{{end}}
            {{.Label}}
          </td>
          {{range .Columns}}
          <td {{if .Mono}}class="mono"{{end}}>{{.Value}}</td>
          {{end}}
          <td class="mono">{{.LatencyMs}} ms</td>
          <td><span class="badge {{statusClass .Status}}">{{statusLabel .Status}}</span></td>
        </tr>
        {{if hasDetails .}}
        <tr class="detail-row" id="detail-{{.Index}}">
          <td colspan="100">
            <div class="detail-box">
              <h4>Details — {{.Label}}</h4>
              <div class="detail-grid">
                {{range .Details}}
                <div class="detail-key">{{.Key}}</div>
                <div class="detail-val {{if .Mono}}mono{{end}}">{{.Value}}</div>
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

  <div class="footer">
    Generated by code-go-automation &nbsp;·&nbsp; {{.Meta.StartedAt.Format "2006-01-02 15:04:05"}}
    {{if .Meta.GitCommit}}&nbsp;·&nbsp; commit {{.Meta.GitCommit}}{{end}}
  </div>
</div>

<script>
  function toggleDetail(idx) {
    const detail = document.getElementById('detail-' + idx);
    const row    = document.getElementById('row-' + idx);
    const exp    = document.getElementById('exp-' + idx);
    if (!detail) return;
    const open = detail.classList.toggle('open');
    row.classList.toggle('expanded', open);
    if (exp) exp.textContent = open ? '▼' : '▶';
  }
</script>
</body>
</html>`
