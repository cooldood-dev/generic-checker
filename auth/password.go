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
const passwordClientID = "00000000402B5328"

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
	client.CheckRedirect = nil // Let standard Go client handle redirects automatically

	q := url.Values{
		"client_id":     {passwordClientID},
		"response_type": {"token"},
		"scope":         {scope},
		"redirect_uri":  {redirectURI},
		"display":       {"touch"},
		"locale":        {"en"},
	}
	startURL := authorizeURL + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, startURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("MSA login page: %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	body := string(bodyBytes)

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
		"login":        {email},
		"passwd":       {password},
		"PPFT":         {ppft},
		"ps":           {"2"},
		"canary":       {""},
		"type":         {"11"},
		"LoginOptions": {"3"},
	}

	postReq, err := http.NewRequestWithContext(ctx, http.MethodPost, postURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.Header.Set("User-Agent", userAgent)
	postReq.Header.Set("Referer", "https://login.live.com/")

	postResp, err := client.Do(postReq)
	if err != nil {
		return "", fmt.Errorf("MSA password login: %w", err)
	}
	defer postResp.Body.Close()

	finalURL := postResp.Request.URL.String()
	if tok := extractAccessToken(finalURL); tok != "" {
		return tok, nil
	}

	respBody, _ := io.ReadAll(postResp.Body)
	respStr := string(respBody)
	if tok := extractAccessToken(respStr); tok != "" {
		return tok, nil
	}

	// Classify error using Shulker's rules
	if strings.Contains(respStr, "password is incorrect") || strings.Contains(respStr, "wrong password") {
		return "", &authErr{msg: "incorrect account or password", status: StatusInvalid}
	}
	if strings.Contains(respStr, "account doesn't exist") || strings.Contains(respStr, "doesn't exist") {
		return "", &authErr{msg: "that microsoft account doesn't exist", status: StatusInvalid}
	}
	if strings.Contains(respStr, "account_locked") || strings.Contains(respStr, "account is locked") || strings.Contains(respStr, "locked") {
		return "", &authErr{msg: "account locked", status: StatusInvalid}
	}
	if strings.Contains(respStr, "verification_required") || strings.Contains(respStr, "security verification required") || strings.Contains(respStr, "identity") {
		return "", &authErr{msg: "2FA / identity challenge required", status: StatusInvalid}
	}
	if strings.Contains(respStr, "rate_limited") || strings.Contains(respStr, "too many requests") {
		return "", &authErr{msg: "Microsoft rate limited / throttled", status: StatusRateLimited}
	}

	return "", classifyMSFailure(postResp.StatusCode, respStr)
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
