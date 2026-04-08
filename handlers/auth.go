package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/AvinGupta27/code-go-automation/constants"
	"github.com/go-resty/resty/v2"
)

// -------- TYPES --------

// UserCred holds one test account's credentials.
type UserCred struct {
	Email string `json:"email"`
	OTP   string `json:"otp"`
}

// AuthToken holds all tokens generated for a single user.
type AuthToken struct {
	Email       string
	AccessToken string
	SSOToken    string
}

// UserToken holds the result of authentication for a user, including any error.
type UserToken struct {
	AuthToken
	Err error
}

type loginResponse struct {
	Status string `json:"status"`
	Data   bool   `json:"data"`
}

type verifyResponse struct {
	Status string `json:"status"`
	Data   struct {
		AccessToken string `json:"access_token"`
	} `json:"data"`
}

type ssoResponse struct {
	Status string `json:"status"`
	Data   struct {
		SSOToken string `json:"ssoToken"`
	} `json:"data"`
}

// GenerateTokens authenticates a single user credential and returns an AuthToken.
func GenerateTokens(fcBFFURL string, cred UserCred) (AuthToken, error) {
	client := resty.New()

	// Step 1: OTP Login
	var loginResp loginResponse
	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]string{"email": cred.Email}).
		SetResult(&loginResp).
		Post(fcBFFURL + constants.AuthOTPLogin)
	if err != nil {
		return AuthToken{}, fmt.Errorf("GenerateTokens: otp login: %w", err)
	}
	if resp.StatusCode() != 200 || loginResp.Status != "success" {
		return AuthToken{}, fmt.Errorf("GenerateTokens: otp login failed (status %d): %s", resp.StatusCode(), resp.String())
	}

	// Step 2: OTP Verify
	var verifyResp verifyResponse
	resp, err = client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]string{"email": cred.Email, "otp": cred.OTP}).
		SetResult(&verifyResp).
		Post(fcBFFURL + constants.AuthOTPVerify)
	if err != nil {
		return AuthToken{}, fmt.Errorf("GenerateTokens: otp verify: %w", err)
	}
	if resp.StatusCode() != 200 || verifyResp.Status != "success" {
		return AuthToken{}, fmt.Errorf("GenerateTokens: otp verify failed (status %d): %s", resp.StatusCode(), resp.String())
	}
	accessToken := verifyResp.Data.AccessToken

	// Step 3: SSO Generate
	var ssoResp ssoResponse
	resp, err = client.R().
		SetHeader("access_token", accessToken).
		SetResult(&ssoResp).
		Post(fcBFFURL + constants.AuthSSOGenerate)
	if err != nil {
		return AuthToken{}, fmt.Errorf("GenerateTokens: sso generate: %w", err)
	}
	if resp.StatusCode() != 200 || ssoResp.Status != "success" {
		return AuthToken{}, fmt.Errorf("GenerateTokens: sso generate failed (status %d): %s", resp.StatusCode(), resp.String())
	}

	return AuthToken{
		Email:       cred.Email,
		AccessToken: accessToken,
		SSOToken:    ssoResp.Data.SSOToken,
	}, nil
}

// LoadUsers reads a JSON array of UserCred from the given file path.
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

// GenerateAllTokens authenticates all users in parallel and returns a UserToken per user.
func GenerateAllTokens(fcBFFURL string, users []UserCred) []UserToken {
	tokens := make([]UserToken, len(users))
	var wg sync.WaitGroup

	for i, u := range users {
		wg.Add(1)
		go func(idx int, cred UserCred) {
			defer wg.Done()
			auth, err := GenerateTokens(fcBFFURL, cred)
			tokens[idx] = UserToken{AuthToken: auth, Err: err}
		}(i, u)
	}
	wg.Wait()
	return tokens
}
