package tests

import (
	"fmt"
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
	TolerancePercent float64 `json:"tolerancePercent"`
}

// ──────────────────────────────────────────────────────────────────────────────
// ROW TYPE
// ──────────────────────────────────────────────────────────────────────────────

type supplyRow struct {
	EventGroupID    string
	Slug            string
	MaxSupply       float64
	FloatingSupply  float64
	LockedSupply    float64
	PacksReserve    float64
	LpReserve       float64
	AvailableSupply float64
	Total           float64
	Overage         float64
	OveragePct      float64
	Pass            bool
}

func (r supplyRow) RowStatus() reporter.Status {
	if r.Pass {
		return reporter.StatusPass
	}
	return reporter.StatusFail
}
func (r supplyRow) RowLatency() time.Duration { return 0 }
func (r supplyRow) RowLabel() string          { return r.Slug }
func (r supplyRow) RowColumns() []reporter.Column {
	return []reporter.Column{
		{Header: "Max Supply", Value: fmt.Sprintf("%.2f", r.MaxSupply)},
		{Header: "Floating", Value: fmt.Sprintf("%.2f", r.FloatingSupply)},
		{Header: "Locked", Value: fmt.Sprintf("%.2f", r.LockedSupply)},
		{Header: "Packs Reserve", Value: fmt.Sprintf("%.2f", r.PacksReserve)},
		{Header: "LP Reserve", Value: fmt.Sprintf("%.2f", r.LpReserve)},
		{Header: "Available", Value: fmt.Sprintf("%.2f", r.AvailableSupply)},
		{Header: "Total", Value: fmt.Sprintf("%.2f", r.Total)},
		{Header: "Overage %", Value: fmt.Sprintf("%.4f%%", r.OveragePct)},
	}
}
func (r supplyRow) RowDetails() []reporter.Detail {
	details := []reporter.Detail{
		{Key: "Event Group ID", Value: r.EventGroupID, Mono: true},
	}
	if r.Pass {
		return details
	}
	return append(details, []reporter.Detail{
		{Key: "Max Supply", Value: fmt.Sprintf("%.6f", r.MaxSupply)},
		{Key: "Total Allocated", Value: fmt.Sprintf("%.6f", r.Total)},
		{Key: "Overage", Value: fmt.Sprintf("%.6f (%.4f%%)", r.Overage, r.OveragePct)},
		{Key: "Floating Supply", Value: fmt.Sprintf("%.6f", r.FloatingSupply)},
		{Key: "Locked Supply", Value: fmt.Sprintf("%.6f", r.LockedSupply)},
		{Key: "Packs Reserve", Value: fmt.Sprintf("%.6f", r.PacksReserve)},
		{Key: "LP Reserve", Value: fmt.Sprintf("%.6f", r.LpReserve)},
		{Key: "Available Supply", Value: fmt.Sprintf("%.6f", r.AvailableSupply)},
	}...)
}

// ──────────────────────────────────────────────────────────────────────────────
// SUITE
// ──────────────────────────────────────────────────────────────────────────────

