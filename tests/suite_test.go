package tests

import (
	"path/filepath"
	"testing"

	"github.com/AvinGupta27/code-go-automation/config"
	"github.com/AvinGupta27/code-go-automation/reporter"
	"github.com/stretchr/testify/suite"
)

// BaseSuite is the shared foundation for every test suite.
// It loads config once in SetupSuite and collects *reporter.Report values
// from each test so TearDownSuite can write a combined suite HTML.
type BaseSuite struct {
	suite.Suite
	Cfg          *config.Config
	suiteReports []*reporter.Report // accumulated by storeSuiteReport()
}

// SetupSuite runs once before all tests — loads .env → Config.
func (s *BaseSuite) SetupSuite() {
	cfg, err := config.Load()
	s.Require().NoError(err, "failed to load config")
	s.Cfg = cfg
	s.T().Logf("SetupSuite: env=%s  SpinnerBFF=%s  FcBFF=%s",
		cfg.Env, cfg.SpinnerBFFURL, cfg.FcBFFURL)
}

// TearDownSuite runs once after all tests — writes the combined run HTML.
// Always writes one file regardless of how many tests ran.
func (s *BaseSuite) TearDownSuite() {
	if len(s.suiteReports) > 0 {
		path, err := reporter.WriteRunHTML(reportDir(), s.Cfg.Env, s.suiteReports...)
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

// storeSuiteReport appends a finished Report so TearDownSuite can include it
// in the combined suite HTML. Call this at the end of every Test* method.
func (s *BaseSuite) storeSuiteReport(rep *reporter.Report) {
	s.suiteReports = append(s.suiteReports, rep)
}

// reportDir returns the absolute path to the reports/ output directory.
func reportDir() string {
	return filepath.Join(config.Root(), "reports")
}

// RunSuite is a convenience wrapper around suite.Run.
func RunSuite(t *testing.T, s suite.TestingSuite) {
	suite.Run(t, s)
}
