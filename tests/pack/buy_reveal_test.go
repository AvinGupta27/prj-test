package pack_test

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/AvinGupta27/code-go-automation/client"
	"github.com/AvinGupta27/code-go-automation/constants"
	"github.com/AvinGupta27/code-go-automation/reporter"
)

// ──────────────────────────────────────────────────────────────────────────────
// ROW TYPE
// ──────────────────────────────────────────────────────────────────────────────

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
	WalletSkipped     bool
	PriceCurrency     string
	ExpectedDeduction float64
	WalletBefore      float64
	WalletAfter       float64
	ActualDeduction   float64
	WalletCheckOK     bool
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
	if r.WalletBefore > 0 && !r.WalletCheckOK {
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
// TestBuyAndReveal
// ──────────────────────────────────────────────────────────────────────────────

func (s *PackSuite) TestBuyAndReveal() {
	s.authenticateUsers()
	cfg := s.Cfg

	packInfo, err := client.FetchPackInfo(cfg.SpinnerBFFURL, s.validTokens[0].AccessToken, s.cfg.PackMasterID)
	s.Require().NoError(err, "FetchPackInfo failed")

	s.T().Logf("Pack: %s | PriceConfig: %s | Currency: %s | Price: %.4f | Limit: %d",
		packInfo.PackName, packInfo.PriceConfigID, packInfo.PriceCurrency, packInfo.PriceValue, packInfo.PerSessionLimit)

	if packInfo.PerSessionLimit > 0 && s.cfg.Quantity > packInfo.PerSessionLimit {
		s.T().Logf("WARNING: quantity (%d) exceeds perSessionLimit (%d)", s.cfg.Quantity, packInfo.PerSessionLimit)
	}

	packReq := client.BuyPackRequest{
		PackMasterID:  s.cfg.PackMasterID,
		Quantity:      s.cfg.Quantity,
		PriceConfigID: packInfo.PriceConfigID,
	}

	run := reporter.NewRunner[buyRevealRow]("Buy and Reveal Packs", reporter.NewMeta(
		cfg.Env, cfg.SpinnerBFFURL, "pack_buy_reveal.json",
		"pack", "buy", "reveal",
	))
	run.Annotate("pack_name", packInfo.PackName)
	run.Annotate("pack_id", s.cfg.PackMasterID)
	run.Annotate("quantity_per_user", fmt.Sprintf("%d", s.cfg.Quantity))
	run.Annotate("users", fmt.Sprintf("%d", len(s.validTokens)))
	run.Annotate("price_config_id", packInfo.PriceConfigID)

	requiredBalance := packInfo.PriceValue * float64(s.cfg.Quantity)
	run.Annotate("price_currency", packInfo.PriceCurrency)
	run.Annotate("price_per_pack", fmt.Sprintf("%.4f %s", packInfo.PriceValue, packInfo.PriceCurrency))
	run.Annotate("required_balance", fmt.Sprintf("%.4f %s", requiredBalance, packInfo.PriceCurrency))

	rowCh := make(chan buyRevealRow, len(s.validTokens))
	var wg sync.WaitGroup

	for _, tok := range s.validTokens {
		wg.Add(1)
		go func(ut client.UserToken) {
			defer wg.Done()
			walletBefore, err := client.FetchWallet(cfg.FcBFFURL, ut.AccessToken)
			if err != nil {
				s.T().Logf("WARN | wallet fetch failed for %s: %v — proceeding without wallet check", ut.Email, err)
				rowCh <- runBuyAndReveal(cfg.SpinnerBFFURL, cfg.FcBFFURL, ut, packReq, packInfo.PriceCurrency, requiredBalance, 0, s.cfg.EnableReveal)
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
			rowCh <- runBuyAndReveal(cfg.SpinnerBFFURL, cfg.FcBFFURL, ut, packReq, packInfo.PriceCurrency, requiredBalance, balanceBefore, s.cfg.EnableReveal)
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
	s.WriteReport(rep)
	s.StoreSuiteReport(rep)
	s.LogSummary(rep)

	if failCount > 0 {
		s.Fail(fmt.Sprintf("%d buy/reveal failure(s)", failCount))
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// WORKER
// ──────────────────────────────────────────────────────────────────────────────

func runBuyAndReveal(spinnerBFFURL, fcBFFURL string, tok client.UserToken, req client.BuyPackRequest, priceCurrency string, expectedDeduction, walletBefore float64, enableReveal bool) buyRevealRow {
	buyStart := time.Now()
	buyRes := client.BuyPack(spinnerBFFURL, tok.AccessToken, req, tok.Email)
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
			RequestURL:     spinnerBFFURL + constants.PacksBuy,
			RequestBody:    fmt.Sprintf(`{"packMasterId":%q,"quantity":%d}`, req.PackMasterID, req.Quantity),
			ResponseStatus: buyRes.Status,
			ResponseBody:   buyRes.ErrorMsg,
			ErrorChain:     buyRes.ErrorMsg,
			OccurredAt:     time.Now(),
		}
		return row
	}

	if priceCurrency != "" && walletBefore > 0 {
		if walletAfter, err := client.FetchWallet(fcBFFURL, tok.AccessToken); err == nil {
			row.WalletAfter = walletAfter.Unlocked(priceCurrency)
			row.ActualDeduction = walletBefore - row.WalletAfter
			diff := row.ActualDeduction - expectedDeduction
			if diff < 0 {
				diff = -diff
			}
			row.WalletCheckOK = expectedDeduction == 0 || diff/expectedDeduction < 0.001
		}
	}

	row.PackIDs = buyRes.UserPackIDs
	if !enableReveal || len(buyRes.UserPackIDs) == 0 {
		return row
	}

	time.Sleep(500 * time.Millisecond)

	revCh := make(chan client.RevealResult, len(buyRes.UserPackIDs))
	var wg sync.WaitGroup
	revStart := time.Now()

	for _, id := range buyRes.UserPackIDs {
		wg.Add(1)
		go func(pid string) {
			defer wg.Done()
			revCh <- client.RevealPack(spinnerBFFURL, tok.AccessToken, pid, tok.Email)
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
				RequestURL:     spinnerBFFURL + constants.PackReveal,
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
