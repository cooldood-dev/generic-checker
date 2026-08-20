package hypixel

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	mcnet "github.com/Tnze/go-mc/net"
	"github.com/Tnze/go-mc/net/CFB8"
	pk "github.com/Tnze/go-mc/net/packet"

	"cookie_checker/internal/proxyutil"
)

const DefaultAddr = "mc.hypixel.net"

// Fallback if status ping fails — 26.1 / current Hypixel-supported drop.
const fallbackProtocol int32 = 775

type Status string

const (
	StatusBanned      Status = "banned"
	StatusUnbanned    Status = "unbanned"
	StatusNeverJoined Status = "never_joined"
	StatusError       Status = "error"
)

type Result struct {
	Status      Status
	Reason      string
	BanDuration string // e.g. "29d 14h", "PERMANENT", or "" for non-banned / unknown
}

// banExpireRe matches the "expires in ..." clause Hypixel puts in its kick messages.
// It captures everything up to the first period or end-of-string.
var banExpireRe = regexp.MustCompile(`(?i)expires?\s+in\s+([^.]+)`)

var (
	banDaysRe    = regexp.MustCompile(`(?i)(\d+)\s*days?`)
	banHoursRe   = regexp.MustCompile(`(?i)(\d+)\s*hours?`)
	banMinutesRe = regexp.MustCompile(`(?i)(\d+)\s*minutes?`)
)

// parseBanDuration extracts the remaining ban duration from a Hypixel disconnect
// message. Returns "PERMANENT" for permanent bans, a compact "29d 14h" style
// string for temporary bans, or "" when no duration can be determined.
func parseBanDuration(text string) string {
	lower := strings.ToLower(text)
	if strings.Contains(lower, "permanent") {
		return "PERMANENT"
	}
	m := banExpireRe.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	chunk := m[1]
	var parts []string
	if d := banDaysRe.FindStringSubmatch(chunk); d != nil {
		n, _ := strconv.Atoi(d[1])
		parts = append(parts, fmt.Sprintf("%dd", n))
	}
	if h := banHoursRe.FindStringSubmatch(chunk); h != nil {
		n, _ := strconv.Atoi(h[1])
		parts = append(parts, fmt.Sprintf("%dh", n))
	}
	if min := banMinutesRe.FindStringSubmatch(chunk); min != nil {
		n, _ := strconv.Atoi(min[1])
		parts = append(parts, fmt.Sprintf("%dm", n))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ")
}

type proxyDialer struct {
	proxyURL *url.URL
}

func (d *proxyDialer) dial(ctx context.Context, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		var addrErr *net.AddrError
		if errors.As(err, &addrErr) && addrErr.Err == "missing port in address" {
			host = addr
			port = "25565"
			err = nil
		}
	}
	if err != nil {
		return nil, err
	}

	var targets []string
	if _, srvs, srvErr := net.DefaultResolver.LookupSRV(ctx, "minecraft", "tcp", host); srvErr == nil {
		for _, rec := range srvs {
			target := strings.TrimSuffix(rec.Target, ".")
			targets = append(targets, net.JoinHostPort(target, fmt.Sprintf("%d", rec.Port)))
		}
	}
	targets = append(targets, net.JoinHostPort(host, port))

	var firstErr error
	for _, target := range targets {
		conn, err := proxyutil.Dial(ctx, d.proxyURL, "tcp", target)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		return conn, nil
	}
	if firstErr == nil {
		firstErr = fmt.Errorf("no addresses for %s", addr)
	}
	return nil, firstErr
}

// Check joins Hypixel with MC credentials. proxyURL may be nil for direct (proxyless) connections.
func Check(ctx context.Context, accessToken, username, playerUUID string, proxyURL *url.URL) Result {
	if accessToken == "" || username == "" {
		return Result{Status: StatusError, Reason: "missing MC credentials for Hypixel join"}
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 35*time.Second)
		defer cancel()
	}

	d := &proxyDialer{proxyURL: proxyURL}
	httpClient := proxyHTTPClient(proxyURL, 20*time.Second)
	proto := pingProtocol(ctx, d, DefaultAddr)
	if proto == 0 {
		proto = fallbackProtocol
	}

	uid := normalizeUUID(playerUUID)
	var reason string
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		reason, err = joinOnce(ctx, d, httpClient, DefaultAddr, proto, accessToken, username, uid)
		if err == nil {
			return Result{Status: StatusUnbanned, Reason: "joined Hypixel"}
		}
		if reason == "" {
			reason = err.Error()
		}
		// Retry transient session-join / dial failures once with a fresh proxy path.
		lower := strings.ToLower(reason)
		if attempt == 0 && (strings.Contains(lower, "session join") ||
			strings.Contains(lower, "timeout") ||
			strings.Contains(lower, "proxy") ||
			strings.Contains(lower, "eof")) {
			time.Sleep(400 * time.Millisecond)
			continue
		}
		break
	}
	return classify(reason, err)
}

