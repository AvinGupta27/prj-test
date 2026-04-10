package constants

// -------- Spinner BFF Endpoints --------
const (
	PackReveal    = "/api/v1/userpacks/reveal"
	UserPacksList = "/api/v1/userpacks/"
	PacksBuy      = "/api/v1/packsmaster/buy"
	PacksEnriched = "/api/v2/packs/enriched"
)

// -------- FC BFF Auth Endpoints --------
const (
	AuthOTPLogin    = "/auth/otp/login"
	AuthOTPVerify   = "/auth/otp/verify"
	AuthSSOGenerate = "/auth/sso/generate"
	UserWallet      = "/v1/userWallet"
)

// -------- Spinner BFF Endpoints (continued) --------
const (
	EventGroupsFindAll = "/api/v1/eventGroups/findAll"
)

// -------- Proxy Endpoints --------
const (
	EventGroupsSupply = "/superteamUserService/api/v1/events/available-supply"
)