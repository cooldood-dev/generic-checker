package license

import "time"

// AuthState represents the high-level state of the application's license.
type AuthState struct {
	Valid     bool      `json:"valid"`
	LicenseID string    `json:"license_id,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// ValidationResult represents the response from the Supabase Edge Function.
type ValidationResult struct {
	Valid     bool   `json:"valid"`
	LicenseID string `json:"license_id,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// HWIDProvider generates a stable hardware identifier for the machine.
type HWIDProvider interface {
	GetHWID() (string, error)
}
