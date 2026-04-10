package tests

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AvinGupta27/code-go-automation/config"
	"github.com/AvinGupta27/code-go-automation/handlers"
	"github.com/AvinGupta27/code-go-automation/reporter"
)

// ──────────────────────────────────────────────────────────────────────────────
// CONFIGURATION
// ──────────────────────────────────────────────────────────────────────────────

const (
	revealWorkers   = 10
	buyPackMasterID = "69ce3ade5f131ce16676c7b7"
	buyQuantity     = 2
	enableReveal    = false
	usersFile       = "users.json"
)

// ──────────────────────────────────────────────────────────────────────────────
// ROW TYPES
// Each implements reporter.Row — no other reporting code needed in this file.
// ──────────────────────────────────────────────────────────────────────────────

// revealRow is the result of one reveal API call.
type revealRow struct {
	PackID     string
	Email      string
	HTTPStatus int
	Dur        time.Duration
	Failure    reporter.FailureDetail
}

func (r revealRow) RowStatus() reporter.Status {
	if r.Failure.IsZero() && r.HTTPStatus == 200 {
		return reporter.StatusPass
	}
	return reporter.StatusFail
}
func (r revealRow) RowLatency() time.Duration { return r.Dur }
func (r revealRow) RowLabel() string          { return r.PackID }
func (r revealRow) RowColumns() []reporter.Column {
	return []reporter.Column{
		{Header: "User", Value: r.Email},
		{Header: "HTTP", Value: fmt.Sprintf("%d", r.HTTPStatus)},
		{Header: "Latency", Value: fmt.Sprintf("%d ms", r.Dur.Milliseconds())},
	}
}
func (r revealRow) RowDetails() []reporter.Detail {
	return r.Failure.ToDetails()
}

// buyRevealRow is the result of one user's full buy + reveal flow.
type buyRevealRow struct {
	Email             string
	PackIDs           []string
	NFTCount          int
	TotalValue        float64
	BuyStatus         int
	BuyDur            time.Duration
	RevDur            time.Duration
	BuyFailure        reporter.FailureDetail
	RevFailures       []reporter.FailureDetail
	// Wallet fields
	WalletSkipped     bool    // true if skipped due to insufficient balance
	PriceCurrency     string
	ExpectedDeduction float64 // price × quantity
	WalletBefore      float64 // unlocked balance before buy
	WalletAfter       float64 // unlocked balance after buy
	ActualDeduction   float64 // WalletBefore - WalletAfter
	WalletCheckOK     bool    // true if actual deduction matches expected
}

