package tests

import (
	"testing"

	"github.com/AvinGupta27/code-go-automation/config"
	"github.com/stretchr/testify/suite"
)

// BaseSuite is the shared test suite. Embed it in any test suite struct to get
// config loaded once before all tests and standard before/after hooks.
type BaseSuite struct {
	suite.Suite
	Cfg *config.Config
}

// SetupSuite runs once before all tests in the suite. Loads config.
func (s *BaseSuite) SetupSuite() {
	cfg, err := config.Load()
	s.Require().NoError(err, "failed to load config")
	s.Cfg = cfg
	s.T().Logf("SetupSuite: env=%s  SpinnerBFF=%s  FcBFF=%s",
		cfg.Env, cfg.SpinnerBFFURL, cfg.FcBFFURL)
}

// TearDownSuite runs once after all tests in the suite.
func (s *BaseSuite) TearDownSuite() {
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

// RunSuite is a convenience wrapper so each suite only needs one TestXxx function.
func RunSuite(t *testing.T, s suite.TestingSuite) {
	suite.Run(t, s)
}
