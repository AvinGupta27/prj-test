package reporter

import (
	"os/exec"
	"strings"
	"time"
)

// RunMeta captures everything about the context of a test run.
// Fields that cannot be resolved are left as empty strings — never fatal.
type RunMeta struct {
	// Supplied by the caller
	Env        string   // e.g. "preprod", "prod"
	BaseURL    string   // primary service URL under test
	ConfigFile string   // e.g. "config_supply.json" — which runtime config was used
	Tags       []string // arbitrary labels e.g. ["pack", "reveal", "regression"]

	// Resolved automatically at construction time
	GitBranch string
	GitCommit string // short SHA
	StartedAt time.Time
	FinishedAt time.Time
}

// NewMeta builds a RunMeta and resolves git context at call time.
func NewMeta(env, baseURL, configFile string, tags ...string) RunMeta {
	return RunMeta{
		Env:        env,
		BaseURL:    baseURL,
		ConfigFile: configFile,
		Tags:       tags,
		GitBranch:  gitOutput("git", "rev-parse", "--abbrev-ref", "HEAD"),
		GitCommit:  gitOutput("git", "rev-parse", "--short", "HEAD"),
		StartedAt:  time.Now(),
	}
}

// Duration returns wall-clock time between start and finish.
func (m RunMeta) Duration() time.Duration {
	if m.FinishedAt.IsZero() {
		return 0
	}
	return m.FinishedAt.Sub(m.StartedAt)
}

// gitOutput runs a git command and returns trimmed stdout.
// Returns empty string on any error so missing git never breaks a test run.
func gitOutput(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
