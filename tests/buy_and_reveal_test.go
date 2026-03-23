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
//
// Only the pack ID, quantity, and country code live here.
// The priceConfigId is resolved at runtime via the enriched API so there is
// no hardcoded pricing to keep in sync with the backend.
// ──────────────────────────────────────────────────────────────────────────────

const (
	buyPackMasterID = "69bd3fc8f030ad91a0caa41b"
	buyQuantity     = 25
	enableReveal    = false

	// usersFile lists all test accounts: [{ "email": "...", "otp": "..." }, …]
	usersFile = "users.json"
)

// ──────────────────────────────────────────────────────────────────────────────
// jobResult holds the complete outcome for one user's buy + all reveals.
// When buyQuantity > 1 there are multiple packs to reveal per user.
// ──────────────────────────────────────────────────────────────────────────────
type jobResult struct {
	buyResult     handlers.BuyPackResult
	revealResults []handlers.RevealResult // one entry per userPackId returned
}

// ──────────────────────────────────────────────────────────────────────────────
// TestBuyAndRevealPacks
//
// Flow:
//  1. Load + authenticate all users in parallel
//  2. Call /api/v2/packs/enriched to resolve the priceConfigId at runtime
//  3. Each user concurrently: BuyPack → RevealPack (for every returned packId)
//  4. Aggregate NFT counts + values and emit JSON + HTML report to /reports/
//
// ──────────────────────────────────────────────────────────────────────────────
func TestBuyAndRevealPacks(t *testing.T) {
	// -------- SETUP --------
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config load error: %v", err)
	}

	// Load all test user accounts.
	users, err := handlers.LoadUsers(config.DataPath(usersFile))
	if err != nil {
		t.Fatalf("failed to load users from %s: %v", usersFile, err)
	}
	if len(users) == 0 {
		t.Fatal("users.json is empty — add at least one account")
	}
	t.Logf("👥 Loaded %d user account(s)", len(users))

	// Authenticate all users in parallel.
	t.Log("🔑 Authenticating all users in parallel …")
	tokens := handlers.GenerateAllTokens(cfg.FcBFFURL, users)

	// Partition tokens into valid / failed.
	var validTokens []handlers.UserToken
	for _, tok := range tokens {
		if tok.Err != nil {
			t.Errorf("❌ Auth FAILED for %s: %v", tok.Email, tok.Err)
		} else {
			validTokens = append(validTokens, tok)
			t.Logf("✅ Auth OK   for %s", tok.Email)
		}
	}
	if len(validTokens) == 0 {
		t.Fatal("all authentications failed — cannot proceed")
	}

	// -------- RESOLVE PACK CONFIG DYNAMICALLY --------
	// Use the first valid token to call the enriched API
	t.Logf("🔍 Fetching pack config from enriched API (packId=%s) …", buyPackMasterID)

	packInfo, err := handlers.FetchPackInfo(
		cfg.SpinnerBFFURL,
		validTokens[0].AccessToken,
		buyPackMasterID,
	)
	if err != nil {
		t.Fatalf("❌ FetchPackInfo failed: %v", err)
	}

	t.Logf("📦 Pack:          %s", packInfo.PackName)
	t.Logf("🆔 Pack ID:       %s", packInfo.PackMasterID)
	t.Logf("💰 Price Config:  id=%s  currency=%s  price=%.4f",
		packInfo.PriceConfigID, packInfo.PriceCurrency, packInfo.PriceValue)
	t.Logf("📊 Session limit: %d  |  Sold: %d / %d",
		packInfo.PerSessionLimit, packInfo.PacksSold, packInfo.TotalPackCount)
	if packInfo.PackOpeningDate != "" {
		t.Logf("📅 Opening date:  %s", packInfo.PackOpeningDate)
	}

	// Warn if the requested quantity would exceed the per-session limit.
	if packInfo.PerSessionLimit > 0 && buyQuantity > packInfo.PerSessionLimit {
		t.Logf("⚠️  buyQuantity (%d) exceeds perSessionLimit (%d) — the API may reject the request",
			buyQuantity, packInfo.PerSessionLimit)
	}

	// Build the buy request with the dynamically resolved priceConfigId.
	packReq := handlers.BuyPackRequest{
		PackMasterID:  buyPackMasterID,
		Quantity:      buyQuantity,
		PriceConfigID: packInfo.PriceConfigID,
	}

	apiURL := cfg.SpinnerBFFURL
	t.Logf("🎯 Spinner BFF:  %s", apiURL)
	t.Logf("🌍 Env:          %s", cfg.Env)
	t.Logf("👤 Active users: %d", len(validTokens))

	// -------- REPORTER SETUP --------
	packCfg := reporter.BuyPackConfig{
		PackMasterID:  buyPackMasterID,
		Quantity:      buyQuantity,
		PriceConfigID: packInfo.PriceConfigID,
		SpinnerBFFURL: apiURL,
	}
	rep := reporter.NewBuyReveal(cfg.Env, packCfg)
	outDir := filepath.Join(config.Root(), "reports")

	// -------- CONCURRENCY SETUP --------
	results := make(chan jobResult, len(validTokens))
	var wg sync.WaitGroup

	startTime := time.Now()

	// Launch one goroutine per user. Each goroutine buys the pack and then
	// reveals ALL returned pack IDs concurrently within that same goroutine.
	for _, tok := range validTokens {
		wg.Add(1)
		go func(ut handlers.UserToken) {
			defer wg.Done()
			results <- buyAndRevealWorker(apiURL, ut, packReq)
		}(tok)
	}

	// Wait for all goroutines, then close the results channel.
	wg.Wait()
	close(results)

	totalTime := time.Since(startTime)

	// -------- COLLECT & AGGREGATE RESULTS --------
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

		// Aggregate NFT data and latency across all reveals for this user.
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

		// Log and mark test failures.
		if !br.Success {
			t.Errorf("❌ BUY  FAILED | User=%s | Status=%d | Error=%s",
				br.Email, br.Status, br.ErrorMsg)
		} else if !allRevealsOK {
			t.Errorf("❌ REVEAL FAILED | User=%s | Errors=%s",
				br.Email, row.RevError)
		} else {
			t.Logf("✅ OK | User=%-30s | Packs=%d | NFTs=%d | TotalValue=%.2f",
				br.Email, len(br.UserPackIDs), row.NFTCount, row.TotalValue)
		}
	}

	// -------- FINALIZE & SAVE REPORT --------
	rep.FinalizeBuyReveal(totalTime)

	jsonPath, err := rep.WriteBuyRevealJSON(outDir)
	if err != nil {
		t.Logf("⚠️  could not write JSON report: %v", err)
	} else {
		t.Logf("📄 JSON report: %s", jsonPath)
	}

	htmlPath, err := rep.WriteBuyRevealHTML(outDir)
	if err != nil {
		t.Logf("⚠️  could not write HTML report: %v", err)
	} else {
		t.Logf("🌐 HTML report: %s", htmlPath)
	}

	// -------- CONSOLE SUMMARY --------
	s := rep.Summary
	t.Logf("────────────── BUY & REVEAL TEST SUMMARY ──────────────")
	t.Logf("Environment:        %s", cfg.Env)
	t.Logf("Pack:               %s (%s)", packInfo.PackName, buyPackMasterID)
	t.Logf("Price Config ID:    %s (resolved at runtime)", packInfo.PriceConfigID)
	t.Logf("Total Users:        %d", s.TotalUsers)
	t.Logf("Buy  Success:       %d / %d", s.BuySuccessCount, s.TotalUsers)
	t.Logf("Reveal Success:     %d / %d", s.RevealSuccessCount, s.BuySuccessCount)
	t.Logf("Total NFTs:         %d", s.TotalNFTs)
	t.Logf("Total NFT Value:    %.2f", s.TotalNFTValue)
	t.Logf("Avg NFT Value:      %.2f", s.AvgNFTValue)
	t.Logf("Avg Buy Latency:    %d ms", s.AvgBuyLatencyMs)
	t.Logf("Avg Reveal Latency: %d ms", s.AvgRevLatencyMs)
	t.Logf("Total Wall Time:    %d ms", s.TotalTimeMs)
	t.Logf("────────────────────────────────────────────────────────")
}

// ──────────────────────────────────────────────────────────────────────────────
// buyAndRevealWorker
//
// For a single user:
//  1. Buy the pack (may return N pack IDs when quantity > 1)
//  2. Reveal EVERY returned userPackId concurrently (inner goroutine pool)
//
// ──────────────────────────────────────────────────────────────────────────────
func buyAndRevealWorker(spinnerBFFURL string, tok handlers.UserToken, req handlers.BuyPackRequest) jobResult {
	// Step 1 — Buy pack
	buyRes := handlers.BuyPack(spinnerBFFURL, tok.AccessToken, req, tok.Email)
	res := jobResult{buyResult: buyRes}

	if !enableReveal || !buyRes.Success || len(buyRes.UserPackIDs) == 0 {
		return res
	}

	// Short delay so the backend can register the purchase before reveal.
	time.Sleep(500 * time.Millisecond)

	// Step 2 — Reveal ALL pack IDs concurrently.
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
		res.revealResults = append(res.revealResults, rr)
	}

	return res
}
