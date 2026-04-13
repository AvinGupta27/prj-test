package client

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
	PackMasterID    string
	PackName        string
	PriceConfigID   string
	PriceCurrency   string
	PriceValue      float64
	PerSessionLimit int
	TotalPackCount  int
	PacksSold       int
	PackOpeningDate string
}

// BuyPackRequest is the payload for buying a pack.
type BuyPackRequest struct {
	PackMasterID  string `json:"packMasterId"`
	Quantity      int    `json:"quantity"`
	PriceConfigID string `json:"priceConfigId"`
}

// BuyPackResult captures the outcome of a buy API call.
type BuyPackResult struct {
	Email       string
	UserPackIDs []string
	Success     bool
	Status      int
	LatencyMs   int64
	ErrorMsg    string
}

// RevealNFT holds data for a single NFT revealed from a pack.
type RevealNFT struct {
	NFTTokenID string  `json:"nftTokenId"`
	CardName   string  `json:"cardName"`
	Rarity     string  `json:"rarity"`
	Value      float64 `json:"value"`
}

// RevealResult captures the outcome of a reveal API call.
type RevealResult struct {
	Email      string
	UserPackID string
	NFTs       []RevealNFT
	TotalValue float64
	NFTCount   int
	Success    bool
	Status     int
	LatencyMs  int64
	ErrorMsg   string
}

// -------- INTERNAL RESPONSE SHAPES --------

type enrichedAPIResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
}

type enrichedDataShape struct {
	HeroSection     []enrichedPack `json:"heroSection"`
	Packs           []enrichedPack `json:"packs"`
	PackOpeningDate string         `json:"packOpeningDate"`
}

type enrichedPack struct {
	ID              string                       `json:"_id"`
	PackName        string                       `json:"packName"`
	PerSessionLimit int                          `json:"perSessionLimit"`
	TotalPackCount  int                          `json:"totalPackCount"`
	PacksSold       int                          `json:"packsSold"`
	PriceConfig     map[string]map[string]float64 `json:"priceConfig"`
}

type buyPackAPIResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Message         string `json:"message"`
		PackOpeningDate string `json:"packOpeningDate"`
		Success         bool   `json:"success"`
		UserPacks       []struct {
			UserPackID string `json:"userPackId"`
		} `json:"userPacks"`
	} `json:"data"`
}

type revealAPIResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
}

type revealDataShape struct {
	UserPackID string `json:"userPackId"`
	Status     string `json:"status"`
	Cards      []struct {
		NFTTokenID string  `json:"nftTokenId"`
		Name       string  `json:"name"`
		CardName   string  `json:"cardName"`
		Rarity     string  `json:"rarity"`
		Value      float64 `json:"value"`
		Price      float64 `json:"price"`
	} `json:"cards"`
}

type userPackItem struct {
	UserPackID string `json:"userPackId"`
	Status     string `json:"status"`
}

type userPacksWrapperData struct {
	UserPacks []userPackItem `json:"userPacks"`
}

type userPacksListResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
}

// -------- PUBLIC API --------