func (r buyRevealRow) RowStatus() reporter.Status {
	if r.WalletSkipped {
		return reporter.StatusSkip
	}
	if !r.BuyFailure.IsZero() {
		return reporter.StatusFail
	}
	for _, f := range r.RevFailures {
		if !f.IsZero() {
			return reporter.StatusFail
		}
	}
	if !r.WalletCheckOK {
		return reporter.StatusWarning
	}
	return reporter.StatusPass
}
func (r buyRevealRow) RowLatency() time.Duration { return r.BuyDur + r.RevDur }
func (r buyRevealRow) RowLabel() string          { return r.Email }
func (r buyRevealRow) RowColumns() []reporter.Column {
	walletStatus := "—"
	if !r.WalletSkipped && r.PriceCurrency != "" {
		if r.WalletCheckOK {
			walletStatus = fmt.Sprintf("✓ %.4f deducted", r.ActualDeduction)
		} else {
			walletStatus = fmt.Sprintf("✗ expected %.4f got %.4f", r.ExpectedDeduction, r.ActualDeduction)
		}
	}
	return []reporter.Column{
		{Header: "Packs", Value: fmt.Sprintf("%d", len(r.PackIDs))},
		{Header: "NFTs", Value: fmt.Sprintf("%d", r.NFTCount)},
		{Header: "Total Value", Value: fmt.Sprintf("%.2f", r.TotalValue)},
		{Header: "Buy Status", Value: fmt.Sprintf("%d", r.BuyStatus)},
		{Header: "Buy Latency", Value: fmt.Sprintf("%d ms", r.BuyDur.Milliseconds())},
		{Header: "Rev Latency", Value: fmt.Sprintf("%d ms", r.RevDur.Milliseconds())},
		{Header: "Wallet Check", Value: walletStatus},
	}
}
func (r buyRevealRow) RowDetails() []reporter.Detail {
	var d []reporter.Detail

	if r.WalletSkipped {
		d = append(d, reporter.Detail{Key: "Skip Reason", Value: fmt.Sprintf("Insufficient %s balance (%.4f < %.4f required)", r.PriceCurrency, r.WalletBefore, r.ExpectedDeduction)})
		return d
	}

	// Wallet section — always shown when wallet data is available.
	if r.PriceCurrency != "" {
		d = append(d,
			reporter.Detail{Key: "Currency", Value: r.PriceCurrency},
			reporter.Detail{Key: "Balance Before", Value: fmt.Sprintf("%.6f", r.WalletBefore)},
			reporter.Detail{Key: "Balance After", Value: fmt.Sprintf("%.6f", r.WalletAfter)},
			reporter.Detail{Key: "Expected Deduction", Value: fmt.Sprintf("%.6f", r.ExpectedDeduction)},
			reporter.Detail{Key: "Actual Deduction", Value: fmt.Sprintf("%.6f", r.ActualDeduction)},
			reporter.Detail{Key: "Wallet Check", Value: map[bool]string{true: "PASS", false: "FAIL — deduction mismatch"}[r.WalletCheckOK]},
		)
	}

	if len(r.PackIDs) > 0 {
		d = append(d, reporter.Detail{Key: "Pack IDs", Value: strings.Join(r.PackIDs, ", "), Mono: true})
	}
	d = append(d, r.BuyFailure.ToDetails()...)
	for _, f := range r.RevFailures {
		d = append(d, f.ToDetails()...)
	}
	return d
}

// ──────────────────────────────────────────────────────────────────────────────
// SUITE
// ──────────────────────────────────────────────────────────────────────────────

type PackSuite struct {
	BaseSuite
	validTokens []handlers.UserToken
}

func (s *PackSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	users, err := handlers.LoadUsers(config.DataPath(usersFile))
	s.Require().NoError(err, "failed to load users from %s", usersFile)
	s.Require().NotEmpty(users, "%s is empty", usersFile)
	s.T().Logf("Loaded %d user account(s)", len(users))

	s.T().Log("Authenticating all users in parallel …")
	tokens := handlers.GenerateAllTokens(s.Cfg.FcBFFURL, users)
	for _, tok := range tokens {
		if tok.Err != nil {
			s.T().Logf("Auth SKIPPED for %s: %v", tok.Email, tok.Err)
		} else {
			s.validTokens = append(s.validTokens, tok)
			s.T().Logf("Auth OK for %s", tok.Email)
		}
	}
	s.Require().NotEmpty(s.validTokens, "all authentications failed")
}

func TestPackSuite(t *testing.T) {
	RunSuite(t, new(PackSuite))
}

// ──────────────────────────────────────────────────────────────────────────────
// TestReveal
// ──────────────────────────────────────────────────────────────────────────────

