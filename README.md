# code-go-automation

Go-based API test automation suite for the FanCraze pack/NFT platform. Tests authenticate multiple users in parallel, exercise pack buying and revealing flows, validate wallet deductions, and assert supply integrity invariants — producing timestamped JSON and combined HTML reports for every run.

---

## Table of Contents

- [Project Structure](#project-structure)
- [Prerequisites](#prerequisites)
- [Configuration](#configuration)
- [Test Data](#test-data)
- [Running Tests](#running-tests)
- [Test Suites](#test-suites)
- [Reports](#reports)
- [Architecture](#architecture)
- [Adding a New Test](#adding-a-new-test)

---

## Project Structure

```
code-go-automation/
├── config/
│   └── config.go                  # Env config loaded from .env
├── constants/
│   └── endpoints.go               # All API endpoint path constants
├── data/
│   ├── users.json                 # Test user credentials
│   ├── config_buy_reveal.json     # Runtime config for pack buy+reveal test
│   └── config_supply.json         # Runtime config for supply integrity test
├── handlers/
│   ├── auth.go                    # OTP login + SSO token generation
│   ├── buy_pack.go                # Buy packs + reveal NFTs
│   ├── fetch_enriched.go          # Resolve pack config from enriched API
│   ├── fetch_packs.go             # Fetch unrevealed pack IDs (paginated)
│   ├── supply.go                  # Fetch event groups + supply breakdowns
│   └── wallet.go                  # Fetch user wallet balances
├── reporter/
│   ├── row.go                     # Row interface, Column, Detail, FailureDetail, Status
│   ├── meta.go                    # RunMeta: git info, env, tags, timestamps
│   ├── stats.go                   # Latency stats: p50/p95/p99, histogram
│   ├── runner.go                  # Generic Runner[R Row]: Add, Annotate, Finish, WriteJSON
│   ├── writers.go                 # writeJSON + writeHTML (universal template)
│   └── suite.go                   # WriteRunHTML: combined run report across all tests
├── testconfig/
│   └── loader.go                  # Generic JSON config file loader
├── testutil/
│   └── suite.go                   # BaseSuite: shared lifecycle, WriteReport, LogSummary
├── tests/
│   ├── pack/
│   │   └── pack_test.go           # PackSuite: TestReveal + TestBuyAndReveal
│   └── supply/
│       └── supply_test.go         # SupplySuite: TestSupplyIntegrity
├── reports/                       # Generated — JSON + HTML reports per run
├── .env                           # Environment variables (not committed)
├── Makefile                       # Test runner shortcuts
└── go.mod
```

---

## Prerequisites

- Go 1.25+
- A `.env` file at the project root (see [Configuration](#configuration))

---

## Configuration

### `.env`

```env
ENV=preprod

SPINNER_BFF_BASE_URL=https://spinnerbff.%s.munna-bhai.xyz
FC_BFF_BASE_URL=https://bff.%s.munna-bhai.xyz
FRONTEND_BASE_URL=https://frontend.%s.munna-bhai.xyz
PROXY_URL=https://proxy.%s.munna-bhai.xyz/proxy
```

`%s` is replaced with `ENV` at load time. Switch to prod:

```env
ENV=prod
SPINNER_BFF_BASE_URL=https://app-api.fancraze.com
FC_BFF_BASE_URL=https://apis.fancraze.com
FRONTEND_BASE_URL=https://fancraze.com
PROXY_URL=https://proxy.fancraze.com/proxy
```

No code change needed — edit `.env` and re-run.

---

## Test Data

### `data/users.json`

Array of test accounts. All suites authenticate these users in parallel before running.

```json
[
  { "email": "user@example.com", "otp": "123456" },
  { "email": "user2@example.com", "otp": "123456" }
]
```

Individual auth failures are logged and skipped. The suite aborts only if **all** users fail.

---

### `data/config_buy_reveal.json`

Runtime config for the pack buy+reveal test. Edit and re-run — no code change needed.

```json
{
  "packMasterID":  "69ce3ade5f131ce16676c7b7",
  "quantity":      2,
  "enableReveal":  false,
  "revealWorkers": 10,
  "usersFile":     "users.json"
}
```

| Field | Type | Description |
|---|---|---|
| `packMasterID` | string | Pack master ID to buy |
| `quantity` | int | Packs to buy per user per run |
| `enableReveal` | bool | Whether to reveal packs after buying |
| `revealWorkers` | int | Concurrent goroutines in the reveal worker pool |
| `usersFile` | string | Filename inside `data/` to load users from |

---

### `data/config_supply.json`

Runtime config for the supply integrity test.

```json
{
  "eventGroupsPage":  1,
  "eventGroupsLimit": 200,
  "tolerancePercent": 0.00
}
```

| Field | Type | Description |
|---|---|---|
| `eventGroupsPage` | int | Page to fetch from the event groups API |
| `eventGroupsLimit` | int | Event groups per page |
| `tolerancePercent` | float64 | Allowed overage fraction (0 = strict, 0.01 = 1%) |

---

## Running Tests

### Using Make (recommended)

```bash
make reveal        # Reveal all unrevealed packs for every user
make buy-reveal    # Buy packs then reveal for every user
make packs         # Run both reveal and buy-reveal
make supply        # Supply integrity check
make all           # Run every test suite
make clean-reports # Delete all generated reports
make build         # Compile check only
make tidy          # go mod tidy + verify
```

### Using go test directly

```bash
# All suites
go test ./tests/... -v -count=1

# Pack suite — both tests
go test ./tests/pack/... -v -count=1

# Specific tests
go test ./tests/pack/...   -v -run TestPackSuite/TestReveal          -count=1
go test ./tests/pack/...   -v -run TestPackSuite/TestBuyAndReveal     -count=1
go test ./tests/supply/... -v -run TestSupplySuite/TestSupplyIntegrity -count=1
```

---

## Test Suites

### BaseSuite (`testutil/suite.go`)

Shared foundation embedded by every suite. Provides:

| Method | When | Purpose |
|---|---|---|
| `SetupSuite()` | Once before all tests | Loads `.env` → `*config.Config` |
| `SetupTest()` | Before each test | Logs test name |
| `TearDownTest()` | After each test | Logs test name |
| `TearDownSuite()` | Once after all tests | Writes combined `run_<ts>.html` |
| `WriteReport(rep)` | End of each Test* | Writes per-test JSON to `reports/` |
| `LogSummary(rep)` | End of each Test* | Logs total/pass/fail/latency to test output |
| `StoreSuiteReport(rep)` | End of each Test* | Queues report for the combined HTML |

---

### PackSuite (`tests/pack/pack_test.go`)

Config: `data/config_buy_reveal.json`  
Auth: all users from `usersFile`, authenticated once in `SetupSuite`

#### `TestReveal`

Fetches every user's unrevealed packs and reveals them via a bounded worker pool.

**Flow:**
1. For each user → fetch all unrevealed pack IDs (paginated)
2. Feed all `(token, packID)` pairs into a buffered job channel
3. `revealWorkers` concurrent goroutines pull and call reveal API
4. Collect results → build report

**Skips** if no unrevealed packs exist for any user.

#### `TestBuyAndReveal`

For each user: check wallet balance → buy packs → verify wallet deduction → optionally reveal.

**Flow:**
1. Resolve `priceConfigId` dynamically from enriched API (uses `SOURCE: WEB` header)
2. For each user in parallel:
   - Fetch wallet balance (`/v1/userWallet`)
   - **Skip** user if `unlockedBalance < price × quantity`
   - Buy packs
   - Fetch wallet after buy → assert deduction within 0.0001 tolerance
   - Reveal all returned pack IDs concurrently (if `enableReveal: true`)
3. Collect results → build report

**Row statuses:**
- `PASS` — buy succeeded + wallet deduction matched
- `SKIP` — insufficient balance
- `WARN` — buy succeeded but wallet deduction didn't match expected
- `FAIL` — buy or reveal API error

---

### SupplySuite (`tests/supply/supply_test.go`)

Config: `data/config_supply.json`  
Auth: first user in `users.json` (supply API only needs one token)

#### `TestSupplyIntegrity`

Validates that no event group's allocated supply exceeds its maximum.

**Invariant:**
```
floatingSupply + lockedSupply + packsReserve + lpReserve + availableSupply <= maxSupply
```

**Flow:**
1. Fetch event groups from Spinner BFF (paginated)
2. Batch-fetch supply breakdowns from Proxy
3. Assert invariant per event group (with tolerance)
4. Build report — table label is `slug`, details show `eventGroupID`

Stores `s.EventGroups` and `s.Breakdowns` for downstream dependent tests.

---

## Reports

Every run produces:

| File | Contents |
|---|---|
| `<test-name>_<timestamp>.json` | Full JSON: metadata, summary, all rows with details |
| `run_<timestamp>.html` | Combined dark-theme HTML with all tests for that run |

### HTML report sections

- **Run header** — env, git branch, commit, timestamp, overall pass/fail badge
- **Overview cards** — total tests, total items, total failures
- **Per-test summary strip** — pass rate, p95 latency, wall time, annotations
- **Detail sections** (collapsible per test):
  - Annotations (key-value run context)
  - Latency histogram (5 buckets) + p50/p95/p99/min/max pills
  - Result table with expandable rows
  - **Failed rows auto-expand** on page load
  - Each expanded row shows: request URL, body, response body, error chain, timestamps

---

## Architecture

```
.env ──► config.Load() ──► Config
                               │
data/users.json                │
 └─ LoadUsers()                │
      └─ GenerateAllTokens() ──► []UserToken (parallel)
                                      │
                    ┌─────────────────┤
                    ▼                 ▼
             PackSuite           SupplySuite
          SetupSuite()          SetupSuite()
          loads config_         loads config_
          buy_reveal.json       supply.json
                    │
       ┌────────────┴────────────┐
       ▼                         ▼
  TestReveal              TestBuyAndReveal
       │                         │
  FetchUnrevealedPackIDs   FetchWallet (before)
       │                         │
  worker pool              FetchPackInfo (enriched)
       │                         │
  RevealPack()             BuyPack()
       │                         │
  reporter.Runner          FetchWallet (after)
       │                         │
  WriteReport()            RevealPack() ×N
  StoreSuiteReport()       reporter.Runner
                           WriteReport()
                           StoreSuiteReport()

TearDownSuite() ──► reporter.WriteRunHTML() ──► run_<ts>.html
```

### Key Design Decisions

| Decision | Rationale |
|---|---|
| `testutil.BaseSuite` as shared package | Suites in subdirectories can import it — not limited to one flat package |
| JSON config files per test domain | Change params without touching code or committing |
| Generic `Runner[R Row]` | Adding a new test only requires defining a struct — all reporting is automatic |
| Wallet check before buy | Prevents wasting API calls on users who will fail finance validation |
| `SOURCE: WEB` header on enriched API | Required by backend to return correct priceConfig per pack |
| `SupplyBreakdown.Total()` method | Single source of truth for the supply invariant |
| Combined `run_<ts>.html` | One file per run regardless of how many tests ran — easy to share |

---

## Adding a New Test

### 1. Create the directory

```bash
mkdir tests/wallet
```

### 2. Create the test file

```go
// tests/wallet/wallet_test.go
package wallet_test

import (
    "testing"
    "time"
    "fmt"

    "github.com/AvinGupta27/code-go-automation/config"
    "github.com/AvinGupta27/code-go-automation/reporter"
    "github.com/AvinGupta27/code-go-automation/testutil"
)

// Row type — implement all 5 methods
type walletRow struct { /* your fields */ }
func (r walletRow) RowStatus()  reporter.Status        { /* ... */ }
func (r walletRow) RowLatency() time.Duration          { /* ... */ }
func (r walletRow) RowLabel()   string                 { /* ... */ }
func (r walletRow) RowColumns() []reporter.Column      { /* ... */ }
func (r walletRow) RowDetails() []reporter.Detail      { /* ... */ }

// Suite
type WalletSuite struct { testutil.BaseSuite }

func TestWalletSuite(t *testing.T) { testutil.Run(t, new(WalletSuite)) }

func (s *WalletSuite) TestWalletBalance() {
    cfg := s.Cfg
    run := reporter.NewRunner[walletRow]("Wallet Balance", reporter.NewMeta(
        cfg.Env, cfg.FcBFFURL, "config_wallet.json", "wallet",
    ))

    // ... test logic ...

    rep := run.Finish()
    s.WriteReport(rep)
    s.StoreSuiteReport(rep)
    s.LogSummary(rep)
}
```

### 3. Add a Makefile target

```makefile
## wallet: Check wallet balances for all users
wallet:
    go test ./tests/wallet/... -v -count=1
```

### 4. Add a runtime config (if needed)

```json
// data/config_wallet.json
{
  "currencyID": "GC12"
}
```

That's the complete setup. `WriteReport`, `LogSummary`, `StoreSuiteReport`, config loading — all inherited from `BaseSuite`.
