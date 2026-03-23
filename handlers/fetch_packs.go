package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/AvinGupta27/code-go-automation/constants"
	"github.com/go-resty/resty/v2"
)

// -------- RESPONSE SHAPES --------

// userPackItem represents a single pack item in the list response.
type userPackItem struct {
	UserPackID string `json:"userPackId"`
	Status     string `json:"status"`
}

// userPacksWrapperData handles the nested { userPacks: [...] } shape.
type userPacksWrapperData struct {
	UserPacks []userPackItem `json:"userPacks"`
}

// userPacksListResponse uses RawMessage for data so we can handle
// both response shapes the API may return:
//   - Array shape:  { "success": true, "data": [ {...}, ... ] }
//   - Object shape: { "success": true, "data": { "userPacks": [...] } }
type userPacksListResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
}

// -------- PUBLIC API --------

// FetchUnrevealedPackIDs calls the Spinner BFF user packs list endpoint and
// returns the userPackId of every pack with status=PURCHASED (bought but not
// yet revealed). It paginates automatically until no more packs are returned.
//
// spinnerBFFURL: e.g. "https://spinnerbff.preprod.munna-bhai.xyz"
// token:         bearer token from GenerateTokens()
func FetchUnrevealedPackIDs(spinnerBFFURL, token string) ([]string, error) {
	client := resty.New()

	const (
		pageLimit        = 500
		unrevealedStatus = "UNREVEALED"
	)

	var allIDs []string
	page := 1

	for {
		var body userPacksListResponse

		resp, err := client.R().
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

		// The data field can be either a direct array or a wrapped object —
		// try array first, then fall back to the { userPacks: [...] } shape.
		packs, err := extractPacks(body.Data)
		if err != nil {
			return nil, fmt.Errorf("FetchUnrevealedPackIDs (page %d): parse error: %w", page, err)
		}

		if len(packs) == 0 {
			break // exhausted all pages
		}

		for _, p := range packs {
			allIDs = append(allIDs, p.UserPackID)
		}

		if len(packs) < pageLimit {
			break // this was the last (partial) page
		}

		page++
	}

	return allIDs, nil
}

// -------- HELPERS --------

// extractPacks handles both API response shapes for the data field.
func extractPacks(raw json.RawMessage) ([]userPackItem, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	// Shape 1: data is a direct array → [ { userPackId, status, ... }, ... ]
	var asArray []userPackItem
	if err := json.Unmarshal(raw, &asArray); err == nil {
		return asArray, nil
	}

	// Shape 2: data is a wrapped object → { userPacks: [ ... ] }
	var asObject userPacksWrapperData
	if err := json.Unmarshal(raw, &asObject); err == nil {
		return asObject.UserPacks, nil
	}

	return nil, fmt.Errorf("unrecognised data shape: %s", string(raw))
}
