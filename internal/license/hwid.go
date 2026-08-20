package license

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

type WindowsHWIDProvider struct{}

func (w *WindowsHWIDProvider) GetHWID() (string, error) {
	var hwid string

	// Try reading MachineGuid forcing the 64-bit registry view (avoids WOW6432Node missing key errors)
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err == nil {
		guid, _, err2 := k.GetStringValue("MachineGuid")
		k.Close()
		if err2 == nil && guid != "" {
			hwid = guid
		}
	}

	// Fallback to Hostname + MAC Address if registry access fails entirely
	if hwid == "" {
		hostname, _ := os.Hostname()
		mac := getFirstMacAddress()
		hwid = hostname + "-" + mac
	}
	hwid = strings.TrimSpace(hwid)
	hwid = strings.ToUpper(hwid)

	// Hash it so we don't send the raw HWID
	hash := sha256.Sum256([]byte(hwid))
	return hex.EncodeToString(hash[:]), nil
}

func getFirstMacAddress() string {
	interfaces, err := net.Interfaces()
	if err == nil {
		for _, i := range interfaces {
			if i.Flags&net.FlagUp != 0 && bytes.Compare(i.HardwareAddr, nil) != 0 {
				return i.HardwareAddr.String()
			}
		}
	}
	return "UNKNOWN-MAC"
}
