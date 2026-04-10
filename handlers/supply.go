package handlers

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/AvinGupta27/code-go-automation/constants"
	"github.com/go-resty/resty/v2"
)

// -------- TYPES --------

// EventGroup is the subset of fields we need from the findAll response.
type EventGroup struct {
	ID              string  `json:"_id"`
	Slug            string  `json:"slug"`
	Rarity          string  `json:"rarity"`
	AvailableSupply float64 `json:"availableSupply"`
}

// SupplyBreakdown holds the supply figures for a single event group.
type SupplyBreakdown struct {
	EventGroupID   string
	MaxSupply      float64
	FloatingSupply float64
	LockedSupply   float64
	PacksReserve   float64
	LpReserve      float64
	AvailableSupply float64
}

// Total returns the sum of all allocated components.
// The invariant we test: Total() must never exceed MaxSupply.
func (s SupplyBreakdown) Total() float64 {
	return s.FloatingSupply + s.LockedSupply + s.PacksReserve + s.LpReserve + s.AvailableSupply
}

type eventGroupsResponse struct {
	Data []EventGroup `json:"data"`
}

type supplyResponseData struct {
	Supply map[string]supplyEntry `json:"supply"`
}

type supplyResponse struct {
	Success bool               `json:"success"`
	Data    supplyResponseData `json:"data"`
}

type supplyEntry struct {
	MaxSupply       float64 `json:"maxSupply"`
	FloatingSupply  float64 `json:"floatingSupply"`
	LockedSupply    float64 `json:"lockedSupply"`
	PacksReserve    float64 `json:"packsReserve"`
	LpReserve       float64 `json:"lpReserve"`
	AvailableSupply float64 `json:"availableSupply"`
}

// -------- PUBLIC API --------

// FetchEventGroups fetches all event groups from the Spinner BFF findAll endpoint.
// spinnerBFFURL: e.g. "https://spinnerbff.preprod.munna-bhai.xyz"
// token:         bearer access token
func FetchEventGroups(spinnerBFFURL, token string, page, limit int) ([]EventGroup, error) {
	client := resty.New().SetTimeout(15 * time.Second)

	var resp eventGroupsResponse
	r, err := client.R().
		SetHeader("Authorization", token).
		SetQueryParams(map[string]string{
			"page":  fmt.Sprintf("%d", page),
			"limit": fmt.Sprintf("%d", limit),
		}).
		SetResult(&resp).
		Get(spinnerBFFURL + constants.EventGroupsFindAll)

	if err != nil {
		return nil, fmt.Errorf("FetchEventGroups: request error: %w", err)
	}
	if r.StatusCode() != 200 {
		return nil, fmt.Errorf("FetchEventGroups: unexpected status %d: %s", r.StatusCode(), r.String())
	}
	return resp.Data, nil
}

func FetchSupplyBreakdowns(proxyURL string, ids []string) ([]SupplyBreakdown, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	client := resty.New().SetTimeout(30 * time.Second)

	body, err := json.Marshal(map[string][]string{"eventIds": ids})
	if err != nil {
		return nil, fmt.Errorf("FetchSupplyBreakdowns: marshal body: %w", err)
	}

	var resp supplyResponse
	r, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(string(body)).
		SetResult(&resp).
		Post(proxyURL + constants.EventGroupsSupply)

	if err != nil {
		return nil, fmt.Errorf("FetchSupplyBreakdowns: request error: %w", err)
	}
	if r.StatusCode() != 200 {
		return nil, fmt.Errorf("FetchSupplyBreakdowns: unexpected status %d: %s", r.StatusCode(), r.String())
	}
	if !resp.Success {
		return nil, fmt.Errorf("FetchSupplyBreakdowns: API returned success=false: %s", r.String())
	}

	breakdowns := make([]SupplyBreakdown, 0, len(resp.Data.Supply))
	for id, s := range resp.Data.Supply {
		breakdowns = append(breakdowns, SupplyBreakdown{
			EventGroupID:    id,
			MaxSupply:       s.MaxSupply,
			FloatingSupply:  s.FloatingSupply,
			LockedSupply:    s.LockedSupply,
			PacksReserve:    s.PacksReserve,
			LpReserve:       s.LpReserve,
			AvailableSupply: s.AvailableSupply,
		})
	}
	return breakdowns, nil
}
