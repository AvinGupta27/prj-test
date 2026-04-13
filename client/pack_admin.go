package client

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/AvinGupta27/code-go-automation/constants"
	"github.com/go-resty/resty/v2"
)

// -------- REQUEST TYPES --------

// SlotConfig describes one slot in a pack's slotConfigs array.
// Exactly one of WeightedMFTs / EligibleMFTs is populated depending on SelectionRule.
type SlotConfig struct {
	WeightPercent int           `json:"weightPercent"`
	SelectionRule string        `json:"selectionRule"`
	IsTradable    bool          `json:"isTradable"`
	WeightedMFTs  []WeightedMFT `json:"weightedMFTs,omitempty"`
	EligibleMFTs  []string      `json:"eligibleMFTs,omitempty"`
}

// WeightedMFT is an event-group ID + weight pair used in weighted slots.
type WeightedMFT struct {
	Slug   string `json:"slug"`
	Weight int    `json:"weight"`
}

// FMVDistribution is one entry in the fmvConfig distribution array.
// FMV is a dollar value — the service multiplies by 100 to get FC gems.
type FMVDistribution struct {
	Probability int     `json:"probability"`
	FMV         float64 `json:"fmv"`
}

// FMVConfig is the fmvConfig block in the pack create payload.
type FMVConfig struct {
	BatchSize      int               `json:"batchSize"`
	NumberOfQueues int               `json:"numberOfQueues"`
	Distribution   []FMVDistribution `json:"distribution"`
}

// CreatePackPayload is the full body sent to the admin pack-create endpoint.
// PriceConfig and FMV values are dollar amounts — the service multiplies by 100
// internally to get FC gem values shown on the frontend.
type CreatePackPayload struct {
	PackName                    string                        `json:"packName"`
	Slug                        string                        `json:"slug"`
	TotalUserBuyLimit           int                           `json:"totalUserBuyLimit"`
	PerSessionLimit             int                           `json:"perSessionLimit"`
	TotalPackCount              int                           `json:"totalPackCount"`
	SegmentConfigs              map[string]interface{}        `json:"segmentConfigs"`
	LockSegments                []interface{}                 `json:"lockSegments"`
	IneligibleSegments          []interface{}                 `json:"ineligibleSegments"`
	ShouldShowToEligibleSegment bool                          `json:"shouldShowToEligibleSegment"`
	Tags                        []string                      `json:"tags"`
	Description                 string                        `json:"description"`
	ShortDescription            string                        `json:"shortDescription"`
	ImageURLs                   map[string]string             `json:"imageURLs"`
	AdditionalInfo              map[string]interface{}        `json:"additionalInfo"`
	FMVConfig                   FMVConfig                     `json:"fmvConfig"`
	PriorityOrder               int                           `json:"priorityOrder"`
	SlotConfigs                 []SlotConfig                  `json:"slotConfigs"`
	PriceConfig                 map[string]map[string]float64 `json:"priceConfig"`
	SaleStartTime               string                        `json:"saleStartTime"`
	SaleEndTime                 string                        `json:"saleEndTime"`
}

// -------- RESULT TYPE --------

// CreatePackResult holds the outcome of a single CreatePack call.
type CreatePackResult struct {
	Status      int
	Success     bool
	PackID      string
	PackSlug    string
	PackName    string
	PackStatus  string
	SlotCount   int
	Price       float64
	StrikePrice float64
	RawResponse string
	ErrorMsg    string
}

// -------- PAYLOAD BUILDER --------

