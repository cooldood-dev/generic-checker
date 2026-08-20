package cookiefile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Cookie struct {
	Name   string
	Value  string
	Domain string
	Path   string
}

var emailRe = regexp.MustCompile(`(?i)[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}`)

var sessionNames = map[string]struct{}{
	"MSPAuth":             {},
	"MSPProf":             {},
	"RPSSecAuth":          {},
	"__Host-MSAAUTH":      {},
	"__Host-MSAAUTHCOOKIE": {},
	"MSPCID":              {},
	"WLSSC":               {},
}

var skipExt = map[string]struct{}{
	".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".webp": {}, ".ico": {},
	".exe": {}, ".zip": {}, ".rar": {}, ".7z": {}, ".dll": {}, ".bin": {},
	".mp4": {}, ".mp3": {}, ".pdf": {}, ".doc": {}, ".docx": {},
}

// Domains kept for MSA / Xbox / Minecraft login Cookie headers.
// Matched as exact domain or parent suffix (e.g. login.live.com → live.com).
var msDomainSet = map[string]struct{}{
	"live.com":           {},
	"microsoft.com":      {},
	"xbox.com":           {},
	"xboxlive.com":       {},
	"office.com":         {},
	"microsoftonline.com": {},
	"msn.com":            {},
	"outlook.com":        {},
	"hotmail.com":        {},
	"passport.com":       {},
	"account.microsoft":  {},
	"login.microsoft":    {},
	"bing.com":           {},
}

func looksLikeCookieName(name string) bool {
	n := len(name)
	if n == 0 || n >= 128 {
		return false
	}
	for i := 0; i < n; i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '-', c == '.':
		default:
			return false
		}
	}
	return true
}

// normalizeDomain strips Netscape HttpOnly/leading-dot prefixes and lowercases.
func normalizeDomain(domain string) string {
	if domain == "" {
		return ""
	}
	if domain[0] == '.' {
		domain = domain[1:]
	}
	// "#HttpOnly_" may already be stripped; plain "HttpOnly_" still appears.
	if len(domain) >= 9 && (domain[0] == 'H' || domain[0] == 'h') {
		if strings.EqualFold(domain[:9], "httponly_") {
			domain = domain[9:]
			if domain != "" && domain[0] == '.' {
				domain = domain[1:]
			}
		}
	}
	// Fast path: already lowercase ASCII (typical cookie dumps).
	for i := 0; i < len(domain); i++ {
		c := domain[i]
		if c >= 'A' && c <= 'Z' {
			return strings.ToLower(domain)
		}
		if c >= 0x80 {
			return strings.ToLower(domain)
		}
	}
	return domain
}

func IsMSDomain(domain string) bool {
	d := normalizeDomain(domain)
	if d == "" {
		return false
	}
	for {
		if _, ok := msDomainSet[d]; ok {
			return true
		}
		i := strings.IndexByte(d, '.')
		if i < 0 {
			return false
		}
		d = d[i+1:]
	}
}

func keepEmptyDomainCookie(name string) bool {
	if _, ok := sessionNames[name]; ok {
		return true
	}
	if strings.HasPrefix(name, "MSP") {
		return true
	}
	if len(name) >= 3 {
		// cheap ASCII "msa" substring check without ToLower alloc when possible
		lower := strings.ToLower(name)
		if strings.Contains(lower, "msa") {
			return true
		}
	}
	return false
}

// FilterMS keeps only Microsoft-related cookies (same idea as extract).
// Full browser dumps must be filtered before auth or login.live.com returns HTTP 400.
func FilterMS(cs []Cookie) []Cookie {
	if len(cs) == 0 {
		return cs
	}
	out := make([]Cookie, 0, min(len(cs), 64))
	for _, c := range cs {
		if c.Domain == "" {
			if keepEmptyDomainCookie(c.Name) {
				out = append(out, c)
			}
			continue
		}
		if IsMSDomain(c.Domain) {
			out = append(out, c)
		}
	}
	return out
}

func sanitizeCookieValue(v string) string {
	if v == "" {
		return v
	}
	// net/http rejects Cookie headers with raw newlines / NUL / DEL
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c == 0x7f || c < 0x20 {
			b := make([]byte, 0, len(v))
			for j := 0; j < len(v); j++ {
				c := v[j]
				if c == 0x7f || c < 0x20 {
					continue
				}
				b = append(b, c)
			}
			return string(b)
		}
	}
	return v
}

