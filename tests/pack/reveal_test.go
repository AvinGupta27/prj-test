package pack_test

import (
	"fmt"
	"sync"
	"time"

	"github.com/AvinGupta27/code-go-automation/client"
	"github.com/AvinGupta27/code-go-automation/constants"
	"github.com/AvinGupta27/code-go-automation/reporter"
)

// ──────────────────────────────────────────────────────────────────────────────
// ROW TYPE
// ──────────────────────────────────────────────────────────────────────────────

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
func (r revealRow) RowDetails() []reporter.Detail { return r.Failure.ToDetails() }

// ──────────────────────────────────────────────────────────────────────────────
// TestReveal
// ──────────────────────────────────────────────────────────────────────────────

func (s *PackSuite) TestReveal() {
	s.authenticateUsers()
	cfg := s.Cfg

	run := reporter.NewRunner[revealRow]("Reveal Packs", reporter.NewMeta(
		cfg.Env, cfg.SpinnerBFFURL, "",
		"pack", "reveal",
	))
	run.Annotate("users_authed", fmt.Sprintf("%d", len(s.validTokens)))

	type job struct {
		tok client.UserToken
		id  string
	}
	var allJobs []job
	var totalUnrevealed int

	for _, tok := range s.validTokens {
		ids, err := client.FetchUnrevealedPackIDs(cfg.SpinnerBFFURL, tok.AccessToken)
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

	jobCh := make(chan job, len(allJobs))
	rowCh := make(chan revealRow, len(allJobs))
	var wg sync.WaitGroup

	for w := 0; w < s.cfg.RevealWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobCh {
				start := time.Now()
				rr := client.RevealPack(cfg.SpinnerBFFURL, j.tok.AccessToken, j.id, j.tok.Email)
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
						RequestURL:     cfg.SpinnerBFFURL + constants.PackReveal,
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
	s.WriteReport(rep)
	s.StoreSuiteReport(rep)
	s.LogSummary(rep)

	if failCount > 0 {
		s.Fail(fmt.Sprintf("%d reveal failure(s)", failCount))
	}
}