func proxyHTTPClient(proxyURL *url.URL, timeout time.Duration) *http.Client {
	if proxyURL == nil {
		return &http.Client{Timeout: timeout}
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return proxyutil.Dial(ctx, proxyURL, network, addr)
			},
			TLSHandshakeTimeout: 10 * time.Second,
			IdleConnTimeout:     30 * time.Second,
		},
	}
}

func pingProtocol(ctx context.Context, d *proxyDialer, addr string) int32 {
	raw, err := statusPing(ctx, d, addr)
	if err != nil {
		return 0
	}
	var st struct {
		Version struct {
			Protocol int32 `json:"protocol"`
		} `json:"version"`
	}
	if json.Unmarshal(raw, &st) != nil {
		return 0
	}
	return st.Version.Protocol
}

func statusPing(ctx context.Context, d *proxyDialer, addr string) ([]byte, error) {
	host, port := splitHostPort(addr)
	conn, err := d.dial(ctx, addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(12 * time.Second))

	mc := mcnet.WrapConn(conn)
	// Handshake intent=1 (status)
	if err := mc.WritePacket(pk.Marshal(
		0x00,
		pk.VarInt(fallbackProtocol),
		pk.String(host),
		pk.UnsignedShort(port),
		pk.VarInt(1),
	)); err != nil {
		return nil, err
	}
	if err := mc.WritePacket(pk.Marshal(0x00)); err != nil {
		return nil, err
	}
	var p pk.Packet
	if err := mc.ReadPacket(&p); err != nil {
		return nil, err
	}
	var jsonStr pk.String
	if err := p.Scan(&jsonStr); err != nil {
		return nil, err
	}
	return []byte(jsonStr), nil
}

func joinOnce(ctx context.Context, d *proxyDialer, httpClient *http.Client, addr string, protocol int32, token, name, id string) (string, error) {
	host, port := splitHostPort(addr)
	conn, err := d.dial(ctx, addr)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	}

	mc := mcnet.WrapConn(conn)
	// Handshake intent=2 (login)
	if err := mc.WritePacket(pk.Marshal(
		0x00,
		pk.VarInt(protocol),
		pk.String(host),
		pk.UnsignedShort(port),
		pk.VarInt(2),
	)); err != nil {
		return "", err
	}

	uid, err := uuid.Parse(id)
	if err != nil {
		return "", fmt.Errorf("uuid: %w", err)
	}
	// Login Start (1.19.3+ includes UUID)
	if err := mc.WritePacket(pk.Marshal(
		0x00,
		pk.String(name),
		pk.UUID(uid),
	)); err != nil {
		return "", err
	}

	for {
		var p pk.Packet
		if err := mc.ReadPacket(&p); err != nil {
			return "", err
		}
		switch p.ID {
		case 0x00: // Disconnect (login)
			var reason pk.String
			if err := p.Scan(&reason); err != nil {
				// try JSON chat component as raw string fallback
				return string(p.Data), fmt.Errorf("disconnect")
			}
			text := stripChat(string(reason))
			return text, fmt.Errorf("disconnect: %s", text)

		case 0x01: // Encryption Request
			if err := handleEncryption(httpClient, mc, token, id, p); err != nil {
				return "", err
			}

		case 0x02: // Login Success
			// Acknowledge (1.20.2+)
			_ = mc.WritePacket(pk.Marshal(0x03))
			// Read a few config packets for disconnect/ban; success if we finish or stay up briefly
			return readConfigOrBan(mc)

		case 0x03: // Set Compression
			var threshold pk.VarInt
			if err := p.Scan(&threshold); err != nil {
				return "", err
			}
			mc.SetThreshold(int(threshold))

		case 0x04: // Login Plugin Request — decline
			var msgid pk.VarInt
			var channel pk.Identifier
			var data pk.PluginMessageData
			if err := p.Scan(&msgid, &channel, &data); err != nil {
				return "", err
			}
			_ = mc.WritePacket(pk.Marshal(
				0x02,
				msgid,
				pk.OptionEncoder[pk.PluginMessageData]{Has: false},
			))

		default:
			// ignore other login packets
		}
	}
}