func (s *PackSuite) TestReveal() {
	cfg := s.Cfg
	outDir := reportDir()

	run := reporter.NewRunner[revealRow]("Reveal Packs", reporter.NewMeta(
		cfg.Env, cfg.SpinnerBFFURL, "",
		"pack", "reveal",
	))
	run.Annotate("users_authed", fmt.Sprintf("%d", len(s.validTokens)))

	// Collect all (token, packID) jobs across every user.
	type job struct {
		tok handlers.UserToken
		id  string
	}
	var allJobs []job
	var totalUnrevealed int

	for _, tok := range s.validTokens {
		ids, err := handlers.FetchUnrevealedPackIDs(cfg.SpinnerBFFURL, tok.AccessToken)
		if err != nil {
			s.T().Errorf("fetch unrevealed failed for %s: %v", tok.Email, err)
			continue
		}
		totalUnrevealed += len(ids)
		s.T().Logf("Found %d unrevealed pack(s) for %s", len(ids), tok.Email)
		for _, id := range ids {
			allJobs = append(allJobs, job{tok: tok, id: id})
		}
	}

	run.Annotate("total_packs", fmt.Sprintf("%d", totalUnrevealed))

	if len(allJobs) == 0 {
		s.T().Skip("No unrevealed packs found for any user.")
	}

	// Worker pool.
	jobCh := make(chan job, len(allJobs))
	rowCh := make(chan revealRow, len(allJobs))
	var wg sync.WaitGroup

	for w := 0; w < revealWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobCh {
				start := time.Now()
				rr := handlers.RevealPack(cfg.SpinnerBFFURL, j.tok.AccessToken, j.id, j.tok.Email)
				dur := time.Since(start)

				row := revealRow{
					PackID:     j.id,
					Email:      j.tok.Email,
					HTTPStatus: rr.Status,
					Dur:        dur,
				}
				if !rr.Success {
					row.Failure = reporter.FailureDetail{
						RequestMethod:  "POST",
						RequestURL:     cfg.SpinnerBFFURL + "/api/v1/userpacks/reveal",
						RequestBody:    fmt.Sprintf(`{"userPackId":"%s"}`, j.id),
						ResponseStatus: rr.Status,
						ResponseBody:   rr.ErrorMsg,
						ErrorChain:     rr.ErrorMsg,
						OccurredAt:     time.Now(),
					}
				}
				rowCh <- row
			}
		}()
	}

	for _, j := range allJobs {
		jobCh <- j
	}
	close(jobCh)
	go func() { wg.Wait(); close(rowCh) }()

	var failCount int
	for row := range rowCh {
		run.Add(row)
		if row.RowStatus() == reporter.StatusFail {
			failCount++
			s.T().Errorf("FAIL | User=%-30s | ID=%-30s | Status=%d | %s",
				row.Email, row.PackID, row.HTTPStatus, row.Failure.ErrorChain)
		} else {
			s.T().Logf("OK   | User=%-30s | ID=%-30s | %d ms",
				row.Email, row.PackID, row.Dur.Milliseconds())
		}
	}

	rep := run.Finish()
	s.writeReport(rep, outDir)
	s.storeSuiteReport(rep)
	s.logSummary(rep)

	if failCount > 0 {
		s.Fail(fmt.Sprintf("%d reveal failure(s)", failCount))
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestBuyAndReveal
// ──────────────────────────────────────────────────────────────────────────────

func (s *PackSuite) TestBuyAndReveal() {
	cfg := s.Cfg
	outDir := reportDir()

	packInfo, err := handlers.FetchPackInfo(cfg.SpinnerBFFURL, s.validTokens[0].AccessToken, buyPackMasterID)
	s.Require().NoError(err, "FetchPackInfo failed")

	s.T().Logf("Pack: %s | PriceConfig: %s | Currency: %s | Price: %.4f | Limit: %d",
		packInfo.PackName, packInfo.PriceConfigID, packInfo.PriceCurrency, packInfo.PriceValue, packInfo.PerSessionLimit)

	if packInfo.PerSessionLimit > 0 && buyQuantity > packInfo.PerSessionLimit {
		s.T().Logf("WARNING: buyQuantity (%d) exceeds perSessionLimit (%d)", buyQuantity, packInfo.PerSessionLimit)
	}

	packReq := handlers.BuyPackRequest{
		PackMasterID:  buyPackMasterID,
		Quantity:      buyQuantity,
		PriceConfigID: packInfo.PriceConfigID,
	}

	run := reporter.NewRunner[buyRevealRow]("Buy and Reveal Packs", reporter.NewMeta(
		cfg.Env, cfg.SpinnerBFFURL, "",
		"pack", "buy", "reveal",
	))
	run.Annotate("pack_name", packInfo.PackName)
	run.Annotate("pack_id", buyPackMasterID)
	run.Annotate("quantity_per_user", fmt.Sprintf("%d", buyQuantity))
	run.Annotate("users", fmt.Sprintf("%d", len(s.validTokens)))
	run.Annotate("price_config_id", packInfo.PriceConfigID)

	requiredBalance := packInfo.PriceValue * float64(buyQuantity)
	run.Annotate("price_currency", packInfo.PriceCurrency)
	run.Annotate("price_per_pack", fmt.Sprintf("%.4f %s", packInfo.PriceValue, packInfo.PriceCurrency))
	run.Annotate("required_balance", fmt.Sprintf("%.4f %s", requiredBalance, packInfo.PriceCurrency))

	rowCh := make(chan buyRevealRow, len(s.validTokens))
	var wg sync.WaitGroup

	for _, tok := range s.validTokens {
		wg.Add(1)
		go func(ut handlers.UserToken) {
			defer wg.Done()

			// Fetch wallet before buy — skip user if balance is insufficient.
			walletBefore, err := handlers.FetchWallet(cfg.FcBFFURL, ut.AccessToken)
			if err != nil {
				s.T().Logf("WARN | wallet fetch failed for %s: %v — proceeding without wallet check", ut.Email, err)
				rowCh <- runBuyAndReveal(cfg.SpinnerBFFURL, cfg.FcBFFURL, ut, packReq, packInfo.PriceCurrency, requiredBalance, 0)
				return
			}

			balanceBefore := walletBefore.Unlocked(packInfo.PriceCurrency)
			if !walletBefore.HasSufficientBalance(packInfo.PriceCurrency, requiredBalance) {
				s.T().Logf("SKIP | %s | %s balance %.4f < required %.4f",
					ut.Email, packInfo.PriceCurrency, balanceBefore, requiredBalance)
				rowCh <- buyRevealRow{
					Email:             ut.Email,
					WalletSkipped:     true,
					PriceCurrency:     packInfo.PriceCurrency,
					ExpectedDeduction: requiredBalance,
					WalletBefore:      balanceBefore,
				}
				return
			}

			s.T().Logf("OK   | %s | %s balance %.4f >= required %.4f — proceeding",
				ut.Email, packInfo.PriceCurrency, balanceBefore, requiredBalance)
			rowCh <- runBuyAndReveal(cfg.SpinnerBFFURL, cfg.FcBFFURL, ut, packReq, packInfo.PriceCurrency, requiredBalance, balanceBefore)
		}(tok)
	}
	wg.Wait()
	close(rowCh)

	var failCount int
	for row := range rowCh {
		run.Add(row)
		if row.RowStatus() == reporter.StatusFail {
			failCount++
			s.T().Errorf("FAIL | User=%-30s | BuyStatus=%d | %s",
				row.Email, row.BuyStatus, row.BuyFailure.ErrorChain)
		} else {
			s.T().Logf("OK   | User=%-30s | Packs=%d | NFTs=%d | Value=%.2f",
				row.Email, len(row.PackIDs), row.NFTCount, row.TotalValue)
		}
	}

	rep := run.Finish()
	s.writeReport(rep, outDir)
	s.storeSuiteReport(rep)
	s.logSummary(rep)

	if failCount > 0 {
		s.Fail(fmt.Sprintf("%d buy/reveal failure(s)", failCount))
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// WORKER
// ──────────────────────────────────────────────────────────────────────────────

func runBuyAndReveal(spinnerBFFURL, fcBFFURL string, tok handlers.UserToken, req handlers.BuyPackRequest, priceCurrency string, expectedDeduction, walletBefore float64) buyRevealRow {
	buyStart := time.Now()
	buyRes := handlers.BuyPack(spinnerBFFURL, tok.AccessToken, req, tok.Email)
	buyDur := time.Since(buyStart)

	row := buyRevealRow{
		Email:             tok.Email,
		BuyStatus:         buyRes.Status,
		BuyDur:            buyDur,
		PriceCurrency:     priceCurrency,
		ExpectedDeduction: expectedDeduction,
		WalletBefore:      walletBefore,
	}

	if !buyRes.Success {
		row.BuyFailure = reporter.FailureDetail{
			RequestMethod:  "POST",
			RequestURL:     spinnerBFFURL + "/api/v1/packsmaster/buy",
			RequestBody:    fmt.Sprintf(`{"packMasterId":%q,"quantity":%d}`, req.PackMasterID, req.Quantity),
			ResponseStatus: buyRes.Status,
			ResponseBody:   buyRes.ErrorMsg,
			ErrorChain:     buyRes.ErrorMsg,
			OccurredAt:     time.Now(),
		}
		return row
	}

	// Fetch wallet after buy to verify deduction — only when we have a before balance.
	if priceCurrency != "" && walletBefore > 0 {
		if walletAfter, err := handlers.FetchWallet(fcBFFURL, tok.AccessToken); err == nil {
			row.WalletAfter = walletAfter.Unlocked(priceCurrency)
			row.ActualDeduction = walletBefore - row.WalletAfter
			// Allow 0.0001 tolerance for floating-point imprecision.
			diff := row.ActualDeduction - expectedDeduction
			if diff < 0 {
				diff = -diff
			}
			row.WalletCheckOK = diff < 0.0001
		}
	}

	row.PackIDs = buyRes.UserPackIDs
	if !enableReveal || len(buyRes.UserPackIDs) == 0 {
		return row
	}

	time.Sleep(500 * time.Millisecond)

	revCh := make(chan handlers.RevealResult, len(buyRes.UserPackIDs))
	var wg sync.WaitGroup
	revStart := time.Now()

	for _, id := range buyRes.UserPackIDs {
		wg.Add(1)
		go func(pid string) {
			defer wg.Done()
			revCh <- handlers.RevealPack(spinnerBFFURL, tok.AccessToken, pid, tok.Email)
		}(id)
	}
	wg.Wait()
	close(revCh)
	row.RevDur = time.Since(revStart)

	for rr := range revCh {
		row.NFTCount += rr.NFTCount
		row.TotalValue += rr.TotalValue
		if !rr.Success {
			row.RevFailures = append(row.RevFailures, reporter.FailureDetail{
				RequestMethod:  "POST",
				RequestURL:     spinnerBFFURL + "/api/v1/userpacks/reveal",
				RequestBody:    fmt.Sprintf(`{"userPackId":%q}`, rr.UserPackID),
				ResponseStatus: rr.Status,
				ResponseBody:   rr.ErrorMsg,
				ErrorChain:     rr.ErrorMsg,
				OccurredAt:     time.Now(),
			})
		}
	}
	return row
}

// ──────────────────────────────────────────────────────────────────────────────
// HELPERS
// ──────────────────────────────────────────────────────────────────────────────

func (s *PackSuite) writeReport(rep *reporter.Report, outDir string) {
	jsonPath, err := reporter.WriteJSON(rep, outDir)
	if err != nil {
		s.T().Logf("report write error: %v", err)
		return
	}
	s.T().Logf("JSON report: %s", jsonPath)
}

func (s *PackSuite) logSummary(rep *reporter.Report) {
	sm := rep.Summary
	s.T().Logf("────────── %s SUMMARY ──────────", rep.Name)
	s.T().Logf("Total: %d  Pass: %d  Fail: %d  Warn: %d  Skip: %d",
		sm.Total, sm.PassCount, sm.FailCount, sm.WarnCount, sm.SkipCount)
	s.T().Logf("Success rate: %.1f%%", sm.SuccessRate)
	s.T().Logf("Latency — p50:%dms  p95:%dms  p99:%dms  max:%dms",
		sm.Latency.P50Ms, sm.Latency.P95Ms, sm.Latency.P99Ms, sm.Latency.MaxMs)
	s.T().Logf("Wall time: %d ms", sm.WallTimeMs)
	s.T().Logf("────────────────────────────────────────")
}
