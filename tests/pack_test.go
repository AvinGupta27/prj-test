package tests

import (
	"fmt"
	"path/filepath"
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
	revealWorkers   = 5
	buyPackMasterID = "69ce3ade5f131ce16676c7b7"
	buyQuantity     = 1
	enableReveal    = true
	usersFile       = "users.json"
)

// ──────────────────────────────────────────────────────────────────────────────
// SUITE
// ──────────────────────────────────────────────────────────────────────────────

type PackSuite struct {
	BaseSuite
	validTokens []handlers.UserToken
}

// SetupSuite loads config (via BaseSuite), then authenticates all users once.
// Both TestReveal and TestBuyAndReveal share the resulting tokens.
func (s *PackSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	users, err := handlers.LoadUsers(config.DataPath(usersFile))
	s.Require().NoError(err, "failed to load users from %s", usersFile)
	s.Require().NotEmpty(users, "%s is empty — add at least one account", usersFile)
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
	s.Require().NotEmpty(s.validTokens, "all authentications failed — cannot proceed")
}

// Entry point — run with:
//
//	go test ./tests/... -v                            (both tests)
//	go test ./tests/... -v -run TestPackSuite/TestReveal
//	go test ./tests/... -v -run TestPackSuite/TestBuyAndReveal
func TestPackSuite(t *testing.T) {
	RunSuite(t, new(PackSuite))
}

// ──────────────────────────────────────────────────────────────────────────────
// TestReveal
//
// For every authenticated user, fetch their unrevealed packs and reveal them
// via a shared worker pool.
// ──────────────────────────────────────────────────────────────────────────────

type revealJob struct {
	token handlers.UserToken
	id    string
}

type revealResult struct {
	id        string
	email     string
	success   bool
	status    int
	latencyMs int64
	errorMsg  string
}

