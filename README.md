# code-go-automation

Go-based API test automation suite for the FanCraze pack/NFT platform. Tests authenticate multiple users in parallel, exercise pack buying and revealing flows, and assert supply integrity invariants — producing timestamped JSON and HTML reports for every run.

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
│   └── config.go            # Env config loaded from .env
├── constants/
│   └── endpoints.go         # API endpoint path constants
├── data/
│   ├── users.json           # Test user credentials
│   └── config_supply.json   # Runtime config for supply test
├── handlers/
│   ├── auth.go              # OTP login + SSO token generation
│   ├── buy_pack.go          # Buy packs + reveal NFTs
│   ├── fetch_packs.go       # Fetch unrevealed pack IDs
│   ├── fetch_enriched.go    # Resolve pack config at runtime
│   └── supply.go            # Fetch event groups + supply breakdowns
├── reporter/
│   └── reporter.go          # JSON + HTML report generation
├── testconfig/
│   └── loader.go            # Generic JSON config loader
├── tests/
│   ├── suite_test.go        # BaseSuite: config load + lifecycle hooks
│   ├── pack_test.go         # PackSuite: reveal + buy-and-reveal tests
│   └── supply_test.go       # SupplySuite: supply integrity test
├── reports/                 # Generated — JSON + HTML reports per run
├── .env                     # Environment variables (not committed)
└── go.mod
```

---

## Prerequisites

- Go 1.25+
- A `.env` file at the project root (see [Configuration](#configuration))

---

## Configuration

Create a `.env` file at the project root:

```env
ENV=preprod

SPINNER_BFF_BASE_URL=https://spinnerbff.%s.munna-bhai.xyz
FC_BFF_BASE_URL=https://bff.%s.munna-bhai.xyz
FRONTEND_BASE_URL=https://frontend.%s.munna-bhai.xyz
PROXY_URL=https://proxy.%s.munna-bhai.xyz/proxy
```

`%s` is replaced with the value of `ENV` at load time. To target `prod`:

```env
ENV=prod
SPINNER_BFF_BASE_URL=https://app-api.fancraze.com
FC_BFF_BASE_URL=https://apis.fancraze.com
FRONTEND_BASE_URL=https://fancraze.com
PROXY_URL=https://proxy.fancraze.com/proxy
```

No code change or commit is needed to switch environments — edit `.env` and re-run.

---

## Test Data

### `data/users.json`

Array of test accounts. All suites authenticate these users in parallel before running tests.

```json
[
  { "email": "user@example.com", "otp": "123456" },
  { "email": "user2@example.com", "otp": "123456" }
]
```

Authentication failures for individual users are logged as warnings and skipped — they do not fail the suite. The suite only fails if **all** users fail authentication.

### `data/config_supply.json`

Runtime configuration for the supply integrity test. Edit this file and re-run — no code change needed.

```json
{
  "eventGroupsPage": 1,
  "eventGroupsLimit": 200,
  "tolerancePercent": 0.01
}
```

| Field | Type | Description |
|---|---|---|
| `eventGroupsPage` | int | Page number to fetch from the event groups API |
| `eventGroupsLimit` | int | Number of event groups to fetch per page |
| `tolerancePercent` | float64 | Allowed overage as a fraction (0.01 = 1%, 0 = strict) |

---

## Running Tests

```bash
# All suites
go test ./tests/... -v

# Pack suite only (both reveal and buy+reveal)
go test ./tests/... -v -run TestPackSuite

# Reveal only (no buy)
go test ./tests/... -v -run TestPackSuite/TestReveal

# Buy and reveal
go test ./tests/... -v -run TestPackSuite/TestBuyAndReveal

