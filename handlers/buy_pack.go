package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
)

// -------- TYPES --------

// UserCred holds one test account's credentials.
type UserCred struct {
	Email string `json:"email"`
	OTP   string `json:"otp"`
}

// UserToken holds the generated token for a user.
type UserToken struct {
	Email       string
	AccessToken string
	SSOToken    string
	Err         error
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

type RevealNFT struct {
	NFTTokenID string  `json:"nftTokenId"`
	CardName   string  `json:"cardName"`
	Rarity     string  `json:"rarity"`
	Value      float64 `json:"value"`
}

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

// -------- AUTH ========

func LoadUsers(path string) ([]UserCred, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var users []UserCred
	if err := json.Unmarshal(b, &users); err != nil {
		return nil, err
	}
	return users, nil
}

// GenerateAllTokens calls the original GenerateTokens via temporary files due to earlier architecture.
func GenerateAllTokens(fcBFFURL string, users []UserCred) []UserToken {
	tokens := make([]UserToken, len(users))
	var wg sync.WaitGroup

	for i, u := range users {
		wg.Add(1)
		go func(idx int, cred UserCred) {
			defer wg.Done()
			
			// Hack: Write a temp file for GenerateTokens to read
			tmpDir := os.TempDir()
			tmpFile := filepath.Join(tmpDir, fmt.Sprintf("user_%d.json", time.Now().UnixNano()+int64(idx)))
			b, _ := json.Marshal(cred)
			os.WriteFile(tmpFile, b, 0644)
			defer os.Remove(tmpFile)

			at, st, err := GenerateTokens(fcBFFURL, tmpFile)
			tokens[idx] = UserToken{
				Email:       cred.Email,
				AccessToken: at,
				SSOToken:    st,
				Err:         err,
			}
		}(i, u)
	}
	wg.Wait()
	return tokens
}

// -------- BUY & REVEAL ========

func BuyPack(spinnerBFFURL, accessToken string, req BuyPackRequest, email string) BuyPackResult {
	result := BuyPackResult{Email: email}
	client := resty.New().SetTimeout(10 * time.Second)

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		result.ErrorMsg = fmt.Sprintf("failed to marshal request body: %v", err)
		return result
	}

	var apiResp buyPackAPIResponse
	start := time.Now()
	resp, err := client.R().
		SetHeader("Authorization", accessToken).
		SetHeader("Content-Type", "text/plain;charset=UTF-8").
		SetBody(string(bodyBytes)).
		SetResult(&apiResp).
		Post(spinnerBFFURL + "/api/v1/packsmaster/buy")
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

func RevealPack(spinnerBFFURL, accessToken, userPackID, email string) RevealResult {
	result := RevealResult{Email: email, UserPackID: userPackID}
	client := resty.New().SetTimeout(15 * time.Second)

	var apiResp revealAPIResponse
	start := time.Now()
	resp, err := client.R().
		SetHeader("Authorization", accessToken).
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]string{"userPackId": userPackID}).
		SetResult(&apiResp).
		Post(spinnerBFFURL + "/api/v1/userpacks/reveal")
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

	for _, c := range shape.Cards {
		v := c.Value
		if v == 0 {
			v = c.Price
		}
		name := c.CardName
		if name == "" {
			name = c.Name
		}
		result.NFTs = append(result.NFTs, RevealNFT{
			NFTTokenID: c.NFTTokenID,
			CardName:   name,
			Rarity:     c.Rarity,
			Value:      v,
		})
		result.TotalValue += v
	}
	result.NFTCount = len(result.NFTs)
	return result
}
