package client

import (
	"fmt"
	"time"

	"github.com/AvinGupta27/code-go-automation/constants"
	"github.com/go-resty/resty/v2"
)

// -------- TYPES --------

// WalletCurrency holds the balance for a single currency in the wallet.
type WalletCurrency struct {
	CurrencyID string  `json:"currencyId"`
	Value      float64 `json:"value"`
	Locked     float64 `json:"locked"`
	Unlocked   float64 `json:"unlocked"`
	ShortTitle string  `json:"shortTitle"`
}

// WalletBalance is a convenience map of currencyId → WalletCurrency,
// built from the raw currencies array for fast lookups.
type WalletBalance map[string]WalletCurrency

// Unlocked returns the unlocked balance for the given currencyId (e.g. "GC12").
// Returns 0 if the currency is not present.
func (w WalletBalance) Unlocked(currencyID string) float64 {
	if c, ok := w[currencyID]; ok {
		return c.Unlocked
	}
	return 0
}

// HasSufficientBalance returns true if unlocked balance >= required amount.
func (w WalletBalance) HasSufficientBalance(currencyID string, required float64) bool {
	return w.Unlocked(currencyID) >= required
}

type walletResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Currencies []WalletCurrency `json:"currencies"`
	} `json:"data"`
}

// -------- PUBLIC API --------

// FetchWallet fetches the user's wallet and returns a WalletBalance map.
func FetchWallet(fcBFFURL, token string) (WalletBalance, error) {
	c := resty.New().SetTimeout(10 * time.Second)

	var resp walletResponse
	r, err := c.R().
		SetHeader("x_auth_token", token).
		SetHeader("source", "WEB").
		SetResult(&resp).
		Get(fcBFFURL + constants.UserWallet)

	if err != nil {
		return nil, fmt.Errorf("FetchWallet: request error: %w", err)
	}
	if r.StatusCode() != 200 {
		return nil, fmt.Errorf("FetchWallet: unexpected status %d: %s", r.StatusCode(), r.String())
	}
	if !resp.Success {
		return nil, fmt.Errorf("FetchWallet: API returned success=false: %s", r.String())
	}

	balance := make(WalletBalance, len(resp.Data.Currencies))
	for _, c := range resp.Data.Currencies {
		balance[c.CurrencyID] = c
	}
	return balance, nil
}