// BuildPackPayload constructs a randomised CreatePackPayload.
//
// slotCount:            how many slotConfigs to generate (≥ 1)
// fmvDistributionCount: how many FMV tiers to generate (≥ 1)
// eventGroupIDs:        event group _id values from the supply API;
//
//	each slot uses a distinct ID — no ID is reused within one pack.
//
// Money rules (dollar values, service multiplies ×100 for FC gems):
//   - strikePrice: random in [0.10, 2.00] USD
//   - price:       strikePrice × 0.67, min $0.01
//   - FMV per tier: random in [0.01, price] USD
//   - buyLimit:    random in [1, 50], same for TotalUserBuyLimit and PerSessionLimit
func BuildPackPayload(slotCount, fmvDistributionCount int, eventGroupIDs []string) CreatePackPayload {
	if slotCount < 1 {
		slotCount = 1
	}
	if fmvDistributionCount < 1 {
		fmvDistributionCount = 1
	}

	now := time.Now()
	rng := rand.New(rand.NewSource(now.UnixNano()))

	// ── Price ─────────────────────────────────────────────────────────────────
	strikePrice := roundTo2DP(0.10 + rng.Float64()*1.90)
	price := roundTo2DP(strikePrice * 0.67)
	if price < 0.01 {
		price = 0.01
	}

	priceConfigID := fmt.Sprintf("%d", 1+rng.Intn(20))
	buyLimit := 1 + rng.Intn(50)

	// ── FMV config ─────────────────────────────────────────────────────────────
	batchSize := 10 + rng.Intn(141)
	numberOfQueues := 1 + rng.Intn(5)

	baseProbability := 100 / fmvDistributionCount
	probRemainder := 100 - baseProbability*fmvDistributionCount

	distributions := make([]FMVDistribution, fmvDistributionCount)
	for i := 0; i < fmvDistributionCount; i++ {
		prob := baseProbability
		if i == fmvDistributionCount-1 {
			prob += probRemainder
		}
		fmv := roundTo2DP(0.01 + rng.Float64()*(price-0.01))
		if fmv < 0.01 {
			fmv = 0.01
		}
		distributions[i] = FMVDistribution{Probability: prob, FMV: fmv}
	}

	// ── Slots — each slot gets a distinct event group ID ─────────────────────
	available := make([]string, len(eventGroupIDs))
	copy(available, eventGroupIDs)
	rng.Shuffle(len(available), func(i, j int) { available[i], available[j] = available[j], available[i] })

	baseWeight := 100 / slotCount
	weightRemainder := 100 - baseWeight*slotCount

	slots := make([]SlotConfig, slotCount)
	idIdx := 0

	nextID := func() string {
		if len(available) == 0 {
			return ""
		}
		id := available[idIdx%len(available)]
		idIdx++
		return id
	}

	for i := 0; i < slotCount; i++ {
		weight := baseWeight
		if i == slotCount-1 {
			weight += weightRemainder
		}
		slot := SlotConfig{WeightPercent: weight, IsTradable: true}
		if rng.Intn(2) == 0 {
			idA := nextID()
			idB := nextID()
			slot.SelectionRule = "weighted"
			slot.WeightedMFTs = []WeightedMFT{
				{Slug: idA, Weight: 50},
				{Slug: idB, Weight: 50},
			}
		} else {
			slot.SelectionRule = "random"
			slot.EligibleMFTs = []string{nextID()}
		}
		slots[i] = slot
	}

	// ── Assemble ───────────────────────────────────────────────────────────────
	packSlug := fmt.Sprintf("automated_pack_%d_%04d", now.Unix(), rng.Intn(10000))
	saleStart := now.UTC().Format(time.RFC3339)

	return CreatePackPayload{
		PackName:                    "Automated Pack Drop",
		Slug:                        packSlug,
		TotalUserBuyLimit:           buyLimit,
		PerSessionLimit:             buyLimit,
		TotalPackCount:              0,
		SegmentConfigs:              map[string]interface{}{},
		LockSegments:                []interface{}{},
		IneligibleSegments:          []interface{}{},
		ShouldShowToEligibleSegment: false,
		Tags:                        []string{"UPGRADED_BUILD", "FC_PACK"},
		Description:                 "<p>Automated test pack</p>",
		ShortDescription:            "<p>Automated</p>",
		ImageURLs: map[string]string{
			"packImageURL":    "https://storage.googleapis.com/fc-moments-assests-prod/superteam-shops/PREPROD/1776060832399/Frame 1321321373 (1)_Progressive.png",
			"bgImageURL":      "https://storage.googleapis.com/fc-moments-assests-prod/superteam-shops/PREPROD/1776060857998/1x1 Down Under.webp",
			"blurredImageURL": "https://storage.googleapis.com/fc-moments-assests-prod/superteam-shops/PREPROD/1776060840257/Frame 1321321377_Progressive.png",
		},
		AdditionalInfo: map[string]interface{}{},
		FMVConfig: FMVConfig{
			BatchSize:      batchSize,
			NumberOfQueues: numberOfQueues,
			Distribution:   distributions,
		},
		PriorityOrder: 0,
		SlotConfigs:   slots,
		PriceConfig: map[string]map[string]float64{
			priceConfigID: {
				"GC12":        price,
				"strikePrice": strikePrice,
			},
		},
		SaleStartTime: saleStart,
		SaleEndTime:   "",
	}
}

