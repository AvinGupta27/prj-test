package pack_test

import (
	"testing"

	"github.com/AvinGupta27/code-go-automation/client"
	"github.com/AvinGupta27/code-go-automation/config"
	"github.com/AvinGupta27/code-go-automation/suite"
)

// ──────────────────────────────────────────────────────────────────────────────
// CONFIGURATION — edit data/pack_buy_reveal.json or data/pack_create.json
// ──────────────────────────────────────────────────────────────────────────────

type packConfig struct {
	PackMasterID  string `json:"packMasterID"`
	Quantity      int    `json:"quantity"`
	EnableReveal  bool   `json:"enableReveal"`
	RevealWorkers int    `json:"revealWorkers"`
	UsersFile     string `json:"usersFile"`
}

type createPackConfig struct {
	PackCount            int `json:"packCount"`
	SlotCount            int `json:"slotCount"`
	FMVDistributionCount int `json:"fmvDistributionCount"`
	EventGroupsPage      int `json:"eventGroupsPage"`
	EventGroupsLimit     int `json:"eventGroupsLimit"`
}

// ──────────────────────────────────────────────────────────────────────────────
// SUITE
// ──────────────────────────────────────────────────────────────────────────────

type PackSuite struct {
	suite.BaseSuite
	cfg         packConfig
	createCfg   createPackConfig
	validTokens []client.UserToken
	eventSlugs  []string // event group IDs with availableSupply > 0, for TestCreatePack
}

func TestPackSuite(t *testing.T) {
	suite.Run(t, new(PackSuite))
}

func (s *PackSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	err := config.LoadJSON(config.DataPath("pack_buy_reveal.json"), &s.cfg)
	s.Require().NoError(err, "failed to load pack_buy_reveal.json")
	s.T().Logf("Pack config: packID=%s  qty=%d  reveal=%v  workers=%d",
		s.cfg.PackMasterID, s.cfg.Quantity, s.cfg.EnableReveal, s.cfg.RevealWorkers)

	err = config.LoadJSON(config.DataPath("pack_create.json"), &s.createCfg)
	s.Require().NoError(err, "failed to load pack_create.json")
	s.T().Logf("CreatePack config: packCount=%d  slotCount=%d  fmvDistributions=%d  page=%d  limit=%d",
		s.createCfg.PackCount, s.createCfg.SlotCount, s.createCfg.FMVDistributionCount,
		s.createCfg.EventGroupsPage, s.createCfg.EventGroupsLimit)

	// Fetch event groups using a single auth — avoids authenticating all users
	// just for the supply lookup used by TestCreatePack.
	s.fetchEventGroupIDs()
}

// fetchEventGroupIDs authenticates one user, calls findAll, and stores IDs of
// event groups that have available supply. Used only by TestCreatePack.
func (s *PackSuite) fetchEventGroupIDs() {
	users, err := client.LoadUsers(config.DataPath(s.cfg.UsersFile))
	if err != nil || len(users) == 0 {
		s.T().Logf("WARN | could not load users for event group fetch: %v — TestCreatePack will be skipped", err)
		return
	}
	auth, err := client.GenerateTokens(s.Cfg.FcBFFURL, users[0])
	if err != nil {
		s.T().Logf("WARN | single-user auth failed (%s): %v — TestCreatePack will be skipped", users[0].Email, err)
		return
	}
	s.T().Logf("Fetching event groups for create-pack (page=%d limit=%d) …",
		s.createCfg.EventGroupsPage, s.createCfg.EventGroupsLimit)
	groups, err := client.FetchEventGroups(
		s.Cfg.SpinnerBFFURL, auth.AccessToken,
		s.createCfg.EventGroupsPage, s.createCfg.EventGroupsLimit,
	)
	if err != nil {
		s.T().Logf("WARN | FetchEventGroups failed — TestCreatePack will be skipped: %v", err)
		return
	}
	ids := make([]string, 0, len(groups))
	for _, g := range groups {
		if g.AvailableSupply > 0 {
			ids = append(ids, g.ID)
		} else {
			s.T().Logf("SKIP supply | slug=%-40s | availableSupply=0", g.Slug)
		}
	}
	s.T().Logf("%d / %d event group(s) have available supply", len(ids), len(groups))
	s.eventSlugs = ids
}

// authenticateUsers does the full parallel auth for all users in users.json.
// Called lazily from TestReveal and TestBuyAndReveal — not in SetupSuite.
func (s *PackSuite) authenticateUsers() {
	if len(s.validTokens) > 0 {
		return
	}
	users, err := client.LoadUsers(config.DataPath(s.cfg.UsersFile))
	s.Require().NoError(err, "failed to load users from %s", s.cfg.UsersFile)
	s.Require().NotEmpty(users, "%s is empty", s.cfg.UsersFile)
	s.T().Logf("Loaded %d user account(s)", len(users))

	s.T().Log("Authenticating all users in parallel …")
	tokens := client.GenerateAllTokens(s.Cfg.FcBFFURL, users)
	for _, tok := range tokens {
		if tok.Err != nil {
			s.T().Logf("Auth SKIPPED for %s: %v", tok.Email, tok.Err)
		} else {
			s.validTokens = append(s.validTokens, tok)
			s.T().Logf("Auth OK for %s", tok.Email)
		}
	}
	s.Require().NotEmpty(s.validTokens, "all authentications failed")
}