func (s *PackSuite) TestReveal() {
	cfg := s.Cfg

	// Collect all (token, packID) pairs across every user.
	var allJobs []revealJob
	for _, tok := range s.validTokens {
		ids, err := handlers.FetchUnrevealedPackIDs(cfg.SpinnerBFFURL, tok.AccessToken)
		if err != nil {
			s.T().Errorf("failed fetching unrevealed packs for %s: %v", tok.Email, err)
			continue
		}
		s.T().Logf("Found %d unrevealed pack(s) for %s", len(ids), tok.Email)
		for _, id := range ids {
			allJobs = append(allJobs, revealJob{token: tok, id: id})
		}
	}

	if len(allJobs) == 0 {
		s.T().Skip("No unrevealed packs found for any user.")
	}
	s.T().Logf("Total unrevealed packs: %d  |  Workers: %d", len(allJobs), revealWorkers)

	jobs := make(chan revealJob, len(allJobs))
	results := make(chan revealResult, len(allJobs))
	var wg sync.WaitGroup

	for w := 0; w < revealWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				rr := handlers.RevealPack(cfg.SpinnerBFFURL, job.token.AccessToken, job.id, job.token.Email)
				results <- revealResult{
					id:        job.id,
					email:     job.token.Email,
					success:   rr.Success,
					status:    rr.Status,
					latencyMs: rr.LatencyMs,
					errorMsg:  rr.ErrorMsg,
				}
			}
		}()
	}

	for _, job := range allJobs {
		jobs <- job
	}
	close(jobs)

	go func() { wg.Wait(); close(results) }()

	rep := reporter.New(cfg.Env, "Reveal Packs API", revealWorkers)
	outDir := filepath.Join(config.Root(), "reports")
	start := time.Now()

	var failCount int
	userErrors := make(map[string][]string)

	for res := range results {
		rep.AddResult(reporter.TestResult{
			ID:        res.id,
			Success:   res.success,
			Status:    res.status,
			LatencyMs: res.latencyMs,
			ErrorMsg:  res.errorMsg,
		})
		if !res.success {
			failCount++
			userErrors[res.email] = append(userErrors[res.email],
				fmt.Sprintf("[%s] %s", res.id, res.errorMsg))
			s.T().Errorf("FAIL | User=%-30s | ID=%-30s | Status=%d | Error=%s",
				res.email, res.id, res.status, res.errorMsg)
		} else {
			s.T().Logf("OK   | User=%-30s | ID=%-30s | Status=%d | %dms",
				res.email, res.id, res.status, res.latencyMs)
		}
	}

	rep.Finalize(time.Since(start))
	s.writeReports(rep, outDir)

	if len(userErrors) > 0 {
		s.T().Logf("────────── PER-USER FAILURES ──────────")
		for email, errs := range userErrors {
			s.T().Logf("%s: %s", email, strings.Join(errs, " | "))
		}
	}

	s.T().Logf("────────────── REVEAL SUMMARY ──────────────")
	s.T().Logf("Total users:    %d", len(s.validTokens))
	s.T().Logf("Total packs:    %d", rep.Summary.TotalRequests)
	s.T().Logf("Success:        %d", rep.Summary.SuccessCount)
	s.T().Logf("Failed:         %d", rep.Summary.FailureCount)
	s.T().Logf("Avg latency:    %d ms", rep.Summary.AvgLatencyMs)
	s.T().Logf("Wall time:      %d ms", rep.Summary.TotalTimeMs)
	s.T().Logf("────────────────────────────────────────────")

	if failCount > 0 {
		s.Fail(fmt.Sprintf("%d reveal failure(s)", failCount))
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestBuyAndReveal
//
// For every authenticated user, buy N packs then reveal all returned pack IDs.
// ──────────────────────────────────────────────────────────────────────────────

type buyRevealJob struct {
	buyResult     handlers.BuyPackResult
	revealResults []handlers.RevealResult
}

func (s *PackSuite) TestBuyAndReveal() {
	cfg := s.Cfg

	// Resolve pack config once using the first valid token.
	s.T().Logf("Fetching pack config (packId=%s) …", buyPackMasterID)
	packInfo, err := handlers.FetchPackInfo(cfg.SpinnerBFFURL, s.validTokens[0].AccessToken, buyPackMasterID)
	s.Require().NoError(err, "FetchPackInfo failed")

	s.T().Logf("Pack:          %s", packInfo.PackName)
	s.T().Logf("Price Config:  id=%s  currency=%s  price=%.4f",
		packInfo.PriceConfigID, packInfo.PriceCurrency, packInfo.PriceValue)
	s.T().Logf("Session limit: %d  |  Sold: %d / %d",
		packInfo.PerSessionLimit, packInfo.PacksSold, packInfo.TotalPackCount)
	if packInfo.PackOpeningDate != "" {
		s.T().Logf("Opening date:  %s", packInfo.PackOpeningDate)
	}
	if packInfo.PerSessionLimit > 0 && buyQuantity > packInfo.PerSessionLimit {
		s.T().Logf("WARNING: buyQuantity (%d) exceeds perSessionLimit (%d)",
			buyQuantity, packInfo.PerSessionLimit)
	}

	packReq := handlers.BuyPackRequest{
		PackMasterID:  buyPackMasterID,
		Quantity:      buyQuantity,
		PriceConfigID: packInfo.PriceConfigID,
	}

	// One goroutine per user — buy then reveal.
	results := make(chan buyRevealJob, len(s.validTokens))
	var wg sync.WaitGroup
	start := time.Now()

	for _, tok := range s.validTokens {
		wg.Add(1)
		go func(ut handlers.UserToken) {
			defer wg.Done()
			results <- runBuyAndReveal(cfg.SpinnerBFFURL, ut, packReq)
		}(tok)
	}

	wg.Wait()
	close(results)

	packCfg := reporter.BuyPackConfig{
		PackMasterID:  buyPackMasterID,
		Quantity:      buyQuantity,
		PriceConfigID: packInfo.PriceConfigID,
		SpinnerBFFURL: cfg.SpinnerBFFURL,
	}
	rep := reporter.NewBuyReveal(cfg.Env, packCfg)
	outDir := filepath.Join(config.Root(), "reports")

	for res := range results {
		br := res.buyResult
		row := reporter.BuyRevealResult{
			Email:        br.Email,
			UserPackIDs:  br.UserPackIDs,
			BuySuccess:   br.Success,
			BuyStatus:    br.Status,
			BuyLatencyMs: br.LatencyMs,
			BuyError:     br.ErrorMsg,
			RevealDone:   len(res.revealResults) > 0,
		}

		var revealErrors []string
		allRevealsOK := true
		for _, rr := range res.revealResults {
			if !rr.Success {
				allRevealsOK = false
				revealErrors = append(revealErrors, fmt.Sprintf("[%s] %s", rr.UserPackID, rr.ErrorMsg))
			}
			row.RevLatencyMs += rr.LatencyMs
			row.NFTCount += rr.NFTCount
			row.TotalValue += rr.TotalValue
			for _, n := range rr.NFTs {
				row.NFTs = append(row.NFTs, reporter.NFTItem{
					NFTTokenID: n.NFTTokenID,
					CardName:   n.CardName,
					Rarity:     n.Rarity,
					Value:      n.Value,
				})
			}
		}
		if len(res.revealResults) > 0 {
			row.RevLatencyMs /= int64(len(res.revealResults))
			row.RevSuccess = allRevealsOK
			if !allRevealsOK {
				row.RevError = strings.Join(revealErrors, " | ")
			}
		}

		rep.AddBuyRevealResult(row)

		if !br.Success {
			s.T().Errorf("BUY FAILED    | User=%s | Status=%d | Error=%s",
				br.Email, br.Status, br.ErrorMsg)
		} else if !allRevealsOK {
			s.T().Errorf("REVEAL FAILED | User=%s | Errors=%s", br.Email, row.RevError)
		} else {
			s.T().Logf("OK | User=%-30s | Packs=%d | NFTs=%d | Value=%.2f",
				br.Email, len(br.UserPackIDs), row.NFTCount, row.TotalValue)
		}
	}

	rep.FinalizeBuyReveal(time.Since(start))
	s.writeBuyRevealReports(rep, outDir)

	sm := rep.Summary
	s.T().Logf("────────────── BUY & REVEAL SUMMARY ──────────────")
	s.T().Logf("Pack:               %s (%s)", packInfo.PackName, buyPackMasterID)
	s.T().Logf("Total Users:        %d", sm.TotalUsers)
	s.T().Logf("Buy  Success:       %d / %d", sm.BuySuccessCount, sm.TotalUsers)
	s.T().Logf("Reveal Success:     %d / %d", sm.RevealSuccessCount, sm.BuySuccessCount)
	s.T().Logf("Total NFTs:         %d", sm.TotalNFTs)
	s.T().Logf("Total NFT Value:    %.2f", sm.TotalNFTValue)
	s.T().Logf("Avg Buy Latency:    %d ms", sm.AvgBuyLatencyMs)
	s.T().Logf("Avg Reveal Latency: %d ms", sm.AvgRevLatencyMs)
	s.T().Logf("Wall Time:          %d ms", sm.TotalTimeMs)
	s.T().Logf("───────────────────────────────────────────────────")
}

// ──────────────────────────────────────────────────────────────────────────────
// HELPERS
// ──────────────────────────────────────────────────────────────────────────────

// runBuyAndReveal buys packs for one user then reveals all returned IDs concurrently.
func runBuyAndReveal(spinnerBFFURL string, tok handlers.UserToken, req handlers.BuyPackRequest) buyRevealJob {
	buyRes := handlers.BuyPack(spinnerBFFURL, tok.AccessToken, req, tok.Email)
	job := buyRevealJob{buyResult: buyRes}

	if !enableReveal || !buyRes.Success || len(buyRes.UserPackIDs) == 0 {
		return job
	}

	time.Sleep(500 * time.Millisecond)

	revealCh := make(chan handlers.RevealResult, len(buyRes.UserPackIDs))
	var wg sync.WaitGroup
	for _, packID := range buyRes.UserPackIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			revealCh <- handlers.RevealPack(spinnerBFFURL, tok.AccessToken, id, tok.Email)
		}(packID)
	}
	wg.Wait()
	close(revealCh)

	for rr := range revealCh {
		job.revealResults = append(job.revealResults, rr)
	}
	return job
}

// writeReports writes JSON + HTML for the reveal-only reporter.
func (s *PackSuite) writeReports(rep *reporter.Report, outDir string) {
	if path, err := rep.WriteJSON(outDir); err != nil {
		s.T().Logf("could not write JSON report: %v", err)
	} else {
		s.T().Logf("JSON report: %s", path)
	}
	if path, err := rep.WriteHTML(outDir); err != nil {
		s.T().Logf("could not write HTML report: %v", err)
	} else {
		s.T().Logf("HTML report: %s", path)
	}
}

// writeBuyRevealReports writes JSON + HTML for the buy+reveal reporter.
func (s *PackSuite) writeBuyRevealReports(rep *reporter.BuyRevealReport, outDir string) {
	if path, err := rep.WriteBuyRevealJSON(outDir); err != nil {
		s.T().Logf("could not write JSON report: %v", err)
	} else {
		s.T().Logf("JSON report: %s", path)
	}
	if path, err := rep.WriteBuyRevealHTML(outDir); err != nil {
		s.T().Logf("could not write HTML report: %v", err)
	} else {
		s.T().Logf("HTML report: %s", path)
	}
}
