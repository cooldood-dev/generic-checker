package license

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type LocalState struct {
	EncryptedLicense string `json:"encrypted_license"`
	LicenseID        string `json:"license_id"`
}

type Storage struct {
	path string
}

func NewStorage(appDataPath string) *Storage {
	return &Storage{
		path: filepath.Join(appDataPath, "license.json"),
	}
}

// encrypt uses AES-GCM to encrypt the plaintext using a key derived from the HWID
func encrypt(plaintext []byte, hwid string) (string, error) {
	key := sha256.Sum256([]byte(hwid + "salt_for_license"))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := aesGCM.Seal(nonce, nonce, plaintext, nil)
	return hex.EncodeToString(ciphertext), nil
}

// decrypt decrypts the hex string back to plaintext
func decrypt(encryptedHex string, hwid string) (string, error) {
	ciphertext, err := hex.DecodeString(encryptedHex)
	if err != nil {
		return "", err
	}

	key := sha256.Sum256([]byte(hwid + "salt_for_license"))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func (s *Storage) Save(licenseKey, licenseID, hwid string) error {
	enc, err := encrypt([]byte(licenseKey), hwid)
	if err != nil {
		return err
	}

	state := LocalState{
		EncryptedLicense: enc,
		LicenseID:        licenseID,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0600)
}

func (s *Storage) Load(hwid string) (string, string, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return "", "", err // File doesn't exist
	}

	var state LocalState
	if err := json.Unmarshal(data, &state); err != nil {
		return "", "", err
	}

	if state.EncryptedLicense == "" {
		return "", "", fmt.Errorf("no license stored")
	}

	licenseKey, err := decrypt(state.EncryptedLicense, hwid)
	if err != nil {
		return "", "", err
	}

	return licenseKey, state.LicenseID, nil
}

func (s *Storage) Clear() error {
	return os.Remove(s.path)
}