type SupplySuite struct {
	BaseSuite
	cfg supplyConfig

	// Shared outputs for downstream dependent tests.
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
// ──────────────────────────────────────────────────────────────────────────────

func (s *SupplySuite) TestSupplyIntegrity() {
	cfg := s.Cfg

	run := reporter.NewRunner[supplyRow]("Supply Integrity", reporter.NewMeta(
		cfg.Env, cfg.SpinnerBFFURL, "config_supply.json",
		"supply", "integrity",
	))
	run.Annotate("tolerance", fmt.Sprintf("%.2f%%", s.cfg.TolerancePercent*100))
	run.Annotate("page", fmt.Sprintf("%d", s.cfg.EventGroupsPage))
	run.Annotate("limit", fmt.Sprintf("%d", s.cfg.EventGroupsLimit))

	token, err := s.getToken()
	s.Require().NoError(err)

	// Step 1 — fetch event groups.
	s.T().Logf("Fetching event groups (page=%d limit=%d) …", s.cfg.EventGroupsPage, s.cfg.EventGroupsLimit)
	groups, err := handlers.FetchEventGroups(cfg.SpinnerBFFURL, token, s.cfg.EventGroupsPage, s.cfg.EventGroupsLimit)
	s.Require().NoError(err, "FetchEventGroups failed")
	s.Require().NotEmpty(groups, "no event groups returned")
	s.EventGroups = groups
	run.Annotate("event_groups_fetched", fmt.Sprintf("%d", len(groups)))
	s.T().Logf("Fetched %d event group(s)", len(groups))

	ids := make([]string, len(groups))
	slugByID := make(map[string]string, len(groups))
	for i, g := range groups {
		ids[i] = g.ID
		slugByID[g.ID] = g.Slug
	}

	// Step 2 — fetch supply breakdowns.
	s.T().Logf("Fetching supply breakdowns for %d event group(s) …", len(ids))
	breakdowns, err := handlers.FetchSupplyBreakdowns(cfg.ProxyURL, ids)
	s.Require().NoError(err, "FetchSupplyBreakdowns failed")
	s.Require().NotEmpty(breakdowns, "no supply data returned")
	s.Breakdowns = breakdowns

	// Step 3 — assert invariant.
	s.T().Logf("Asserting supply integrity (tolerance=%.2f%%) …", s.cfg.TolerancePercent*100)
	var failCount int

	for _, b := range breakdowns {
		total := b.Total()
		allowedMax := b.MaxSupply * (1 + s.cfg.TolerancePercent)
		overage := total - b.MaxSupply
		overagePct := safePercent(overage, b.MaxSupply)
		pass := total <= allowedMax

		row := supplyRow{
			EventGroupID:    b.EventGroupID,
			Slug:            slugByID[b.EventGroupID],
			MaxSupply:       b.MaxSupply,
			FloatingSupply:  b.FloatingSupply,
			LockedSupply:    b.LockedSupply,
			PacksReserve:    b.PacksReserve,
			LpReserve:       b.LpReserve,
			AvailableSupply: b.AvailableSupply,
			Total:           total,
			Overage:         overage,
			OveragePct:      overagePct,
			Pass:            pass,
		}
		run.Add(row)

		if !pass {
			failCount++
			s.T().Errorf("FAIL | ID=%-30s | total=%.4f > max=%.4f | overage=%.4f (%.4f%%)",
				b.EventGroupID, total, b.MaxSupply, overage, overagePct)
		} else {
			s.T().Logf("OK   | ID=%-30s | total=%.4f / max=%.4f",
				b.EventGroupID, total, b.MaxSupply)
		}
	}

	rep := run.Finish()
	s.writeReport(rep, reportDir())
	s.storeSuiteReport(rep)
	s.logSummary(rep)

	if failCount > 0 {
		s.Fail(fmt.Sprintf("%d supply integrity violation(s)", failCount))
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// HELPERS
// ──────────────────────────────────────────────────────────────────────────────

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

func (s *SupplySuite) writeReport(rep *reporter.Report, outDir string) {
	jsonPath, err := reporter.WriteJSON(rep, outDir)
	if err != nil {
		s.T().Logf("report write error: %v", err)
		return
	}
	s.T().Logf("JSON report: %s", jsonPath)
}

func (s *SupplySuite) logSummary(rep *reporter.Report) {
	sm := rep.Summary
	s.T().Logf("────────── %s SUMMARY ──────────", rep.Name)
	s.T().Logf("Total: %d  Pass: %d  Fail: %d", sm.Total, sm.PassCount, sm.FailCount)
	s.T().Logf("Success rate: %.1f%%", sm.SuccessRate)
	s.T().Logf("Wall time: %d ms", sm.WallTimeMs)
	s.T().Logf("────────────────────────────────────────")
}

func safePercent(part, total float64) float64 {
	if total == 0 {
		return 0
	}
	return (part / total) * 100
}
