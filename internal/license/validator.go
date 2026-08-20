package license

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// In a real application, these should be injected via build flags (e.g. -ldflags="-X ...") or read from an env file.
const (
	SupabaseURL = "https://lehzyfcfmosrrdpizgbu.supabase.co/functions/v1/validate-license"
	// Optional: if the edge function requires Authorization header
	SupabaseAnonKey = "sb_publishable_7S8OlobdmjUUNi4B8XtM9w_TliwXU8-" 
)

type Validator struct {
	client *http.Client
}

func NewValidator() *Validator {
	return &Validator{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

type validateRequest struct {
	License string `json:"license"`
	HWID    string `json:"hwid"`
}

func (v *Validator) Validate(ctx context.Context, licenseKey, hwid string) (*ValidationResult, error) {
	reqBody := validateRequest{
		License: licenseKey,
		HWID:    hwid,
	}
	
	b, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", SupabaseURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if SupabaseAnonKey != "" && SupabaseAnonKey != "eyJhb..." {
		req.Header.Set("Authorization", "Bearer "+SupabaseAnonKey)
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network_error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("server_error: status %d", resp.StatusCode)
	}

	var res ValidationResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &res, nil
}
