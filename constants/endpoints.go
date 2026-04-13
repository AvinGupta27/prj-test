package constants

// Spinner BFF endpoints.
const (
	PackReveal         = "/api/v1/userpacks/reveal"
	UserPacksList      = "/api/v1/userpacks/"
	PacksBuy           = "/api/v1/packsmaster/buy"
	PacksEnriched      = "/api/v2/packs/enriched"
	EventGroupsFindAll = "/api/v1/eventGroups/findAll"
)

// FC BFF endpoints.
const (
	AuthOTPLogin    = "/auth/otp/login"
	AuthOTPVerify   = "/auth/otp/verify"
	AuthSSOGenerate = "/auth/sso/generate"
	UserWallet      = "/v1/userWallet"
)

// Proxy endpoints.
const (
	EventGroupsSupply = "/superteamUserService/api/v1/events/available-supply"
	AdminPackCreate   = "/superteamPacksAdminService/api/v2/admin/packs"
)