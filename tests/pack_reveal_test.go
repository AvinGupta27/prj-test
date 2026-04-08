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

// Workers is the number of concurrent goroutines making reveal API calls per user.
const Workers = 5

// -------- STRUCTS --------

// result holds the raw outcome of a single reveal call (internal to test).
type result struct {
	id        string
	email     string
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

	// Load all test user accounts from users.json.
	users, err := handlers.LoadUsers(config.DataPath("users.json"))
	if err != nil {
		t.Fatalf("failed to load users from users.json: %v", err)
	}
	if len(users) == 0 {
		t.Fatal("users.json is empty — add at least one account")
	}
	t.Logf("Loaded %d user account(s)", len(users))

	// Authenticate all users in parallel.
	t.Log("Authenticating all users in parallel …")
	tokens := handlers.GenerateAllTokens(cfg.FcBFFURL, users)

	// Partition tokens into valid / failed.
	var validTokens []handlers.UserToken
	for _, tok := range tokens {
		if tok.Err != nil {
			t.Errorf("Auth FAILED for %s: %v", tok.Email, tok.Err)
		} else {
			validTokens = append(validTokens, tok)
			t.Logf("Auth OK for %s", tok.Email)
		}
	}
	if len(validTokens) == 0 {
		t.Fatal("all authentications failed — cannot proceed")
	}

	t.Logf("Environment: %s", cfg.Env)
	t.Logf("Spinner BFF: %s", cfg.SpinnerBFFURL)
	t.Logf("Using worker pool size per user: %d", Workers)

	rep := reporter.New(cfg.Env, "Reveal Packs API", Workers)
	outDir := filepath.Join(config.Root(), "reports")

	startTime := time.Now()

	// Collect all reveal jobs across all users.
	type revealJob struct {
		token handlers.UserToken
		id    string
	}

	// First, fetch unrevealed pack IDs for every authenticated user.
	var allJobs []revealJob
	for _, tok := range validTokens {
		ids, err := handlers.FetchUnrevealedPackIDs(cfg.SpinnerBFFURL, tok.AccessToken)
		if err != nil {
			t.Errorf("failed fetching unrevealed packs for %s: %v", tok.Email, err)
			continue
		}
		t.Logf("Found %d unrevealed pack(s) for %s", len(ids), tok.Email)
		for _, id := range ids {
			allJobs = append(allJobs, revealJob{token: tok, id: id})
		}
	}

	if len(allJobs) == 0 {
		t.Skip("No unrevealed packs found for any user. Skip.")
	}

	t.Logf("Total unrevealed packs across all users: %d", len(allJobs))

	results := make(chan result, len(allJobs))
	jobs := make(chan revealJob, len(allJobs))

	var wg sync.WaitGroup

	// Launch generic worker pool.
	for w := 1; w <= Workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for job := range jobs {
				rr := handlers.RevealPack(cfg.SpinnerBFFURL, job.token.AccessToken, job.id, job.token.Email)
				results <- result{
					id:        job.id,
					email:     job.token.Email,
					success:   rr.Success,
					status:    rr.Status,
					latencyMs: rr.LatencyMs,
					errorMsg:  rr.ErrorMsg,
				}
			}
		}(w)
	}

	for _, job := range allJobs {
		jobs <- job
	}
	close(jobs)

	// Wait in background and close results.
	go func() {
		wg.Wait()
		close(results)
	}()

	var passCount, failCount int
	userErrors := make(map[string][]string)

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
			userErrors[res.email] = append(userErrors[res.email], fmt.Sprintf("[%s] %s", res.id, res.errorMsg))
			t.Errorf("FAIL | User=%-30s | ID=%-30s | Status=%d | Error=%s",
				res.email, res.id, res.status, res.errorMsg)
		} else {
			passCount++
			t.Logf("OK   | User=%-30s | ID=%-30s | Status=%d | %dms",
				res.email, res.id, res.status, res.latencyMs)
		}
	}

	totalTime := time.Since(startTime)
	rep.Finalize(totalTime)

	jsonPath, err := rep.WriteJSON(outDir)
	if err != nil {
		t.Logf("could not write JSON report: %v", err)
	} else {
		t.Logf("JSON report: %s", jsonPath)
	}

	htmlPath, err := rep.WriteHTML(outDir)
	if err != nil {
		t.Logf("could not write HTML report: %v", err)
	} else {
		t.Logf("HTML report: %s", htmlPath)
	}

	// Per-user failure summary.
	if len(userErrors) > 0 {
		t.Logf("────────────── PER-USER FAILURES ──────────────")
		for email, errs := range userErrors {
			t.Logf("User: %s | Failures: %s", email, strings.Join(errs, " | "))
		}
	}

	t.Logf("────────────── TEST SUMMARY ──────────────")
	t.Logf("Total users:    %d", len(validTokens))
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
