package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type PromoCode struct {
	Code      string `json:"code"`
	Source    string `json:"source"` // e.g., "Xbox_Rewards"
	IsValid   bool   `json:"is_valid"`
	ExpiredAt string `json:"expired_at,omitempty"`
	FetchTime string `json:"fetch_time"`
}

// DiscordValidationRequest is the payload for Discord's validation API
type DiscordValidationRequest struct {
	Code string `json:"code"`
}

// ValidateDiscordCode sends the code to Discord's API to check if it's valid
// Includes exponential backoff for HTTP 429 Too Many Requests
func ValidateDiscordCode(ctx context.Context, code string, client *http.Client) (*PromoCode, error) {
	url := "https://discord.com/api/v9/entitlements/gift-codes/validate"
	payload, _ := json.Marshal(DiscordValidationRequest{Code: code})
	
	promo := &PromoCode{
		Code:      code,
		Source:    "Discord_API",
		FetchTime: time.Now().UTC().Format(time.RFC3339),
	}

	maxRetries := 3
	backoff := 1 * time.Second

	for i := 0; i <= maxRetries; i++ {
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(payload))
		if err != nil {
			return nil, err
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			if i == maxRetries {
				return nil, fmt.Errorf("rate limited after %d retries", maxRetries)
			}
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			backoff *= 2
			continue
		}
		
		defer resp.Body.Close()
		_, _ = io.ReadAll(resp.Body) // Drain body

		if resp.StatusCode == http.StatusNoContent {
			promo.IsValid = true
		} else {
			promo.IsValid = false
		}
		
		return promo, nil
	}

	return promo, nil
}

// extractCodesFromXbox is a stub for extracting Discord Nitro codes from Xbox Game Pass Perks
func (c *Checker) extractCodesFromXbox(ctx context.Context, cookieHeader string) []string {
	// TODO: Implement Xbox Rewards / Game Pass Perks scraping logic here
	// This would involve calling the Xbox API using the provided MSA/Xbox token.
	return []string{}
}