// roundTo2DP rounds f to 2 decimal places.
func roundTo2DP(f float64) float64 {
	return math.Round(f*100) / 100
}

// -------- PUBLIC API --------

// CreatePack posts a pack creation payload to the admin proxy endpoint.
func CreatePack(proxyURL string, payload CreatePackPayload) CreatePackResult {
	c := resty.New().SetTimeout(15 * time.Second)

	type createPackData struct {
		ID       string `json:"_id"`
		Slug     string `json:"slug"`
		PackName string `json:"packName"`
		Status   string `json:"status"`
	}
	type createPackResponse struct {
		Success bool           `json:"success"`
		Data    createPackData `json:"data"`
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return CreatePackResult{
			Success:  false,
			ErrorMsg: fmt.Sprintf("marshal payload: %v", err),
		}
	}

	var apiResp createPackResponse
	resp, err := c.R().
		SetHeader("Content-Type", "text/plain").
		SetBody(string(bodyBytes)).
		SetResult(&apiResp).
		Post(proxyURL + constants.AdminPackCreate)

	if err != nil {
		return CreatePackResult{
			Success:  false,
			ErrorMsg: fmt.Sprintf("request error: %v", err),
		}
	}

	prettyResp := prettyJSON(resp.Body())

	if resp.StatusCode() != 200 && resp.StatusCode() != 201 {
		return CreatePackResult{
			Status:      resp.StatusCode(),
			Success:     false,
			RawResponse: prettyResp,
			ErrorMsg:    resp.String(),
		}
	}
	if !apiResp.Success {
		return CreatePackResult{
			Status:      resp.StatusCode(),
			Success:     false,
			RawResponse: prettyResp,
			ErrorMsg:    fmt.Sprintf("API returned success=false: %s", resp.String()),
		}
	}

	var price, strikePrice float64
	for _, currencies := range payload.PriceConfig {
		price = currencies["GC12"]
		strikePrice = currencies["strikePrice"]
		break
	}

	return CreatePackResult{
		Status:      resp.StatusCode(),
		Success:     true,
		PackID:      apiResp.Data.ID,
		PackSlug:    apiResp.Data.Slug,
		PackName:    apiResp.Data.PackName,
		PackStatus:  apiResp.Data.Status,
		SlotCount:   len(payload.SlotConfigs),
		Price:       price,
		StrikePrice: strikePrice,
		RawResponse: prettyResp,
	}
}

// prettyJSON re-indents raw JSON bytes. Falls back to the raw string on error.
func prettyJSON(b []byte) string {
	var buf interface{}
	if err := json.Unmarshal(b, &buf); err != nil {
		return string(b)
	}
	out, err := json.MarshalIndent(buf, "", "  ")
	if err != nil {
		return string(b)
	}
	return string(out)
}
