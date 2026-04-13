// Package suite provides the shared BaseSuite foundation for all test suites.
// Every domain test package imports this and embeds BaseSuite.
package suite

import (
	"path/filepath"
	"testing"

	"github.com/AvinGupta27/code-go-automation/config"
	"github.com/AvinGupta27/code-go-automation/reporter"
	"github.com/stretchr/testify/suite"
)

// BaseSuite is the shared foundation for every test suite.
// Embed it in your suite struct to get config, lifecycle hooks,
// report helpers, and combined run HTML for free.
type BaseSuite struct {
	suite.Suite
	Cfg          *config.Config
	suiteReports []*reporter.Report
}

// SetupSuite runs once before all tests — loads .env → Config.
func (s *BaseSuite) SetupSuite() {
	cfg, err := config.Load()
	s.Require().NoError(err, "failed to load config")
	s.Cfg = cfg
	s.T().Logf("SetupSuite: env=%s  SpinnerBFF=%s  FcBFF=%s",
		cfg.Env, cfg.SpinnerBFFURL, cfg.FcBFFURL)
}

// TearDownSuite writes the combined run HTML after all tests finish.
func (s *BaseSuite) TearDownSuite() {
	if len(s.suiteReports) > 0 {
		path, err := reporter.WriteRunHTML(ReportDir(), s.Cfg.Env, s.suiteReports...)
		if err != nil {
			s.T().Logf("run report write error: %v", err)
		} else {
			s.T().Logf("Run report (HTML): %s", path)
		}
	}
	s.T().Log("TearDownSuite: all tests finished")
}

// SetupTest runs before each individual test.
func (s *BaseSuite) SetupTest() {
	s.T().Logf("SetupTest: starting %s", s.T().Name())
}

// TearDownTest runs after each individual test.
func (s *BaseSuite) TearDownTest() {
	s.T().Logf("TearDownTest: finished %s", s.T().Name())
}

// StoreSuiteReport appends a finished Report for inclusion in the run HTML.
// Call at the end of every Test* method after WriteReport.
func (s *BaseSuite) StoreSuiteReport(rep *reporter.Report) {
	s.suiteReports = append(s.suiteReports, rep)
}

// WriteReport writes the JSON report for rep and logs the path.
func (s *BaseSuite) WriteReport(rep *reporter.Report) {
	jsonPath, err := reporter.WriteJSON(rep, ReportDir())
	if err != nil {
		s.T().Logf("report write error: %v", err)
		return
	}
	s.T().Logf("JSON report: %s", jsonPath)
}

// LogSummary logs a compact summary of the finished report.
func (s *BaseSuite) LogSummary(rep *reporter.Report) {
	sm := rep.Summary
	s.T().Logf("────────── %s SUMMARY ──────────", rep.Name)
	s.T().Logf("Total: %d  Pass: %d  Fail: %d  Warn: %d  Skip: %d",
		sm.Total, sm.PassCount, sm.FailCount, sm.WarnCount, sm.SkipCount)
	s.T().Logf("Success rate: %.1f%%", sm.SuccessRate)
	if sm.Latency.Count > 0 {
		s.T().Logf("Latency — p50:%dms  p95:%dms  p99:%dms  max:%dms",
			sm.Latency.P50Ms, sm.Latency.P95Ms, sm.Latency.P99Ms, sm.Latency.MaxMs)
	}
	s.T().Logf("Wall time: %d ms", sm.WallTimeMs)
	s.T().Logf("────────────────────────────────────────")
}

// ReportDir returns the absolute path to the reports/ output directory.
func ReportDir() string {
	return filepath.Join(config.Root(), "reports")
}

// Run is a convenience wrapper around suite.Run.
func Run(t *testing.T, s suite.TestingSuite) {
	suite.Run(t, s)
}
