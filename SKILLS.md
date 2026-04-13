# SKILLS.md — Team Knowledge Base

Patterns, pitfalls, and decisions accumulated while building this project. Read before adding new tests or touching existing handlers.

---

## Go Patterns Used

### Generic Runner

The reporter uses Go generics (`Runner[R Row]`). Every test defines one struct implementing `Row` — the runner handles everything else.

```go
// Define your row type
type myRow struct { Label string; Pass bool }
func (r myRow) RowStatus()  reporter.Status       { if r.Pass { return reporter.StatusPass }; return reporter.StatusFail }
func (r myRow) RowLatency() time.Duration         { return 0 }
func (r myRow) RowLabel()   string                { return r.Label }
func (r myRow) RowColumns() []reporter.Column     { return []reporter.Column{{Header: "Label", Value: r.Label}} }
func (r myRow) RowDetails() []reporter.Detail     { return nil }

// Use it
run := reporter.NewRunner[myRow]("My Test", reporter.NewMeta(env, url, "config.json", "tag"))
run.Add(myRow{Label: "item-1", Pass: true})
rep := run.Finish()
```

### testify suite lifecycle

```
TestMyDomainSuite(t)         ← entry point, called by `go test`
  └─ suite.Run(t, s)
       ├─ SetupSuite()        ← once, before all Test* methods
       ├─ SetupTest()         ← before each Test* method
       │    Test*()           ← your test logic
       ├─ TearDownTest()      ← after each Test* method
       └─ TearDownSuite()     ← once, after all Test* methods
```

`BaseSuite.SetupSuite()` must always be called first when overriding:
```go
func (s *MySuite) SetupSuite() {
    s.BaseSuite.SetupSuite() // ← always first
    // domain-specific setup
}
```

### Parallel workers with channels

Standard pattern for the worker pool used in `TestReveal`:

```go
jobCh := make(chan job, len(allJobs))    // buffered — fill before starting workers
rowCh := make(chan result, len(allJobs)) // buffered — workers write without blocking
var wg sync.WaitGroup

for w := 0; w < workerCount; w++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        for j := range jobCh { // blocks until jobCh is closed
            rowCh <- doWork(j)
        }
    }()
}

for _, j := range allJobs { jobCh <- j }
close(jobCh) // signals workers to stop

go func() { wg.Wait(); close(rowCh) }() // close result channel after all workers finish

for r := range rowCh { /* collect */ }  // drain results
```

### Goroutine closure variable capture

Always pass loop variables as goroutine parameters — never capture directly:

```go
// WRONG — all goroutines capture the same `id` variable
for _, id := range ids {
    go func() { doWork(id) }()
}

// CORRECT — each goroutine gets its own copy
for _, id := range ids {
    go func(pid string) { doWork(pid) }(id)
}
```

### Safe slice append

When appending one slice to another, use an explicit allocation to avoid mutating the backing array:

```go
// RISKY — may mutate data.HeroSection backing array
all := append(data.HeroSection, data.Packs...)

// SAFE
all := make([]enrichedPack, 0, len(data.HeroSection)+len(data.Packs))
all = append(all, data.HeroSection...)
all = append(all, data.Packs...)
```

---

## API Quirks

### Enriched packs API (`/api/v2/packs/enriched`)

- Requires `SOURCE: WEB` header — without it, returns `GC1` instead of `GC12` as the pack currency
- Requires `countryCode=IN` query param
- Returns packs in both `heroSection` and `packs` arrays — deduplicate by `_id`
- `priceConfig` is a nested map: `{ "6": { "GC12": 0.45 } }` — sort keys for determinism when selecting

### User wallet API (`/v1/userWallet`)

- Use `x_auth_token` header (not `Authorization`)
- `value` field includes locked balance — always use `unlocked` for buy validation
- `appMultiplier` field exists per currency but raw values in the API are already in the correct unit for comparison with `priceConfig` prices

### Supply API (`/superteamUserService/api/v1/events/available-supply`)

- Send event group IDs as `eventIds` array in POST body
- Returns map keyed by event group ID — order is not guaranteed

### User packs list (`/api/v1/userpacks/`)

- Returns either `{ data: [{userPackId, status}] }` or `{ data: { userPacks: [...] } }` — handle both
- Filter by `status=UNREVEALED` to get unrevealed packs
- Paginate with `page` and `limit` params; stop when response count < limit

### OTP auth flow

Three steps — all must succeed in sequence:
1. POST `/auth/otp/login` with `{ email }` → triggers OTP send
2. POST `/auth/otp/verify` with `{ email, otp }` → returns `access_token`
3. POST `/auth/sso/generate` with `access_token` header → returns `ssoToken`

---

## Common Mistakes

### Forgetting `storeSuiteReport`

If you call `WriteReport` but not `StoreSuiteReport`, the test's data won't appear in the combined `run_<ts>.html`.

Always call all three:
```go
s.WriteReport(rep)
s.StoreSuiteReport(rep)
s.LogSummary(rep)
```

### Using `s.T().Errorf` in `SetupSuite`

`Errorf` marks the test as failed but continues execution. In `SetupSuite` this causes confusing failures. Use `s.Require().NoError()` for hard failures and `s.T().Logf()` for soft warnings.

### Hardcoding values in test files

Use `data/config_<domain>.json` + `testconfig.Load()`. Hardcoded values in `.go` files require a commit to change.

### Map iteration order in Go

Go map iteration is randomised. Never rely on it for deterministic behaviour:
```go
// Non-deterministic — may pick different currency each run
for cur, val := range priceConfig { currency = cur; break }

// Deterministic — sort keys first
keys := sortedKeys(priceConfig)
currency = keys[0]
```

### `go test -v` vs debugger flags

`go test -v` works from the CLI. The VS Code debugger requires `go test` binary flags:
```
CLI:       -v -count=1 -run TestName
Debugger:  -test.v -test.count=1 -test.run TestName
```

---

## Decisions Log

| Decision | Why |
|---|---|
| `testutil` as a separate package | Allows `tests/<domain>` subdirectories (different packages) to import `BaseSuite` |
| JSON config files per test | Change pack IDs, quantities, tolerances without touching code or committing |
| `Runner[R Row]` generic | Adding test 50 is the same effort as test 1 — define a struct, nothing else |
| Single `run_<ts>.html` | One file to share for review regardless of how many tests ran |
| Wallet check before buy | Prevents wasting API quota on users who will fail finance validation |
| 0.0001 wallet tolerance | Accounts for floating-point precision — `0.45 * 2 = 0.8999999...` in float64 |
| `slug` as supply row label | Human-readable in the report table; `eventGroupID` available in expandable details |
| `storeSuiteReport` lowercase → `StoreSuiteReport` exported | Required when method is called from test code in a different package |

---

## File Ownership

| Area | Files | Owner concern |
|---|---|---|
| API clients | `handlers/*.go` | One file per service domain |
| Test logic | `tests/<domain>/*.go` | One subdirectory per feature area |
| Shared test infra | `testutil/suite.go` | Touch rarely — breaking this breaks everything |
| Report rendering | `reporter/writers.go`, `reporter/suite.go` | HTML template lives here |
| Report data model | `reporter/row.go`, `reporter/runner.go` | Row interface contract |
| Runtime config | `data/config_*.json` | Edit freely — no code impact |
| Environment config | `.env` | Never commit |