func readConfigOrBan(mc *mcnet.Conn) (string, error) {
	// After login success we're in configuration. Ban kicks often arrive here.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		_ = mc.Socket.SetReadDeadline(time.Now().Add(2 * time.Second))
		var p pk.Packet
		if err := mc.ReadPacket(&p); err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				// Quiet for a bit after login => treated as unbanned
				return "", nil
			}
			// connection closed without ban message after success — unbanned
			if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "closed") {
				return "", nil
			}
			return "", err
		}
		switch p.ID {
		case 0x01: // Disconnect (config) — packet ids vary; also try string scan on common disconnect
			text := tryDisconnectText(p)
			if text != "" {
				return text, fmt.Errorf("disconnect: %s", text)
			}
		case 0x02: // Finish Configuration (varies by version)
			_ = mc.WritePacket(pk.Marshal(0x03)) // acknowledge finish if this is the right id; ignore errors
			return "", nil
		case 0x00:
			text := tryDisconnectText(p)
			if text != "" && looksLikeKick(text) {
				return text, fmt.Errorf("disconnect: %s", text)
			}
		}
		// Any sustained connection past login success without ban = unbanned
		if time.Since(deadline.Add(-8*time.Second)) > 1500*time.Millisecond {
			return "", nil
		}
	}
	return "", nil
}

func tryDisconnectText(p pk.Packet) string {
	var s pk.String
	if p.Scan(&s) == nil && len(s) > 0 {
		return stripChat(string(s))
	}
	// raw JSON in packet data
	if len(p.Data) > 0 {
		return stripChat(string(p.Data))
	}
	return ""
}

func looksLikeKick(s string) bool {
	l := strings.ToLower(s)
	return strings.Contains(l, "ban") || strings.Contains(l, "kick") ||
		strings.Contains(l, "disconnect") || strings.Contains(l, "punish") ||
		strings.Contains(l, "watchdog") || strings.Contains(l, "compromised") ||
		strings.Contains(l, "version") || strings.Contains(l, "{")
}

func handleEncryption(httpClient *http.Client, mc *mcnet.Conn, token, id string, p pk.Packet) error {
	var serverID pk.String
	var publicKey pk.ByteArray
	var verifyToken pk.ByteArray
	// 1.20.5+ may append shouldAuthenticate (boolean). Ignore parse failure on older layouts.
	var shouldAuth pk.Boolean
	if err := p.Scan(&serverID, &publicKey, &verifyToken, &shouldAuth); err != nil {
		if err := p.Scan(&serverID, &publicKey, &verifyToken); err != nil {
			return err
		}
		shouldAuth = true
	}
	if !shouldAuth {
		// Offline-mode encryption request — still need shared secret, skip Mojang join.
		return enableEncryption(mc, verifyToken, publicKey, nil)
	}

	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return err
	}

	if err := sessionJoin(httpClient, token, id, string(serverID), key, publicKey); err != nil {
		return err
	}
	return enableEncryption(mc, verifyToken, publicKey, key)
}

func enableEncryption(mc *mcnet.Conn, verifyToken, publicKey, key []byte) error {
	if key == nil {
		key = make([]byte, 16)
		if _, err := rand.Read(key); err != nil {
			return err
		}
	}

	iPK, err := x509.ParsePKIXPublicKey(publicKey)
	if err != nil {
		return err
	}
	rsaKey, ok := iPK.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("encryption public key is not RSA")
	}
	encSecret, err := rsa.EncryptPKCS1v15(rand.Reader, rsaKey, key)
	if err != nil {
		return err
	}
	encVerify, err := rsa.EncryptPKCS1v15(rand.Reader, rsaKey, verifyToken)
	if err != nil {
		return err
	}
	if err := mc.WritePacket(pk.Marshal(
		0x01,
		pk.ByteArray(encSecret),
		pk.ByteArray(encVerify),
	)); err != nil {
		return err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	mc.SetCipher(CFB8.NewCFB8Encrypt(block, key), CFB8.NewCFB8Decrypt(block, key))
	return nil
}