// FetchPackInfo calls the enriched packs endpoint and returns runtime config
// for the given packMasterID.
func FetchPackInfo(spinnerBFFURL, token, packMasterID string) (*PackInfo, error) {
	const countryCode = "IN"
	c := resty.New().SetTimeout(10 * time.Second)

	var apiResp enrichedAPIResponse
	resp, err := c.R().
		SetHeader("Authorization", token).
		SetHeader("Content-Type", "application/json").
		SetHeader("source", "WEB").
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

	var data enrichedDataShape
	if err := json.Unmarshal(apiResp.Data, &data); err != nil {
		return nil, fmt.Errorf("FetchPackInfo: parse inner data: %w", err)
	}

	all := make([]enrichedPack, 0, len(data.HeroSection)+len(data.Packs))
	all = append(all, data.HeroSection...)
	all = append(all, data.Packs...)

	seen := make(map[string]bool)
	var unique []enrichedPack
	for _, p := range all {
		if !seen[p.ID] {
			seen[p.ID] = true
			unique = append(unique, p)
		}
	}

	for _, p := range unique {
		if p.ID != packMasterID {
			continue
		}
		if len(p.PriceConfig) == 0 {
			return nil, fmt.Errorf("FetchPackInfo: pack %s has no priceConfig entries", packMasterID)
		}
		configKeys := make([]string, 0, len(p.PriceConfig))
		for k := range p.PriceConfig {
			configKeys = append(configKeys, k)
		}
		sort.Strings(configKeys)
		priceConfigID := configKeys[0]

		currencies := p.PriceConfig[priceConfigID]
		if len(currencies) == 0 {
			return nil, fmt.Errorf("FetchPackInfo: pack %s priceConfigID %s has no currency entries", packMasterID, priceConfigID)
		}
		var currency string
		var priceVal float64
		for cur, val := range currencies {
			currency = cur
			priceVal = val
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

// BuyPack purchases a pack for the given user.
func BuyPack(spinnerBFFURL, accessToken string, req BuyPackRequest, email string) BuyPackResult {
	result := BuyPackResult{Email: email}
	c := resty.New().SetTimeout(10 * time.Second)

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		result.ErrorMsg = fmt.Sprintf("failed to marshal request body: %v", err)
		return result
	}

	var apiResp buyPackAPIResponse
	start := time.Now()
	resp, err := c.R().
		SetHeader("Authorization", accessToken).
		SetHeader("Content-Type", "text/plain;charset=UTF-8").
		SetBody(string(bodyBytes)).
		SetResult(&apiResp).
		Post(spinnerBFFURL + constants.PacksBuy)
	result.LatencyMs = time.Since(start).Milliseconds()

	switch {
	case err != nil:
		result.ErrorMsg = err.Error()
	case resp.StatusCode() != 200:
		result.Status = resp.StatusCode()
		result.ErrorMsg = resp.String()
	case !apiResp.Success || !apiResp.Data.Success:
		result.Status = resp.StatusCode()
		result.ErrorMsg = fmt.Sprintf("API returned success=false: %s", resp.String())
	default:
		result.Success = true
		result.Status = resp.StatusCode()
		for _, p := range apiResp.Data.UserPacks {
			result.UserPackIDs = append(result.UserPackIDs, p.UserPackID)
		}
	}
	return result
}

// RevealPack reveals a single purchased pack.
func RevealPack(spinnerBFFURL, accessToken, userPackID, email string) RevealResult {
	result := RevealResult{Email: email, UserPackID: userPackID}
	c := resty.New().SetTimeout(15 * time.Second)

	var apiResp revealAPIResponse
	start := time.Now()
	resp, err := c.R().
		SetHeader("Authorization", accessToken).
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]string{"userPackId": userPackID}).
		SetResult(&apiResp).
		Post(spinnerBFFURL + constants.PackReveal)
	result.LatencyMs = time.Since(start).Milliseconds()

	if err != nil {
		result.ErrorMsg = err.Error()
		return result
	}
	if resp.StatusCode() != 200 {
		result.Status = resp.StatusCode()
		result.ErrorMsg = resp.String()
		return result
	}
	if !apiResp.Success {
		result.Status = resp.StatusCode()
		result.ErrorMsg = fmt.Sprintf("API success=false: %s", resp.String())
		return result
	}

	result.Status = resp.StatusCode()
	result.Success = true

	var shape revealDataShape
	if err := json.Unmarshal(apiResp.Data, &shape); err != nil {
		result.ErrorMsg = fmt.Sprintf("reveal HTTP OK but could not parse NFT payload: %v | raw: %s", err, string(apiResp.Data))
		return result
	}

	for _, card := range shape.Cards {
		v := card.Value
		if v == 0 {
			v = card.Price
		}
		name := card.CardName
		if name == "" {
			name = card.Name
		}
		result.NFTs = append(result.NFTs, RevealNFT{
			NFTTokenID: card.NFTTokenID,
			CardName:   name,
			Rarity:     card.Rarity,
			Value:      v,
		})
		result.TotalValue += v
	}
	result.NFTCount = len(result.NFTs)
	return result
}

// FetchUnrevealedPackIDs returns the userPackId of every UNREVEALED pack for a user.
// It paginates automatically until all pages are consumed.
func FetchUnrevealedPackIDs(spinnerBFFURL, token string) ([]string, error) {
	c := resty.New().SetTimeout(15 * time.Second)

	const (
		pageLimit        = 500
		unrevealedStatus = "UNREVEALED"
	)

	var allIDs []string
	page := 1

	for {
		var body userPacksListResponse
		resp, err := c.R().
			SetHeader("Authorization", token).
			SetHeader("Content-Type", "application/json").
			SetQueryParams(map[string]string{
				"page":   fmt.Sprintf("%d", page),
				"limit":  fmt.Sprintf("%d", pageLimit),
				"status": unrevealedStatus,
				"sortBy": "-purchaseDate",
			}).
			SetResult(&body).
			Get(spinnerBFFURL + constants.UserPacksList)

		if err != nil {
			return nil, fmt.Errorf("FetchUnrevealedPackIDs (page %d): request error: %w", page, err)
		}
		if resp.StatusCode() != 200 {
			return nil, fmt.Errorf("FetchUnrevealedPackIDs (page %d): unexpected status %d: %s",
				page, resp.StatusCode(), resp.String())
		}
		if !body.Success {
			return nil, fmt.Errorf("FetchUnrevealedPackIDs (page %d): API returned success=false: %s",
				page, resp.String())
		}

		packs, err := extractPacks(body.Data)
		if err != nil {
			return nil, fmt.Errorf("FetchUnrevealedPackIDs (page %d): parse error: %w", page, err)
		}
		if len(packs) == 0 {
			break
		}
		for _, p := range packs {
			allIDs = append(allIDs, p.UserPackID)
		}
		if len(packs) < pageLimit {
			break
		}
		page++
	}

	return allIDs, nil
}

// extractPacks handles both API response shapes for the data field:
// array shape [ {...} ] and wrapped object { "userPacks": [...] }.
func extractPacks(raw json.RawMessage) ([]userPackItem, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var asArray []userPackItem
	if err := json.Unmarshal(raw, &asArray); err == nil {
		return asArray, nil
	}
	var asObject userPacksWrapperData
	if err := json.Unmarshal(raw, &asObject); err == nil {
		return asObject.UserPacks, nil
	}
	return nil, fmt.Errorf("unrecognised data shape: %s", string(raw))
}
