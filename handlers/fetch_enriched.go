package handlers

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/AvinGupta27/code-go-automation/constants"
	"github.com/go-resty/resty/v2"
)

// -------- TYPES --------

// PackInfo holds the fields from the enriched API that the test needs at runtime.
type PackInfo struct {
	PackMasterID    string  // _id of the pack
	PackName        string  // human-readable name
	PriceConfigID   string  // first key of the priceConfig map  (e.g. "6")
	PriceCurrency   string  // first currency key inside priceConfig (e.g. "GC12")
	PriceValue      float64 // price value (e.g. 0.1)
	PerSessionLimit int     // max packs a user can buy per session
	TotalPackCount  int     // total supply
	PacksSold       int     // how many have been sold
	PackOpeningDate string  // ISO-8601 opening date
}

// enrichedAPIResponse is the top-level shape of /api/v2/packs/enriched.
type enrichedAPIResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"` // flexible — handle both shapes
}

// enrichedDataShape is the inner data object of the enriched response.
type enrichedDataShape struct {
	HeroSection     []enrichedPack `json:"heroSection"`
	Packs           []enrichedPack `json:"packs"`
	PackOpeningDate string         `json:"packOpeningDate"`
}

// enrichedPack is the per-pack object returned inside heroSection / packs.
type enrichedPack struct {
	ID              string `json:"_id"`
	PackName        string `json:"packName"`
	PerSessionLimit int    `json:"perSessionLimit"`
	TotalPackCount  int    `json:"totalPackCount"`
	PacksSold       int    `json:"packsSold"`
	// priceConfig is a map whose keys are priceConfigId strings and whose
	// values are maps of currencyCode → price. Example:
	//   { "6": { "GC12": 0.1 } }
	PriceConfig map[string]map[string]float64 `json:"priceConfig"`
}

// -------- PUBLIC API --------

// FetchPackInfo calls the enriched packs endpoint and returns the runtime
// configuration for the given packMasterID.
//
// spinnerBFFURL: base URL e.g. "https://spinnerbff.preprod.munna-bhai.xyz"
// token:         bearer access token (any authenticated user's token works)
// packMasterID:  the _id of the pack to look up
//
// The priceConfigID is the lowest (first sorted) key of the priceConfig map
// for determinism across random Go map iteration order.
func FetchPackInfo(spinnerBFFURL, token, packMasterID string) (*PackInfo, error) {
	const countryCode = "IN" // required query param by the enriched API
	client := resty.New().SetTimeout(10 * time.Second)

	var apiResp enrichedAPIResponse
	resp, err := client.R().
		SetHeader("Authorization", token).
		SetHeader("Content-Type", "application/json").
		SetQueryParam("countryCode", countryCode).
		SetResult(&apiResp).
		Get(spinnerBFFURL + constants.PacksEnriched)

	if err != nil {
		return nil, fmt.Errorf("FetchPackInfo: request error: %w", err)
	}
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("FetchPackInfo: unexpected status %d: %s", resp.StatusCode(), resp.String())
	}
	if !apiResp.Success {
		return nil, fmt.Errorf("FetchPackInfo: API returned success=false: %s", resp.String())
	}

	// Parse the inner data object.
	var data enrichedDataShape
	if err := json.Unmarshal(apiResp.Data, &data); err != nil {
		return nil, fmt.Errorf("FetchPackInfo: parse inner data: %w", err)
	}

	// Search heroSection first, then packs (union of all listed packs).
	all := append(data.HeroSection, data.Packs...)

	// Deduplicate by ID — heroSection and packs may overlap.
	seen := make(map[string]bool)
	var unique []enrichedPack
	for _, p := range all {
		if !seen[p.ID] {
			seen[p.ID] = true
			unique = append(unique, p)
		}
	}

	// Find the target pack.
	for _, p := range unique {
		if p.ID != packMasterID {
			continue
		}

		if len(p.PriceConfig) == 0 {
			return nil, fmt.Errorf("FetchPackInfo: pack %s has no priceConfig entries", packMasterID)
		}

		// Pick the lowest priceConfigId key (sorted for determinism).
		configKeys := make([]string, 0, len(p.PriceConfig))
		for k := range p.PriceConfig {
			configKeys = append(configKeys, k)
		}
		sort.Strings(configKeys)
		priceConfigID := configKeys[0]

		// Extract the first currency + price for info purposes.
		var currency string
		var priceVal float64
		for cur, val := range p.PriceConfig[priceConfigID] {
			currency = cur
			priceVal = val
			break
		}

		return &PackInfo{
			PackMasterID:    p.ID,
			PackName:        p.PackName,
			PriceConfigID:   priceConfigID,
			PriceCurrency:   currency,
			PriceValue:      priceVal,
			PerSessionLimit: p.PerSessionLimit,
			TotalPackCount:  p.TotalPackCount,
			PacksSold:       p.PacksSold,
			PackOpeningDate: data.PackOpeningDate,
		}, nil
	}

	return nil, fmt.Errorf("FetchPackInfo: pack %q not found in enriched response (checked %d packs)", packMasterID, len(unique))
}
