package tests

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/AvinGupta27/code-go-automation/config"
	"github.com/AvinGupta27/code-go-automation/handlers"
	"github.com/AvinGupta27/code-go-automation/reporter"
	"github.com/AvinGupta27/code-go-automation/testconfig"
)

// ──────────────────────────────────────────────────────────────────────────────
// CONFIG — edit data/config_supply.json, no code change needed
// ──────────────────────────────────────────────────────────────────────────────

type supplyConfig struct {
	EventGroupsPage  int     `json:"eventGroupsPage"`
	EventGroupsLimit int     `json:"eventGroupsLimit"`
	TolerancePercent float64 `json:"tolerancePercent"` // e.g. 0.01 = 1%, 0 = strict
}

// ──────────────────────────────────────────────────────────────────────────────
// SUITE
// ──────────────────────────────────────────────────────────────────────────────

type SupplySuite struct {
	BaseSuite
	cfg supplyConfig

	// Shared outputs — available to downstream dependent tests.
	EventGroups []handlers.EventGroup
	Breakdowns  []handlers.SupplyBreakdown
}

func TestSupplySuite(t *testing.T) {
	RunSuite(t, new(SupplySuite))
}

func (s *SupplySuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	err := testconfig.Load(config.DataPath("config_supply.json"), &s.cfg)
	s.Require().NoError(err, "failed to load config_supply.json")

	s.T().Logf("Supply config: page=%d  limit=%d  tolerance=%.2f%%",
		s.cfg.EventGroupsPage, s.cfg.EventGroupsLimit, s.cfg.TolerancePercent*100)
}

// ──────────────────────────────────────────────────────────────────────────────
// TestSupplyIntegrity
//
// Invariant: floatingSupply + lockedSupply + packsReserve + lpReserve +
//            availableSupply  <=  maxSupply  (within tolerancePercent)
//
// Run alone:
//   go test ./tests/... -v -run TestSupplySuite/TestSupplyIntegrity
// ──────────────────────────────────────────────────────────────────────────────

func (s *SupplySuite) TestSupplyIntegrity() {
	cfg := s.Cfg
	outDir := filepath.Join(config.Root(), "reports")
	start := time.Now()

	token, err := s.getToken()
	s.Require().NoError(err)

	// Step 1 — fetch event groups from Spinner BFF.
	s.T().Logf("Fetching event groups (page=%d limit=%d) …",
		s.cfg.EventGroupsPage, s.cfg.EventGroupsLimit)

	groups, err := handlers.FetchEventGroups(
		cfg.SpinnerBFFURL, token,
		s.cfg.EventGroupsPage, s.cfg.EventGroupsLimit,
	)
	s.Require().NoError(err, "FetchEventGroups failed")
	s.Require().NotEmpty(groups, "no event groups returned")
	s.T().Logf("Fetched %d event group(s)", len(groups))
	s.EventGroups = groups

	ids := make([]string, len(groups))
	for i, g := range groups {
		ids[i] = g.ID
	}

	// Step 2 — fetch supply breakdowns from Proxy.
	s.T().Logf("Fetching supply breakdowns for %d event group(s) …", len(ids))
	breakdowns, err := handlers.FetchSupplyBreakdowns(cfg.ProxyURL, ids)
	s.Require().NoError(err, "FetchSupplyBreakdowns failed")
	s.Require().NotEmpty(breakdowns, "no supply data returned")
	s.Breakdowns = breakdowns

	// Step 3 — assert invariant and build report.
	s.T().Logf("Asserting supply integrity (tolerance=%.2f%%) …", s.cfg.TolerancePercent*100)

	rep := reporter.NewSupplyReport(cfg.Env, s.cfg.TolerancePercent*100)

	var failCount int
	for _, b := range breakdowns {
		total := b.Total()
		allowedMax := b.MaxSupply * (1 + s.cfg.TolerancePercent)
		overage := total - b.MaxSupply
		overagePct := safePercent(overage, b.MaxSupply)
		pass := total <= allowedMax

		rep.AddSupplyResult(reporter.SupplyResult{
			EventGroupID:    b.EventGroupID,
			MaxSupply:       b.MaxSupply,
			FloatingSupply:  b.FloatingSupply,
			LockedSupply:    b.LockedSupply,
			PacksReserve:    b.PacksReserve,
			LpReserve:       b.LpReserve,
			AvailableSupply: b.AvailableSupply,
			Total:           total,
			Overage:         overage,
			OveragePercent:  overagePct,
			Pass:            pass,
		})

		if !pass {
			failCount++
			s.T().Errorf(
				"FAIL | ID=%-30s | total=%.4f > maxSupply=%.4f | overage=%.4f (%.4f%%)\n"+
					"      floating=%.4f  locked=%.4f  packsReserve=%.4f  lpReserve=%.4f  available=%.4f",
				b.EventGroupID, total, b.MaxSupply, overage, overagePct,
				b.FloatingSupply, b.LockedSupply, b.PacksReserve, b.LpReserve, b.AvailableSupply,
			)
		} else {
			s.T().Logf("OK   | ID=%-30s | total=%.4f / maxSupply=%.4f",
				b.EventGroupID, total, b.MaxSupply)
		}
	}

	rep.FinalizeSupply(time.Since(start))

	if jsonPath, err := rep.WriteSupplyJSON(outDir); err != nil {
		s.T().Logf("could not write JSON report: %v", err)
	} else {
		s.T().Logf("JSON report: %s", jsonPath)
	}

	if htmlPath, err := rep.WriteSupplyHTML(outDir); err != nil {
		s.T().Logf("could not write HTML report: %v", err)
	} else {
		s.T().Logf("HTML report: %s", htmlPath)
	}

	s.T().Logf("────────────── SUPPLY INTEGRITY SUMMARY ──────────────")
	s.T().Logf("Event groups fetched:   %d", len(groups))
	s.T().Logf("Supply entries checked: %d", rep.Summary.TotalChecked)
	s.T().Logf("Pass:                   %d", rep.Summary.PassCount)
	s.T().Logf("Violations:             %d", rep.Summary.FailCount)
	s.T().Logf("Max overage:            %.4f%%", rep.Summary.MaxOverage)
	s.T().Logf("Tolerance:              %.2f%%", s.cfg.TolerancePercent*100)
	s.T().Logf("Wall time:              %d ms", rep.Summary.TotalTimeMs)
	s.T().Logf("───────────────────────────────────────────────────────")

	if failCount > 0 {
		s.Fail(fmt.Sprintf("%d supply integrity violation(s) — total exceeds maxSupply", failCount))
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// HELPERS
// ──────────────────────────────────────────────────────────────────────────────

// getToken authenticates the first user in users.json and returns their access token.
func (s *SupplySuite) getToken() (string, error) {
	users, err := handlers.LoadUsers(config.DataPath("users.json"))
	if err != nil || len(users) == 0 {
		return "", fmt.Errorf("getToken: could not load users.json: %w", err)
	}
	auth, err := handlers.GenerateTokens(s.Cfg.FcBFFURL, users[0])
	if err != nil {
		return "", fmt.Errorf("getToken: auth failed for %s: %w", users[0].Email, err)
	}
	return auth.AccessToken, nil
}

// safePercent returns (part/total)*100, avoiding division by zero.
func safePercent(part, total float64) float64 {
	if total == 0 {
		return 0
	}
	return (part / total) * 100
}
