# CLAUDE.md — Project Instructions for Claude Code

This file tells Claude Code how to work in this repository. Read it before making any changes.

---

## Project Summary

Go-based API test automation suite for the FanCraze NFT pack platform. Tests are organised by domain in `tests/<domain>/`. All shared infrastructure lives in `testutil/`, `handlers/`, and `reporter/`.

---

## Tech Stack

- **Language:** Go 1.25
- **HTTP client:** `github.com/go-resty/resty/v2`
- **Test framework:** `github.com/stretchr/testify/suite`
- **Config:** `.env` via `godotenv`, JSON test configs via `testconfig/loader.go`

---

## Repository Layout (critical paths)

```
testutil/suite.go          ← BaseSuite — embed this in every new test suite
handlers/                  ← One file per API domain (auth, wallet, supply, etc.)
constants/endpoints.go     ← All API endpoint paths — add new ones here
reporter/                  ← Generic Runner[R Row] — touch only to add Row types
testconfig/loader.go       ← Generic JSON loader — do not modify
data/                      ← Runtime JSON configs — edit these, not code
tests/<domain>/            ← One subdirectory per test domain
reports/                   ← Generated output — never commit
```

---

## Rules

### Always

- Add new API endpoints to `constants/endpoints.go` first, then reference the constant
- Add new test domains as `tests/<domain>/<domain>_test.go` with `package <domain>_test`
- Embed `testutil.BaseSuite` — never duplicate `SetupSuite`, `WriteReport`, `LogSummary`, `StoreSuiteReport`
- Use `testconfig.Load(config.DataPath("config_<domain>.json"), &s.cfg)` for any configurable payload
- Call these three at the end of every `Test*` method, in this order:
  ```go
  s.WriteReport(rep)
  s.StoreSuiteReport(rep)
  s.LogSummary(rep)
  ```
- Keep `handlers/` files focused on one API domain each
- Capture `FailureDetail{RequestMethod, RequestURL, RequestBody, ResponseStatus, ResponseBody, ErrorChain, OccurredAt}` for every failed API call

### Never

- Hardcode environment URLs, pack IDs, quantities, or OTPs in Go source files
- Write HTML, JSON marshalling, or latency calculations in test files — use `reporter.Runner`
- Duplicate `writeReport` or `logSummary` in a new suite — they live on `BaseSuite`
- Commit to `reports/` directory
- Add debug `fmt.Printf` / `fmt.Println` statements without removing them before committing
- Use `append(slice1, slice2...)` where `slice1` may have excess capacity — always allocate explicitly

---

## Adding a New Test Domain

```bash
mkdir tests/<domain>
```

```go
// tests/<domain>/<domain>_test.go
package <domain>_test

import (
    "testing"
    "github.com/AvinGupta27/code-go-automation/testutil"
)

type MyDomainSuite struct { testutil.BaseSuite }
func TestMyDomainSuite(t *testing.T) { testutil.Run(t, new(MyDomainSuite)) }

func (s *MyDomainSuite) TestSomething() {
    // ... test logic using reporter.NewRunner[myRow] ...
    rep := run.Finish()
    s.WriteReport(rep)
    s.StoreSuiteReport(rep)
    s.LogSummary(rep)
}
```

Add a Makefile target:
```makefile
## <domain>: Description of what this test does
<domain>:
    go test ./tests/<domain>/... -v -count=1
```

Add a runtime config if needed:
```json
// data/config_<domain>.json
{ "field": "value" }
```

---

## Adding a New Handler

1. Create `handlers/<domain>.go`
2. Add endpoint constants to `constants/endpoints.go`
3. Use `resty.New().SetTimeout(Xs)` — always set a timeout
4. Return a typed result struct, never `(interface{}, error)`
5. Set `result.Status = resp.StatusCode()` on both success and failure paths

---

## Adding a New Row Type

A row type is a Go struct that implements the `reporter.Row` interface:

```go
type myRow struct { /* fields */ }

func (r myRow) RowStatus()  reporter.Status       { /* pass/fail/warn/skip */ }
func (r myRow) RowLatency() time.Duration         { /* 0 if not applicable */ }
func (r myRow) RowLabel()   string                { /* primary identifier */ }
func (r myRow) RowColumns() []reporter.Column     { /* table columns */ }
func (r myRow) RowDetails() []reporter.Detail     { /* expandable details */ }
```

The `reporter.Runner[myRow]` handles everything else automatically.

---

## Environment

```bash
# Switch to prod: edit .env only
ENV=prod
SPINNER_BFF_BASE_URL=https://app-api.fancraze.com
FC_BFF_BASE_URL=https://apis.fancraze.com
PROXY_URL=https://proxy.fancraze.com/proxy
```

---

## Run Commands

```bash
make reveal        # Reveal unrevealed packs
make buy-reveal    # Buy + reveal packs
make packs         # Both reveal and buy-reveal
make supply        # Supply integrity
make all           # Everything
make build         # Compile only
make clean-reports # Delete reports/
```

---

## Known Behaviours

- `FetchPackInfo` uses `SOURCE: WEB` header — required for correct `priceConfig` from enriched API
- Wallet deduction check uses 0.0001 tolerance for floating-point precision
- Supply test uses `slug` as row label, `eventGroupID` in expandable details
- `data/config_supply.json` `tolerancePercent: 0.00` means strict mode — any overage fails
- Auth failures for individual users log as `Auth SKIPPED` — suite only aborts if ALL users fail
- `config_buy_reveal.json` `enableReveal: false` means packs are bought but not revealed