# Supply integrity only
go test ./tests/... -v -run TestSupplySuite/TestSupplyIntegrity
```

---

## Test Suites

### BaseSuite (`tests/suite_test.go`)

Shared foundation embedded by every suite. Runs once per suite:

| Hook | When | What it does |
|---|---|---|
| `SetupSuite` | Before all tests | Loads `.env` → `*config.Config` |
| `SetupTest` | Before each test | Logs test name |
| `TearDownTest` | After each test | Logs test name |
| `TearDownSuite` | After all tests | Logs completion |

---

### PackSuite (`tests/pack_test.go`)

Authenticates all users in `SetupSuite` (once, shared by both tests).

#### `TestReveal`

For every authenticated user, fetches their unrevealed packs and reveals them via a shared worker pool.

**Flow:**
1. Fetch unrevealed pack IDs for each user (`UNREVEALED` status, paginated)
2. Feed all `(token, packID)` pairs into a channel
3. 5 concurrent workers pull from the channel and call the reveal API
4. Aggregate results → generate report

**Configurable via constants in `pack_test.go`:**

| Constant | Default | Description |
|---|---|---|
| `revealWorkers` | `5` | Number of concurrent reveal goroutines |

#### `TestBuyAndReveal`

For every authenticated user, buys N packs then reveals all returned pack IDs.

**Flow:**
1. Resolve `priceConfigId` dynamically from the enriched API (one call)
2. Launch one goroutine per user
3. Each goroutine: `BuyPack` → wait 500ms → reveal all returned pack IDs concurrently
4. Aggregate NFT counts, values, latencies → generate report

**Configurable via constants in `pack_test.go`:**

| Constant | Default | Description |
|---|---|---|
| `buyPackMasterID` | `"69c378ce93725fbf11f5c183"` | Pack master ID to buy |
| `buyQuantity` | `10` | Number of packs to buy per user |
| `enableReveal` | `true` | Whether to reveal after buying |

---

### SupplySuite (`tests/supply_test.go`)

#### `TestSupplyIntegrity`

Validates the core supply invariant: no event group's allocated components may exceed its `maxSupply`.

**Invariant:**
```
floatingSupply + lockedSupply + packsReserve + lpReserve + availableSupply <= maxSupply
```

**Flow:**
1. Authenticate one user (first in `users.json`)
2. Fetch all event groups from Spinner BFF (`/api/v1/eventGroups/findAll`)
3. Fetch supply breakdowns for all IDs from Proxy in one request
4. Assert invariant for each event group (with tolerance from config)
5. Generate supply report

**Configurable via `data/config_supply.json`** (no code change needed).

---

## Reports

Every test run writes two files to `reports/`:

| Suite | JSON | HTML |
|---|---|---|
| `TestReveal` | `reveal_<timestamp>.json` | `reveal_<timestamp>.html` |
| `TestBuyAndReveal` | `buy_reveal_<timestamp>.json` | `buy_reveal_<timestamp>.html` |
| `TestSupplyIntegrity` | `supply_<timestamp>.json` | `supply_<timestamp>.html` |

HTML reports are self-contained (no external dependencies) and include:
- Summary metric cards (totals, success rate, latency, wall time)
- Per-item result table with pass/fail badges
- Dark theme with monospace IDs for readability

---

## Architecture

```
.env
 └─ config.Load() ──────────────────────────────────► Config
                                                         │
data/users.json                                          │
 └─ LoadUsers()                                          │
      └─ GenerateAllTokens() ──► []UserToken             │
                                      │                  │
                          ┌───────────┘                  │
                          ▼                              ▼
              PackSuite.SetupSuite()        SupplySuite.SetupSuite()
                          │                              │
          ┌───────────────┴────────────┐    testconfig.Load(config_supply.json)
          ▼                            ▼
    TestReveal               TestBuyAndReveal
          │                            │
  FetchUnrevealedPackIDs()     FetchPackInfo()
          │                            │
  worker pool (N goroutines)   per-user goroutine
          │                            │
    RevealPack()           BuyPack() → RevealPack()
          │                            │
   reporter.Report          reporter.BuyRevealReport
          │                            │
   reports/reveal_*.{json,html}  reports/buy_reveal_*.{json,html}

              TestSupplyIntegrity
                      │
           FetchEventGroups()  (Spinner BFF)
                      │
           FetchSupplyBreakdowns()  (Proxy)
                      │
             assert Total() <= MaxSupply
                      │
           reporter.SupplyReport
                      │
           reports/supply_*.{json,html}
```

### Key Design Decisions

| Decision | Rationale |
|---|---|
| Auth runs once in `SetupSuite` | Tokens are shared across all tests in a suite — no redundant logins |
| `testconfig.Load()` for payloads | Change request params by editing JSON, no recompile or commit needed |
| Worker pool for reveals | Limits concurrency without spawning one goroutine per pack |
| Per-user goroutines for buy+reveal | Each user's buy+reveal is independent; inner parallelism handles multiple packs |
| `SupplyBreakdown.Total()` method | Single source of truth for the invariant being tested |
| `tolerancePercent` in supply config | Allows tuning strictness per environment without code change |

---

## Adding a New Test

1. Create `tests/<domain>_test.go`
2. Define a suite embedding `BaseSuite`:
   ```go
   type MyDomainSuite struct {
       BaseSuite
   }
   func TestMyDomainSuite(t *testing.T) { RunSuite(t, new(MyDomainSuite)) }
   ```
3. Override `SetupSuite` if you need domain-specific setup (call `s.BaseSuite.SetupSuite()` first)
4. Add `Test*` methods for each test case
5. If the test needs a configurable payload: add `data/config_<domain>.json` and load it via `testconfig.Load()`
6. Add a new report type to `reporter/reporter.go` if the output shape differs from existing reporters

Run with:
```bash
go test ./tests/... -v -run TestMyDomainSuite
```
