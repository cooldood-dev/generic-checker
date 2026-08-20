package combo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// Cred is an email/password pair from a combolist or ULP log.
type Cred struct {
	Email    string
	Password string
	URL      string // set for ULP / passwords.txt Host
	Source   string // file label for results
}

var msHostMarkers = []string{
	"live.com",
	"microsoft.com",
	"microsoftonline.com",
	"xbox.com",
	"xboxlive.com",
	"hotmail.com",
	"outlook.com",
	"passport.com",
	"office.com",
	"minecraft.net",
	"mojang.com",
	"account.microsoft",
	"login.microsoft",
}

// IsMSHost reports whether a ULP URL/host looks Microsoft-related.
func IsMSHost(raw string) bool {
	d := strings.ToLower(strings.TrimSpace(raw))
	if d == "" {
		return false
	}
	for _, m := range msHostMarkers {
		if strings.Contains(d, m) {
			return true
		}
	}
	return false
}

// LoadOptions controls credential loading from mixed dumps.
type LoadOptions struct {
	// MSOnly filters ULP / Soft-Host-Login-Password entries to Microsoft hosts.
	// Plain email:password combo lines are always kept.
	MSOnly bool
}

// LoadComboFile reads email:password lines (password may contain ':').
func LoadComboFile(path string) ([]Cred, error) {
	return LoadAnyFile(path, LoadOptions{})
}

// LoadComboPath loads a combo file or every .txt under a directory.
// Accepts email:pass, url:login:pass, and Soft/Host/Login/Password in the same files.
func LoadComboPath(path string) ([]Cred, error) {
	return LoadAnyPath(path, LoadOptions{})
}

// LoadAnyFile reads every supported credential format from one file.
func LoadAnyFile(path string, opt LoadOptions) ([]Cred, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if looksBinary(b) {
		return nil, nil
	}
	base := filepath.Base(path)
	text := string(b)
	var out []Cred
	out = append(out, parsePasswordBlocks(text, base, opt.MSOnly)...)
	out = append(out, parseULPLines(text, base, opt.MSOnly)...)
	return dedupeCreds(out), nil
}

// LoadAnyPath loads a file or walks a directory for credential dumps of any format.
func LoadAnyPath(path string, opt LoadOptions) ([]Cred, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return LoadAnyFile(path, opt)
	}
	var all []Cred
	err = filepath.WalkDir(path, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		name := strings.ToLower(d.Name())
		ext := filepath.Ext(name)
		switch ext {
		case "", ".txt", ".combo", ".ulp", ".csv":
		default:
			return nil
		}
		creds, err := LoadAnyFile(p, opt)
		if err != nil || len(creds) == 0 {
			return nil
		}
		all = append(all, creds...)
		return nil
	})
	return dedupeCreds(all), err
}

func splitCombo(line string) (email, pass string, ok bool) {
	// Reject binary / control-heavy lines early (LevelDB logs, etc.).
	if !isPrintableLine(line) {
		return "", "", false
	}
	// email:password — split on first ':'
	i := strings.IndexByte(line, ':')
	if i <= 0 || i == len(line)-1 {
		return "", "", false
	}
	email = strings.TrimSpace(line[:i])
	pass = strings.TrimSpace(line[i+1:])
	if !isValidEmail(email) || pass == "" || len(pass) > 512 || !isPrintableLine(pass) {
		return "", "", false
	}
	return email, pass, true
}

func isValidEmail(s string) bool {
	if len(s) < 5 || len(s) > 254 {
		return false
	}
	at := strings.IndexByte(s, '@')
	if at <= 0 || at != strings.LastIndexByte(s, '@') {
		return false
	}
	domain := s[at+1:]
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '@', r == '.', r == '_', r == '-', r == '+', r == '%':
		default:
			return false
		}
	}
	return true
}