// forEachLine walks text without allocating a []string of all lines.
func forEachLine(text string, fn func(line string)) {
	for len(text) > 0 {
		var line string
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			line = text[:i]
			text = text[i+1:]
		} else {
			line = text
			text = ""
		}
		if n := len(line); n > 0 && line[n-1] == '\r' {
			line = line[:n-1]
		}
		// trim spaces without allocating when already clean
		start, end := 0, len(line)
		for start < end && (line[start] == ' ' || line[start] == '\t') {
			start++
		}
		for end > start && (line[end-1] == ' ' || line[end-1] == '\t') {
			end--
		}
		if start >= end {
			continue
		}
		fn(line[start:end])
	}
}

func splitNetscapeFields(line string) (domain, path, name, value string, ok bool) {
	// Netscape: domain \t flag \t path \t secure \t expiry \t name \t value
	var fields [7]string
	n := 0
	start := 0
	for i := 0; i <= len(line) && n < 7; i++ {
		if i == len(line) || line[i] == '\t' {
			fields[n] = line[start:i]
			n++
			start = i + 1
			if i < len(line) && n == 6 {
				// value may contain tabs — take the rest
				fields[6] = line[start:]
				n = 7
				break
			}
		}
	}
	if n < 7 {
		return "", "", "", "", false
	}
	return fields[0], fields[2], fields[5], fields[6], true
}

func ParseNetscape(text string) []Cookie {
	return parseNetscape(text, false)
}

// ParseNetscapeMS parses Netscape cookies and keeps only Microsoft-domain rows.
func ParseNetscapeMS(text string) []Cookie {
	return parseNetscape(text, true)
}

func parseNetscape(text string, msOnly bool) []Cookie {
	out := make([]Cookie, 0, 32)
	forEachLine(text, func(line string) {
		working := line
		if working[0] == '#' {
			// #HttpOnly_domain\t...
			if len(working) > 10 && strings.EqualFold(working[:10], "#httponly_") {
				working = working[10:]
			} else {
				return
			}
		}
		domain, path, name, value, ok := splitNetscapeFields(working)
		if !ok || name == "" || value == "" {
			return
		}
		if msOnly && !IsMSDomain(domain) {
			return
		}
		if path == "" {
			path = "/"
		}
		out = append(out, Cookie{Name: name, Value: value, Domain: domain, Path: path})
	})
	return out
}

func ParseJSON(text string) ([]Cookie, error) {
	text = strings.TrimSpace(text)
	var out []Cookie

	var arr []map[string]any
	if err := json.Unmarshal([]byte(text), &arr); err == nil {
		for _, item := range arr {
			name := strField(item, "name", "Name")
			value := strField(item, "value", "Value")
			if name == "" || value == "" {
				continue
			}
			domain := strField(item, "domain", "Domain")
			if domain == "" {
				domain = ".live.com"
			}
			path := strField(item, "path", "Path")
			if path == "" {
				path = "/"
			}
			out = append(out, Cookie{Name: name, Value: value, Domain: domain, Path: path})
		}
		return out, nil
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(text), &obj); err != nil {
		return nil, err
	}
	if raw, ok := obj["cookies"]; ok {
		b, err := json.Marshal(raw)
		if err != nil {
			return nil, err
		}
		return ParseJSON(string(b))
	}
	for name, v := range obj {
		s, ok := v.(string)
		if !ok || !looksLikeCookieName(name) {
			continue
		}
		out = append(out, Cookie{Name: name, Value: s, Domain: ".live.com", Path: "/"})
	}
	return out, nil
}

func strField(m map[string]any, keys ...string) string {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			return t
		case float64:
			return fmt.Sprintf("%.0f", t)
		case json.Number:
			return t.String()
		default:
			return fmt.Sprint(t)
		}
	}
	return ""
}

func ParseHeader(text string) []Cookie {
	var out []Cookie
	for _, chunk := range splitHeaderChunks(text) {
		if chunk == "" || !strings.Contains(chunk, "=") {
			continue
		}
		name, value, _ := strings.Cut(chunk, "=")
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if strings.HasPrefix(name, "Cookie:") {
			name = strings.TrimSpace(name[len("Cookie:"):])
		}
		if looksLikeCookieName(name) && value != "" {
			out = append(out, Cookie{Name: name, Value: value, Domain: ".live.com", Path: "/"})
		}
	}
	return out
}

