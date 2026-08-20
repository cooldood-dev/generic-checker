// Package minemen implements a Minecraft-protocol ban check for eu.minemen.club.
// It follows the same pattern as the donutsmp package.
package minemen

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
	"strings"
	"time"

	"github.com/google/uuid"
	mcnet "github.com/Tnze/go-mc/net"
	"github.com/Tnze/go-mc/net/CFB8"
	pk "github.com/Tnze/go-mc/net/packet"

	"cookie_checker/internal/proxyutil"
)

const DefaultAddr = "eu.minemen.club"

// Fallback protocol version (1.21.x = 767).
const fallbackProtocol int32 = 767

// Status is the outcome of a Minemen Club ban check.
type Status string

const (
	StatusBanned      Status = "banned"
	StatusUnbanned    Status = "unbanned"
	StatusNeverJoined Status = "never_joined"
	StatusError       Status = "error"
)

// Result is the outcome of a single Minemen Club ban check.
type Result struct {
	Status Status
	Reason string
}

// ── proxy helpers ────────────────────────────────────────────────────────────

type proxyDialer struct {
	proxyURL *url.URL // nil = direct connection
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

	// SRV lookup
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
		var conn net.Conn
		if d.proxyURL == nil {
			conn, err = (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, "tcp", target)
		} else {
			conn, err = proxyutil.Dial(ctx, d.proxyURL, "tcp", target)
		}
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

// ── public API ───────────────────────────────────────────────────────────────

// Check joins eu.minemen.club and returns a ban-check Result.
// proxyURL is optional — if nil, connects directly.
func Check(ctx context.Context, accessToken, username, playerUUID string, proxyURL *url.URL) Result {
	if accessToken == "" || username == "" {
		return Result{Status: StatusError, Reason: "missing MC credentials for Minemen join"}
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
			return Result{Status: StatusUnbanned, Reason: "joined Minemen Club"}
		}
		if reason == "" {
			reason = err.Error()
		}
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

// ── internal helpers ─────────────────────────────────────────────────────────

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
	pingCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	conn, err := d.dial(pingCtx, addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(12 * time.Second))

	mc := mcnet.WrapConn(conn)
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
				return string(p.Data), fmt.Errorf("disconnect")
			}
			text := stripChat(string(reason))
			return text, fmt.Errorf("disconnect: %s", text)

		case 0x01: // Encryption Request
			if err := handleEncryption(httpClient, mc, token, id, p); err != nil {
				return "", err
			}

		case 0x02: // Login Success
			_ = mc.WritePacket(pk.Marshal(0x03))
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
		}
	}
}

func readConfigOrBan(mc *mcnet.Conn) (string, error) {
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		_ = mc.Socket.SetReadDeadline(time.Now().Add(2 * time.Second))
		var p pk.Packet
		if err := mc.ReadPacket(&p); err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				return "", nil
			}
			if strings.Contains(err.Error(), "EOF") || strings.Contains(err.Error(), "closed") {
				return "", nil
			}
			return "", err
		}
		switch p.ID {
		case 0x01:
			text := tryDisconnectText(p)
			if text != "" {
				return text, fmt.Errorf("disconnect: %s", text)
			}
		case 0x02:
			_ = mc.WritePacket(pk.Marshal(0x03))
			return "", nil
		case 0x00:
			text := tryDisconnectText(p)
			if text != "" && looksLikeKick(text) {
				return text, fmt.Errorf("disconnect: %s", text)
			}
		}
	}
	return "", nil
}

func tryDisconnectText(p pk.Packet) string {
	var s pk.String
	if p.Scan(&s) == nil && len(s) > 0 {
		return stripChat(string(s))
	}
	if len(p.Data) > 0 {
		return stripChat(string(p.Data))
	}
	return ""
}

func looksLikeKick(s string) bool {
	l := strings.ToLower(s)
	return strings.Contains(l, "ban") || strings.Contains(l, "kick") ||
		strings.Contains(l, "disconnect") || strings.Contains(l, "punish") ||
		strings.Contains(l, "version") || strings.Contains(l, "{")
}

func handleEncryption(httpClient *http.Client, mc *mcnet.Conn, token, id string, p pk.Packet) error {
	var serverID pk.String
	var publicKey pk.ByteArray
	var verifyToken pk.ByteArray
	var shouldAuth pk.Boolean
	if err := p.Scan(&serverID, &publicKey, &verifyToken, &shouldAuth); err != nil {
		if err := p.Scan(&serverID, &publicKey, &verifyToken); err != nil {
			return err
		}
		shouldAuth = true
	}
	if !shouldAuth {
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
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
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
		return fmt.Errorf("session join: %s", msg)
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

// classify maps a disconnect reason to a Status.
func classify(reason string, err error) Result {
	text := reason
	if text == "" && err != nil {
		text = err.Error()
	}
	lower := strings.ToLower(text)

	switch {
	// ── Positive ban indicators ──────────────────────────────────────────────
	case strings.Contains(lower, "banned"),
		strings.Contains(lower, "punished"),
		strings.Contains(lower, "blacklisted"),
		strings.Contains(lower, "suspended"),
		strings.Contains(lower, "tempbanned"),
		strings.Contains(lower, "permanently"),
		strings.Contains(lower, "muted"):
		return Result{Status: StatusBanned, Reason: cleanReason(text)}

	// ── Account has never played on Minemen Club ──────────────────────────
	case strings.Contains(lower, "never joined"),
		strings.Contains(lower, "hasn't joined"),
		strings.Contains(lower, "has not joined"),
		strings.Contains(lower, "not whitelisted"),
		strings.Contains(lower, "not in whitelist"),
		strings.Contains(lower, "whitelist"),
		strings.Contains(lower, "not on the whitelist"):
		return Result{Status: StatusNeverJoined, Reason: cleanReason(text)}

	// ── Transient / auth errors ───────────────────────────────────────────
	case strings.Contains(lower, "invalid session"),
		strings.Contains(lower, "session expired"),
		strings.Contains(lower, "outdated client"),
		strings.Contains(lower, "outdated server"),
		strings.Contains(lower, "please update"),
		strings.Contains(lower, "wrong version"),
		strings.Contains(lower, "multiplayer disabled"),
		strings.Contains(lower, "not authenticated"),
		strings.Contains(lower, "failed to verify"),
		strings.Contains(lower, "connection refused"),
		strings.Contains(lower, "io.timeout"),
		strings.Contains(lower, "connection timed out"),
		strings.Contains(lower, "server is full"),
		strings.Contains(lower, "too many connections"),
		strings.Contains(lower, "maintenance"),
		strings.Contains(lower, "restarting"),
		strings.Contains(lower, "shutting down"):
		return Result{Status: StatusError, Reason: cleanReason(text)}

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
