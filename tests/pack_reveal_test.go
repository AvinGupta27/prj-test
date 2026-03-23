package tests

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/AvinGupta27/code-go-automation/config"
	"github.com/AvinGupta27/code-go-automation/handlers" 
	"github.com/AvinGupta27/code-go-automation/reporter"
)

// Workers is the number of concurrent goroutines making reveal API calls.
const Workers = 5

// -------- STRUCTS --------

// result holds the raw outcome of a single reveal call (internal to test).
type result struct {
	id        string
	success   bool
	status    int
	latencyMs int64
	errorMsg  string
}

// -------- TEST --------

func TestRevealAPI(t *testing.T) {
	// -------- SETUP --------
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config load error: %v", err)
	}

	token, _, err := handlers.GenerateTokens(cfg.FcBFFURL, config.DataPath("user.json"))
	if err != nil {
		t.Fatalf("token generation failed: %v", err)
	}

	// Fetch all unrevealed pack IDs from the live API — no ids.json needed.
	ids, err := handlers.FetchUnrevealedPackIDs(cfg.SpinnerBFFURL, token)
	if err != nil {
		t.Fatalf("failed fetching unrevealed pack IDs: %v", err)
	}

	totalIDs := len(ids)
	if totalIDs == 0 {
		t.Skip("No unrevealed packs found for the current user. Skip.")
	}

	t.Logf("Environment: %s", cfg.Env)
	t.Logf("Spinner BFF: %s", cfg.SpinnerBFFURL)
	t.Logf("Found %d unrevealed pack(s)", totalIDs)
	t.Logf("Using worker pool size: %d", Workers)

	// We pass the total ID count and the constant pool size.
	// We'll give it a readable name for the report's endpoint field.
	rep := reporter.New(cfg.Env, "Reveal Packs API", Workers)
	outDir := filepath.Join(config.Root(), "reports")

	results := make(chan result, totalIDs)
	jobs := make(chan string, totalIDs)

	var wg sync.WaitGroup
	startTime := time.Now()

	// Launch generic worker pool
	for w := 1; w <= Workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for id := range jobs {
				// We don't have the user's email easily available here without parsing the JSON again,
				// but that's fine since RevealPack just uses it for reporting. We can pass a dummy string
				// since the old pack_reveal_test didn't need user emails in its simple report.
				rr := handlers.RevealPack(cfg.SpinnerBFFURL, token, id, "user.json")
				results <- result{
					id:        id,
					success:   rr.Success,
					status:    rr.Status,
					latencyMs: rr.LatencyMs,
					errorMsg:  rr.ErrorMsg,
				}
			}
		}(w)
	}

	for _, id := range ids {
		jobs <- id
	}
	close(jobs)

	// Wait in background and close results.
	go func() {
		wg.Wait()
		close(results)
	}()

	var passCount, failCount int

	for res := range results {
		row := reporter.TestResult{
			ID:        res.id,
			Success:   res.success,
			Status:    res.status,
			LatencyMs: res.latencyMs,
			ErrorMsg:  res.errorMsg,
		}
		rep.AddResult(row)

		if !res.success {
			failCount++
			t.Errorf("FAIL | ID=%-30s | Status=%d | Error=%s", res.id, res.status, res.errorMsg)
		} else {
			passCount++
			t.Logf("OK   | ID=%-30s | Status=%d | %dms", res.id, res.status, res.latencyMs)
		}
	}

	totalTime := time.Since(startTime)
	rep.Finalize(totalTime)

	jsonPath, err := rep.WriteJSON(outDir)
	if err != nil {
		t.Logf("could not write JSON report: %v", err)
	} else {
		t.Logf("📄 JSON report: %s", jsonPath)
	}

	htmlPath, err := rep.WriteHTML(outDir)
	if err != nil {
		t.Logf("could not write HTML report: %v", err)
	} else {
		t.Logf("🌐 HTML report: %s", htmlPath)
	}

	t.Logf("────────────── TEST SUMMARY ──────────────")
	t.Logf("Total items:    %d", rep.Summary.TotalRequests)
	t.Logf("Success:        %d", rep.Summary.SuccessCount)
	t.Logf("Failed:         %d", rep.Summary.FailureCount)
	t.Logf("Average latency:%d ms", rep.Summary.AvgLatencyMs)
	t.Logf("Total wall time:%d ms", rep.Summary.TotalTimeMs)
	t.Logf("Concurrency:    %d workers", Workers)
	t.Logf("──────────────────────────────────────────")

	if failCount > 0 {
		t.Errorf("Test completed with %d failure(s)", failCount)
	}
}
