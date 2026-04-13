package client

import (
	"encoding/json"
	"os"
)

// UserCred holds one test account's credentials.
type UserCred struct {
	Email string `json:"email"`
	OTP   string `json:"otp"`
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
