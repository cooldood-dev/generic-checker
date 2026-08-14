package combo

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// DiscoverInDir walks dir for combolists, ULP logs, and stealer passwords.txt
// dumps, then returns deduped credentials. Skips Netscape/MS cookie dumps.
func DiscoverInDir(dir string, opt LoadOptions) ([]Cred, error) {
	var (
		all      []Cred
		scanned  int
		filesHit int
	)

	fmt.Printf("scanning creds in %s ...\n", dir)

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if path != dir && skipCredDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		lower := strings.ToLower(name)
		if strings.HasPrefix(name, ".") {
			return nil
		}
		ext := filepath.Ext(lower)
		switch ext {
		case "", ".txt", ".combo", ".ulp", ".csv":
		default:
			return nil
		}
		// LevelDB / browser binary logs — never credential lists.
		if strings.Contains(lower, "leveldb") || strings.HasSuffix(lower, ".log") {
			return nil
		}
		// Never treat obvious non-credential stealer junk as a list.
		if lower == "readme" || strings.HasPrefix(lower, "readme.") ||
			lower == "information.txt" || lower == "unfireproof.txt" ||
			lower == "log_info.json" || lower == "pc_info.json" ||
			lower == "system_info.json" || lower == "userinformation.json" ||
			lower == "tlauncherprofiles.json" {
			return nil
		}

		scanned++
		if scanned%5000 == 0 {
			fmt.Printf("\rcreds scan %d files (%d hits)   ", scanned, filesHit)
		}

		// Fast reject: numbered extract cookies (1.txt, 42.txt) are never lists.
		if isNumberedCookieName(lower) {
			return nil
		}

		nameHit := nameLooksLikeCred(lower)
		// Cheap path: only open non-name hits when extension suggests a list,
		// or when we still want Soft/Host content detection on .txt.
		if !nameHit && ext != ".combo" && ext != ".ulp" && ext != ".csv" && ext != ".log" {
			// Still allow content detect for .txt, but skip huge files by size first.
			if info, err := d.Info(); err == nil && info.Size() > 8<<20 {
				return nil
			}
		}

		sample, err := readSample(path, 8<<10) // 8 KiB peek
		if err != nil || len(sample) == 0 {
			return nil
		}
		if looksBinary(sample) {
			return nil
		}
		if !looksLikeCredFile(lower, sample) {
			return nil
		}

		creds, err := LoadAnyFile(path, opt)
		if err != nil || len(creds) == 0 {
			return nil
		}
		filesHit++
		all = append(all, creds...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := dedupeCreds(all)
	fmt.Printf("\rcreds scan done: %d files, %d list files, %d accounts          \n",
		scanned, filesHit, len(out))
	return out, nil
}

func skipCredDir(name string) bool {
	switch strings.ToLower(name) {
	// Don't skip "hits" — stealer/export folders often use that name under -dir.
	case "results", "node_modules", ".git", "logs",
		"wallets", "autofills", "extensions", "history", "downloads",
		"plugins", "local extension settings", "indexeddb":
		return true
	default:
		return false
	}
}

func readSample(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, n)
	got, _ := f.Read(buf)
	return buf[:got], nil
}

func isNumberedCookieName(lower string) bool {
	base := strings.TrimSuffix(lower, filepath.Ext(lower))
	if base == "" {
		return false
	}
	for _, r := range base {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func nameLooksLikeCred(lowerName string) bool {
	return lowerName == "passwords.txt" || strings.HasSuffix(lowerName, "passwords.txt") ||
		(strings.Contains(lowerName, "password") && strings.HasSuffix(lowerName, ".txt")) ||
		strings.Contains(lowerName, "combo") ||
		strings.Contains(lowerName, "combolist") ||
		strings.Contains(lowerName, "ulp") ||
		lowerName == "credentials.txt" || lowerName == "logins.txt" ||
		lowerName == "accounts.txt" || lowerName == "mails.txt"
}

func looksLikeCredFile(lowerName string, sample []byte) bool {
	if nameLooksLikeCred(lowerName) {
		if isCookieDumpSample(sample) && !hasPasswordBlock(sample) {
			return false
		}
		return true
	}

	if isCookieDumpSample(sample) {
		return false
	}
	if hasPasswordBlock(sample) {
		return true
	}
	return comboLineDensity(sample) >= 3
}

func isCookieDumpSample(sample []byte) bool {
	if bytes.Contains(sample, []byte("MSPAuth")) ||
		bytes.Contains(sample, []byte("MSPProf")) ||
		bytes.Contains(sample, []byte("RPSSecAuth")) ||
		bytes.Contains(sample, []byte("__Host-MSAAUTH")) {
		return true
	}
	// Netscape tables are tab-heavy.
	if bytes.Count(sample, []byte{'\t'}) >= 20 &&
		(bytes.Contains(sample, []byte(".com")) || bytes.Contains(sample, []byte("TRUE"))) {
		return true
	}
	return false
}

func hasPasswordBlock(sample []byte) bool {
	lower := bytes.ToLower(sample)
	hasPass := bytes.Contains(lower, []byte("\npassword:")) || bytes.HasPrefix(lower, []byte("password:"))
	hasLogin := bytes.Contains(lower, []byte("\nlogin:")) || bytes.Contains(lower, []byte("\nuser:")) ||
		bytes.Contains(lower, []byte("\nusername:")) || bytes.Contains(lower, []byte("\nemail:"))
	hasHost := bytes.Contains(lower, []byte("\nhost:")) || bytes.Contains(lower, []byte("\nurl:"))
	return hasPass && (hasLogin || hasHost)
}

func comboLineDensity(sample []byte) int {
	n := 0
	for _, raw := range bytes.Split(sample, []byte{'\n'}) {
		line := strings.TrimSpace(string(raw))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, _, _, ok := splitULP(line); ok {
			n++
			continue
		}
		if _, _, ok := splitCombo(line); ok {
			n++
		}
	}
	return n
}
