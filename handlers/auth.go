package handlers

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/AvinGupta27/code-go-automation/constants"
	"github.com/go-resty/resty/v2"
)

type User struct {
	Email string `json:"email"`
	OTP   string `json:"otp"`
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

func GenerateTokens(fcBFFURL, userFile string) (accessToken, ssoToken string, err error) {
	user, err := loadUser(userFile)
	if err != nil {
		return "", "", fmt.Errorf("GenerateTokens: load user: %w", err)
	}
	client := resty.New()

	// Step 1: OTP Login
	var loginResp loginResponse
	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]string{"email": user.Email}).
		SetResult(&loginResp).
		Post(fcBFFURL + constants.AuthOTPLogin)
	if err != nil {
		return "", "", fmt.Errorf("GenerateTokens: otp login: %w", err)
	}
	if resp.StatusCode() != 200 || loginResp.Status != "success" {
		return "", "", fmt.Errorf("GenerateTokens: otp login failed (status %d): %s", resp.StatusCode(), resp.String())
	}
	
	// Step 2: OTP Verify
	var verifyResp verifyResponse
	resp, err = client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]string{"email": user.Email, "otp": user.OTP}).
		SetResult(&verifyResp).
		Post(fcBFFURL + constants.AuthOTPVerify)
	if err != nil {
		return "", "", fmt.Errorf("GenerateTokens: otp verify: %w", err)
	}
	if resp.StatusCode() != 200 || verifyResp.Status != "success" {
		return "", "", fmt.Errorf("GenerateTokens: otp verify failed (status %d): %s", resp.StatusCode(), resp.String())
	}
	accessToken = verifyResp.Data.AccessToken

	// Step 3: SSO Generate
	var ssoResp ssoResponse
	resp, err = client.R().
		SetHeader("access_token", accessToken).
		SetResult(&ssoResp).
		Post(fcBFFURL + constants.AuthSSOGenerate)
	if err != nil {
		return "", "", fmt.Errorf("GenerateTokens: sso generate: %w", err)
	}
	if resp.StatusCode() != 200 || ssoResp.Status != "success" {
		return "", "", fmt.Errorf("GenerateTokens: sso generate failed (status %d): %s", resp.StatusCode(), resp.String())
	}
	ssoToken = ssoResp.Data.SSOToken

	return accessToken, ssoToken, nil
}

// -------- HELPERS --------

func loadUser(path string) (*User, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var user User
	if err := json.Unmarshal(file, &user); err != nil {
		return nil, err
	}
	return &user, nil
}
