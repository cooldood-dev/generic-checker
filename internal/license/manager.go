package license

import (
	"context"
	"fmt"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type Manager struct {
	mu        sync.RWMutex
	ctx       context.Context // Wails app context
	hwidProv  HWIDProvider
	storage   *Storage
	validator *Validator

	state      AuthState
	activeKey  string
	tickerStop chan struct{}
}

func NewManager(appDataPath string) *Manager {
	return &Manager{
		hwidProv:  &WindowsHWIDProvider{},
		storage:   NewStorage(appDataPath),
		validator: NewValidator(),
		state:     AuthState{Valid: false},
	}
}

func (m *Manager) Startup(ctx context.Context) {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()

	// Initial load and validate
	m.checkLocalActivation()
}

func (m *Manager) checkLocalActivation() {
	hwid, err := m.hwidProv.GetHWID()
	if err != nil {
		fmt.Printf("[AUTH] Failed to get HWID: %v\n", err)
		return
	}

	key, id, err := m.storage.Load(hwid)
	if err != nil || key == "" {
		fmt.Printf("[AUTH] No local activation found or corrupted.\n")
		return
	}

	fmt.Printf("[AUTH] Local activation found. Validating...\n")
	
	// Validate locally stored license synchronously for startup
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	
	res, err := m.validator.Validate(ctx, key, hwid)
	if err != nil {
		fmt.Printf("[AUTH] Temporary network error during startup validation: %v. Allowing grace period.\n", err)
		// Assume valid temporarily during grace period if network is down at startup
		m.updateState(true, id, time.Time{}, key)
		m.startBackgroundValidator()
		return
	}

	if res.Valid {
		var exp time.Time
		if res.ExpiresAt != "" {
			exp, _ = time.Parse(time.RFC3339, res.ExpiresAt)
		}
		fmt.Printf("[AUTH] License validation successful.\n")
		m.updateState(true, res.LicenseID, exp, key)
		m.startBackgroundValidator()
	} else {
		fmt.Printf("[AUTH] License validation definitively failed: %s\n", res.Reason)
		m.storage.Clear()
		m.updateState(false, "", time.Time{}, "")
	}
}

func (m *Manager) updateState(valid bool, licenseID string, exp time.Time, key string) {
	m.mu.Lock()
	m.state.Valid = valid
	m.state.LicenseID = licenseID
	m.state.ExpiresAt = exp
	m.activeKey = key
	m.mu.Unlock()

	if m.ctx != nil {
		wailsruntime.EventsEmit(m.ctx, "license:status", m.state)
	}
}

// -----------------------------------------------------------------------------
// Wails Exported Methods
// -----------------------------------------------------------------------------

// GetAuthenticationState returns the current auth state
func (m *Manager) GetAuthenticationState() AuthState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// ActivateLicense attempts to bind a new license key
func (m *Manager) ActivateLicense(key string) error {
	hwid, err := m.hwidProv.GetHWID()
	if err != nil {
		return fmt.Errorf("failed to generate HWID: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	res, err := m.validator.Validate(ctx, key, hwid)
	if err != nil {
		return fmt.Errorf("network_error: %v", err)
	}

	if !res.Valid {
		return fmt.Errorf(res.Reason) // Invalid license, mismatch, expired, etc.
	}

	// Persist
	if err := m.storage.Save(key, res.LicenseID, hwid); err != nil {
		return fmt.Errorf("failed to save activation locally: %v", err)
	}

	var exp time.Time
	if res.ExpiresAt != "" {
		exp, _ = time.Parse(time.RFC3339, res.ExpiresAt)
	}

	m.updateState(true, res.LicenseID, exp, key)
	m.startBackgroundValidator()

	return nil
}

// Logout revokes local auth state
func (m *Manager) Logout() error {
	m.mu.Lock()
	if m.tickerStop != nil {
		close(m.tickerStop)
		m.tickerStop = nil
	}
	m.mu.Unlock()

	m.storage.Clear()
	m.updateState(false, "", time.Time{}, "")
	return nil
}

// -----------------------------------------------------------------------------
// Background Validation
// -----------------------------------------------------------------------------

func (m *Manager) startBackgroundValidator() {
	m.mu.Lock()
	if m.tickerStop != nil {
		m.mu.Unlock()
		return // Already running
	}
	m.tickerStop = make(chan struct{})
	stop := m.tickerStop
	m.mu.Unlock()

	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		fmt.Println("[AUTH] Background validator started")

		for {
			select {
			case <-ticker.C:
				m.validateRoutine()
			case <-stop:
				fmt.Println("[AUTH] Background validator stopped")
				return
			}
		}
	}()
}

func (m *Manager) validateRoutine() {
	m.mu.RLock()
	key := m.activeKey
	m.mu.RUnlock()

	if key == "" {
		return
	}

	hwid, err := m.hwidProv.GetHWID()
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	res, err := m.validator.Validate(ctx, key, hwid)
	if err != nil {
		// Network error (timeout, 500, DNS) -> Grace period, do not kill application yet
		fmt.Printf("[AUTH] Background validation network error: %v. Retrying next cycle.\n", err)
		return
	}

	if !res.Valid {
		fmt.Printf("[AUTH] Background validation DEFINITIVELY FAILED: %s\n", res.Reason)
		m.handleRevocation()
	}
}

func (m *Manager) handleRevocation() {
	m.Logout()

	// Notify frontend
	if m.ctx != nil {
		wailsruntime.EventsEmit(m.ctx, "license:revoked", "Your license is no longer valid or has been revoked.")
		
		// Gracefully shut down after a short delay so user sees message
		go func() {
			time.Sleep(3 * time.Second)
			wailsruntime.Quit(m.ctx)
		}()
	}
}