func splitHeaderChunks(text string) []string {
	// Split on ; and newlines without regexp.
	var out []string
	start := 0
	for i := 0; i <= len(text); i++ {
		if i == len(text) || text[i] == ';' || text[i] == '\n' || text[i] == '\r' {
			if i > start {
				part := strings.TrimSpace(text[start:i])
				if part != "" {
					out = append(out, part)
				}
			}
			start = i + 1
		}
	}
	return out
}

func ParseFile(path string) ([]Cookie, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(string(b))
	if text == "" {
		return nil, nil
	}

	if text[0] == '{' || text[0] == '[' {
		if c, err := ParseJSON(text); err == nil && len(c) > 0 {
			return c, nil
		}
	}

	if strings.IndexByte(text, '\t') >= 0 && looksLikeNetscapeMS(text) {
		// Filter during parse so full browser dumps never materialize thousands
		// of unrelated cookies just to throw them away in FilterMS.
		if c := ParseNetscapeMS(text); len(c) > 0 {
			return c, nil
		}
	}

	return ParseHeader(text), nil
}

func looksLikeNetscapeMS(text string) bool {
	// Single scan for any MS domain marker instead of multiple Contains passes.
	n := len(text)
	if n > 10<<20 {
		n = 10 << 20
	}
	sample := text[:n]
	for marker := range msDomainSet {
		if strings.Contains(sample, marker) {
			return true
		}
	}
	return false
}

func IsLikelyCookieFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	base := filepath.Base(path)
	if strings.HasPrefix(base, ".") {
		return false
	}
	if _, skip := skipExt[strings.ToLower(filepath.Ext(path))]; skip {
		return false
	}

	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 8000)
	n, _ := f.Read(buf)
	sample := buf[:n]

	if hitMSSession(sample) {
		return true
	}
	for _, m := range [][]byte{[]byte("MSPCID"), []byte(".live.com"), []byte("login.live.com")} {
		if bytes.Contains(sample, m) {
			return true
		}
	}
	if bytes.Count(sample, []byte{'\t'}) >= 5 {
		return true
	}
	trimmed := bytes.TrimSpace(sample)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		lower := bytes.ToLower(sample)
		if bytes.Contains(lower, []byte("name")) || bytes.Contains(sample, []byte("MSP")) {
			return true
		}
	}
	return bytes.Contains(sample, []byte{'='}) && bytes.Contains(sample, []byte{';'})
}

func ToHeader(cs []Cookie) string {
	merged := make(map[string]string, len(cs))
	order := make([]string, 0, len(cs))
	for _, c := range cs {
		name := strings.TrimSpace(c.Name)
		val := sanitizeCookieValue(c.Value)
		if name == "" || val == "" || !looksLikeCookieName(name) {
			continue
		}
		if _, ok := merged[name]; !ok {
			order = append(order, name)
		}
		merged[name] = val
	}
	if len(order) == 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(len(order) * 48)
	for i, name := range order {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(name)
		b.WriteByte('=')
		b.WriteString(merged[name])
	}
	return b.String()
}

// ToMSAHeader builds a Cookie header from Microsoft-domain cookies only.
func ToMSAHeader(cs []Cookie) string {
	return ToHeader(FilterMS(cs))
}

func HasSessionCookies(cs []Cookie) bool {
	for _, c := range cs {
		if _, ok := sessionNames[c.Name]; ok {
			return true
		}
	}
	return false
}

// HasLoginDomainCookies reports cookies on login/account hosts (usable MSA surface
// even when MSPAuth isn't present in the export).
func HasLoginDomainCookies(cs []Cookie) bool {
	for _, c := range cs {
		d := strings.ToLower(c.Domain)
		if strings.Contains(d, "login.live.com") ||
			strings.Contains(d, "account.live.com") ||
			strings.Contains(d, "login.microsoftonline.com") ||
			strings.Contains(d, "microsoftonline.com") ||
			(strings.Contains(d, "live.com") && (c.Name == "MSPCID" || c.Name == "WLSSC" ||
				strings.HasPrefix(c.Name, "MSP") || strings.HasPrefix(c.Name, "__Host-MSA"))) {
			return true
		}
	}
	return false
}

func IsSessionCookieName(name string) bool {
	_, ok := sessionNames[name]
	return ok
}

func GuessEmail(cookieHeader string) string {
	return emailRe.FindString(cookieHeader)
}
