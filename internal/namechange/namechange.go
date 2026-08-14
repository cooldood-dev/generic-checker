package namechange

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	mcProfileNameURL  = "https://api.minecraftservices.com/minecraft/profile/name/"
	mcProfileSkinsURL = "https://api.minecraftservices.com/minecraft/profile/skins"
	mcNameChangeURL   = "https://api.minecraftservices.com/minecraft/profile/namechange"
)

// ErrNotEligible is returned by ChangeName when the account is on a rename
// cooldown or is otherwise restricted from changing its name (HTTP 403).
var ErrNotEligible = errors.New("name change not available (account on cooldown or restricted)")

// ErrNameUnavailable is returned when the requested name is already taken or
// contains invalid characters (HTTP 400).
var ErrNameUnavailable = errors.New("name unavailable or invalid")

// GenerateName returns a fresh name in the format g00n_XXXX
// where X is 4–6 random lowercase letters (a-z).
func GenerateName() string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	n := 4 + rand.Intn(3) // 4, 5, or 6 chars
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return "g00n_" + string(b)
}

// IsEligible checks whether the account is allowed to change its name.
// Mojang enforces a 30-day cooldown between renames.
func IsEligible(ctx context.Context, client *http.Client, accessToken string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mcNameChangeURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var out struct {
		NameChangeAllowed bool `json:"nameChangeAllowed"`
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	_ = json.Unmarshal(body, &out)
	return out.NameChangeAllowed
}

// ChangeName renames the account in-game to newName using the MC Bearer token.
// Returns nil on success.
// Returns ErrNotEligible (HTTP 403) when the account is on cooldown or restricted.
// Returns ErrNameUnavailable (HTTP 400) when the name is taken or has invalid characters.
func ChangeName(ctx context.Context, client *http.Client, accessToken, newName string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, mcProfileNameURL+newName, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("name change request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusForbidden:
		// 403 = account on rename cooldown or otherwise ineligible.
		return ErrNotEligible
	case http.StatusBadRequest:
		// 400 = name taken or contains illegal characters.
		return ErrNameUnavailable
	default:
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
	}
}

// ChangeSkinFromFile reads a local PNG file and uploads it as the account's skin.
// variant must be "classic" or "slim".
func ChangeSkinFromFile(ctx context.Context, client *http.Client, accessToken, skinPath, variant string) error {
	f, err := os.Open(skinPath)
	if err != nil {
		return fmt.Errorf("open skin: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("variant", variant); err != nil {
		return err
	}
	fw, err := mw.CreateFormFile("file", filepath.Base(skinPath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(fw, f); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mcProfileSkinsURL, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("skin change request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = resp.Status
	}
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
}