func isPrintableLine(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r == '\t' {
			continue
		}
		if r < 32 || r == 127 || r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func looksBinary(b []byte) bool {
	n := len(b)
	if n == 0 {
		return false
	}
	if n > 1024 {
		n = 1024
	}
	nonPrint := 0
	for i := 0; i < n; i++ {
		c := b[i]
		if c == 0 {
			return true
		}
		if c < 0x09 || (c > 0x0d && c < 0x20) || c >= 0x7f {
			nonPrint++
		}
	}
	return nonPrint*100/n > 20
}

// LoadULPFile reads url:login:password, Soft/Host/Login/Password, and email:pass lines.
func LoadULPFile(path string, msOnly bool) ([]Cred, error) {
	return LoadAnyFile(path, LoadOptions{MSOnly: msOnly})
}

// LoadULPPath loads a ULP file or walks a directory for credential logs.
func LoadULPPath(path string, msOnly bool) ([]Cred, error) {
	return LoadAnyPath(path, LoadOptions{MSOnly: msOnly})
}

// Merge dedupes credentials from multiple loads (email+password).
func Merge(parts ...[]Cred) []Cred {
	var all []Cred
	for _, p := range parts {
		all = append(all, p...)
	}
	return dedupeCreds(all)
}

func parseULPLines(text, source string, msOnly bool) []Cred {
	var out []Cred
	lines := strings.Split(text, "\n")
	for i, raw := range lines {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		// Skip stealer block keys (handled separately).
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "soft:") || strings.HasPrefix(lower, "host:") ||
			strings.HasPrefix(lower, "login:") || strings.HasPrefix(lower, "password:") ||
			strings.HasPrefix(lower, "user:") || strings.HasPrefix(lower, "username:") {
			continue
		}
		u, login, pass, ok := splitULP(line)
		if !ok {
			// Fallback: treat as email:pass combo line inside a ULP dump.
			if email, p, ok := splitCombo(line); ok {
				out = append(out, Cred{
					Email:    email,
					Password: p,
					Source:   fmt.Sprintf("%s:%d", source, i+1),
				})
			}
			continue
		}
		if msOnly && !IsMSHost(u) {
			continue
		}
		if !strings.Contains(login, "@") {
			continue
		}
		out = append(out, Cred{
			Email:    login,
			Password: pass,
			URL:      u,
			Source:   fmt.Sprintf("%s:%d", source, i+1),
		})
	}
	return out
}

// splitULP parses url:login:password by splitting from the right.
func splitULP(line string) (hostURL, login, pass string, ok bool) {
	if !isPrintableLine(line) {
		return "", "", "", false
	}
	// Need at least two ':' separators.
	i := strings.LastIndex(line, ":")
	if i <= 0 || i == len(line)-1 {
		return "", "", "", false
	}
	pass = strings.TrimSpace(line[i+1:])
	rest := line[:i]
	j := strings.LastIndex(rest, ":")
	if j <= 0 || j == len(rest)-1 {
		return "", "", "", false
	}
	login = strings.TrimSpace(rest[j+1:])
	hostURL = strings.TrimSpace(rest[:j])
	if hostURL == "" || !isValidEmail(login) || pass == "" || len(pass) > 512 || !isPrintableLine(pass) {
		return "", "", "", false
	}
	// Reject if "URL" looks like an email (that's a combo line).
	if strings.Contains(hostURL, "@") && !strings.Contains(hostURL, "/") {
		return "", "", "", false
	}
	return hostURL, login, pass, true
}

func parsePasswordBlocks(text, source string, msOnly bool) []Cred {
	var out []Cred
	var host, login, pass string
	flush := func() {
		if login == "" || pass == "" {
			host, login, pass = "", "", ""
			return
		}
		if !isValidEmail(login) || pass == "" || !isPrintableLine(pass) {
			host, login, pass = "", "", ""
			return
		}
		if msOnly && host != "" && !IsMSHost(host) {
			host, login, pass = "", "", ""
			return
		}
		// No host: treat as a bare login/password pair (always kept).
		out = append(out, Cred{
			Email:    login,
			Password: pass,
			URL:      host,
			Source:   source,
		})
		host, login, pass = "", "", ""
	}

	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" {
			flush()
			continue
		}
		key, val, cut := strings.Cut(line, ":")
		if !cut {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		val = strings.TrimSpace(val)
		switch key {
		case "host", "url", "website":
			if login != "" || pass != "" {
				flush()
			}
			host = val
		case "login", "user", "username", "email":
			login = val
		case "password", "pass":
			pass = val
			if login != "" {
				flush()
			}
		case "soft", "browser", "profile":
			// ignore; new record often starts after Soft
		}
	}
	flush()
	return out
}

func dedupeCreds(in []Cred) []Cred {
	if len(in) < 2 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]Cred, 0, len(in))
	for _, c := range in {
		key := strings.ToLower(c.Email) + "\x00" + c.Password
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, c)
	}
	return out
}
