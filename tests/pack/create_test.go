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

type createPackRow struct {
	PackNum     int // 1-based index within the run
	PackID      string
	PackSlug    string
	PackStatus  string
	SlotCount   int
	Price       float64
	StrikePrice float64
	BuyLimit    int
	HTTPStatus  int
	Dur         time.Duration
	RawResponse string
	Failure     reporter.FailureDetail
}

func (r createPackRow) RowStatus() reporter.Status {
	if r.Failure.IsZero() && (r.HTTPStatus == 200 || r.HTTPStatus == 201) {
		return reporter.StatusPass
	}
	return reporter.StatusFail
}
func (r createPackRow) RowLatency() time.Duration { return r.Dur }
func (r createPackRow) RowLabel() string          { return fmt.Sprintf("pack #%d", r.PackNum) }
func (r createPackRow) RowColumns() []reporter.Column {
	slug := r.PackSlug
	if len(slug) > 28 {
		slug = slug[:28] + "…"
	}
	return []reporter.Column{
		{Header: "Pack ID", Value: r.PackID},
		{Header: "Slug", Value: slug},
		{Header: "Status", Value: r.PackStatus},
		{Header: "Slots", Value: fmt.Sprintf("%d", r.SlotCount)},
		{Header: "Price", Value: fmt.Sprintf("$%.2f", r.Price)},
		{Header: "Strike", Value: fmt.Sprintf("$%.2f", r.StrikePrice)},
		{Header: "Buy Limit", Value: fmt.Sprintf("%d", r.BuyLimit)},
		{Header: "HTTP", Value: fmt.Sprintf("%d", r.HTTPStatus)},
		{Header: "Latency", Value: fmt.Sprintf("%d ms", r.Dur.Milliseconds())},
	}
}
func (r createPackRow) RowDetails() []reporter.Detail {
	details := []reporter.Detail{
		{Key: "Full Slug", Value: r.PackSlug, Mono: true},
		{Key: "Response", Value: r.RawResponse, Mono: true},
	}
	return append(details, r.Failure.ToDetails()...)
}

// ──────────────────────────────────────────────────────────────────────────────
// TestCreatePack
// ──────────────────────────────────────────────────────────────────────────────

func (s *PackSuite) TestCreatePack() {
	cfg := s.Cfg

	if len(s.eventSlugs) == 0 {
		s.T().Skip("No event groups with available supply — skipping pack creation")
	}

	packCount := s.createCfg.PackCount
	if packCount < 1 {
		packCount = 1
	}

	run := reporter.NewRunner[createPackRow]("Create Pack", reporter.NewMeta(
		cfg.Env, cfg.ProxyURL, "pack_create.json",
		"admin", "pack", "create",
	))
	run.Annotate("pack_count", fmt.Sprintf("%d", packCount))
	run.Annotate("slot_count", fmt.Sprintf("%d", s.createCfg.SlotCount))
	run.Annotate("fmv_distribution_count", fmt.Sprintf("%d", s.createCfg.FMVDistributionCount))
	run.Annotate("eligible_event_groups", fmt.Sprintf("%d", len(s.eventSlugs)))

	rowCh := make(chan createPackRow, packCount)

	var wg sync.WaitGroup
	for i := 0; i < packCount; i++ {
		wg.Add(1)
		go func(num int) {
			defer wg.Done()

			payload := client.BuildPackPayload(s.createCfg.SlotCount, s.createCfg.FMVDistributionCount, s.eventSlugs)

			s.T().Logf("pack #%d | slug=%s | slots=%d", num, payload.Slug, len(payload.SlotConfigs))
			for si, slot := range payload.SlotConfigs {
				s.T().Logf("  pack #%d slot[%d]: rule=%s  weight=%d%%", num, si, slot.SelectionRule, slot.WeightPercent)
			}

			start := time.Now()
			result := client.CreatePack(cfg.ProxyURL, payload)
			dur := time.Since(start)

			row := createPackRow{
				PackNum:     num,
				PackID:      result.PackID,
				PackSlug:    result.PackSlug,
				PackStatus:  result.PackStatus,
				SlotCount:   result.SlotCount,
				Price:       result.Price,
				StrikePrice: result.StrikePrice,
				BuyLimit:    payload.TotalUserBuyLimit,
				HTTPStatus:  result.Status,
				Dur:         dur,
				RawResponse: result.RawResponse,
			}

			if !result.Success {
				row.Failure = reporter.FailureDetail{
					RequestMethod:  "POST",
					RequestURL:     cfg.ProxyURL + constants.AdminPackCreate,
					ResponseStatus: result.Status,
					ResponseBody:   result.ErrorMsg,
					ErrorChain:     result.ErrorMsg,
					OccurredAt:     time.Now(),
				}
				s.T().Errorf("FAIL | pack #%d | HTTP=%d | %s", num, result.Status, result.ErrorMsg)
			} else {
				s.T().Logf("OK   | pack #%d | PackID=%s | Slug=%s | price=$%.2f | buyLimit=%d | %d ms",
					num, result.PackID, result.PackSlug, result.Price, payload.TotalUserBuyLimit, dur.Milliseconds())
			}

			rowCh <- row
		}(i + 1)
	}

	wg.Wait()
	close(rowCh)

	var failCount int
	for row := range rowCh {
		run.Add(row)
		if row.RowStatus() == reporter.StatusFail {
			failCount++
		}
	}

	rep := run.Finish()
	s.WriteReport(rep)
	s.StoreSuiteReport(rep)
	s.LogSummary(rep)

	if failCount > 0 {
		s.Fail(fmt.Sprintf("%d pack creation failure(s)", failCount))
	}
}