func sessionJoin(httpClient *http.Client, accessToken, id, serverID string, secret, publicKey []byte) error {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	digest := authDigest(serverID, secret, publicKey)
	// Mojang expects selectedProfile as undashed UUID string (not {id,name}).
	body, _ := json.Marshal(map[string]string{
		"accessToken":     accessToken,
		"selectedProfile": strings.ReplaceAll(id, "-", ""),
		"serverId":        digest,
	})
	req, err := http.NewRequest(http.MethodPost, "https://sessionserver.mojang.com/session/minecraft/join", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("session join: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusNoContent {
		msg := strings.TrimSpace(string(b))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("session join auth fail: %s", msg)
	}
	return nil
}

func authDigest(serverID string, sharedSecret, publicKey []byte) string {
	h := sha1.New()
	h.Write([]byte(serverID))
	h.Write(sharedSecret)
	h.Write(publicKey)
	hash := h.Sum(nil)
	negative := (hash[0] & 0x80) == 0x80
	if negative {
		carry := true
		for i := len(hash) - 1; i >= 0; i-- {
			hash[i] = ^hash[i]
			if carry {
				carry = hash[i] == 0xff
				hash[i]++
			}
		}
	}
	res := strings.TrimLeft(hex.EncodeToString(hash), "0")
	if negative {
		res = "-" + res
	}
	return res
}

func splitHostPort(addr string) (string, uint16) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, 25565
	}
	var port uint16 = 25565
	fmt.Sscanf(portStr, "%d", &port)
	return host, port
}

func normalizeUUID(u string) string {
	u = strings.TrimSpace(u)
	u = strings.ReplaceAll(u, "-", "")
	if len(u) != 32 {
		return u
	}
	return u[0:8] + "-" + u[8:12] + "-" + u[12:16] + "-" + u[16:20] + "-" + u[20:32]
}

func stripChat(s string) string {
	if strings.HasPrefix(strings.TrimSpace(s), "{") {
		var m map[string]any
		if json.Unmarshal([]byte(s), &m) == nil {
			var b strings.Builder
			walkChat(m, &b)
			if b.Len() > 0 {
				s = b.String()
			}
		}
	}
	// strip section signs
	out := make([]rune, 0, len(s))
	skip := false
	for _, r := range s {
		if skip {
			skip = false
			continue
		}
		if r == '§' {
			skip = true
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

func walkChat(v any, b *strings.Builder) {
	switch t := v.(type) {
	case map[string]any:
		if text, ok := t["text"].(string); ok {
			b.WriteString(text)
		}
		if translate, ok := t["translate"].(string); ok {
			b.WriteString(translate)
			b.WriteByte(' ')
		}
		if extra, ok := t["extra"].([]any); ok {
			for _, e := range extra {
				walkChat(e, b)
			}
		}
		if with, ok := t["with"].([]any); ok {
			for _, e := range with {
				walkChat(e, b)
			}
		}
	case string:
		b.WriteString(t)
	case []any:
		for _, e := range t {
			walkChat(e, b)
		}
	}
}

func classify(reason string, err error) Result {
	text := reason
	if text == "" && err != nil {
		text = err.Error()
	}
	lower := strings.ToLower(text)

	switch {
	case strings.Contains(lower, "banned"),
		strings.Contains(lower, "punished"),
		strings.Contains(lower, "watchdog"),
		strings.Contains(lower, "compromised"),
		strings.Contains(lower, "suspicious activity"),
		strings.Contains(lower, "fairplay"),
		strings.Contains(lower, "security alert"),
		strings.Contains(lower, "userbannedexception"),
		strings.Contains(lower, "insufficientprivilegesexception"):
		return Result{
			Status:      StatusBanned,
			Reason:      cleanReason(text),
			BanDuration: parseBanDuration(text),
		}

	case strings.Contains(lower, "never joined"),
		strings.Contains(lower, "hasn't joined"),
		strings.Contains(lower, "has not joined"):
		return Result{Status: StatusNeverJoined, Reason: cleanReason(text)}

	default:
		return Result{Status: StatusError, Reason: cleanReason(text)}
	}
}

func cleanReason(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	if len(s) > 240 {
		s = s[:240] + "..."
	}
	return s
}
