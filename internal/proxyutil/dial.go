package proxyutil

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

// Dial opens a TCP connection to address through the given proxy.
// Supports http/https (CONNECT) and socks5/socks5h.
// If proxyURL is nil, a direct TCP connection is made (proxyless mode).
func Dial(ctx context.Context, proxyURL *url.URL, network, address string) (net.Conn, error) {
	if proxyURL == nil {
		// Proxyless: connect directly
		if network == "" {
			network = "tcp"
		}
		return (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, network, address)
	}
	if network == "" {
		network = "tcp"
	}
	scheme := strings.ToLower(proxyURL.Scheme)
	switch scheme {
	case "http", "https":
		return dialHTTPConnect(ctx, proxyURL, address)
	case "socks5", "socks5h":
		return dialSOCKS5(ctx, proxyURL, network, address)
	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q", proxyURL.Scheme)
	}
}

func dialHTTPConnect(ctx context.Context, proxyURL *url.URL, address string) (net.Conn, error) {
	conn, err := dialPreferIPv4(ctx, proxyURL.Host)
	if err != nil {
		return nil, fmt.Errorf("proxy dial: %w", err)
	}

	ok := false
	defer func() {
		if !ok {
			_ = conn.Close()
		}
	}()

	req := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: address},
		Host:   address,
		Header: make(http.Header),
		Proto:  "HTTP/1.1",
	}
	req = req.WithContext(ctx)
	if proxyURL.User != nil {
		user := proxyURL.User.Username()
		pass, _ := proxyURL.User.Password()
		token := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
		req.Header.Set("Proxy-Authorization", "Basic "+token)
	}
	req.Header.Set("User-Agent", "cookie_checker")

	if err := req.Write(conn); err != nil {
		return nil, fmt.Errorf("proxy CONNECT write: %w", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		return nil, fmt.Errorf("proxy CONNECT read: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("proxy CONNECT %s", resp.Status)
	}

	// If the buffered reader has leftover bytes, wrap so they are not lost.
	if br.Buffered() > 0 {
		ok = true
		return &bufConn{Conn: conn, r: br}, nil
	}
	ok = true
	return conn, nil
}

type bufConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufConn) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

func dialPreferIPv4(ctx context.Context, address string) (net.Conn, error) {
	d := net.Dialer{Timeout: 20 * time.Second}
	// Many proxy hosts publish broken AAAA records; try IPv4 first, then dual-stack.
	conn, err := d.DialContext(ctx, "tcp4", address)
	if err == nil {
		return conn, nil
	}
	return d.DialContext(ctx, "tcp", address)
}

func dialSOCKS5(ctx context.Context, proxyURL *url.URL, network, address string) (net.Conn, error) {
	var auth *proxy.Auth
	if proxyURL.User != nil {
		pass, _ := proxyURL.User.Password()
		auth = &proxy.Auth{
			User:     proxyURL.User.Username(),
			Password: pass,
		}
	}
	base := dialerFunc(func(network, addr string) (net.Conn, error) {
		return dialPreferIPv4(context.Background(), addr)
	})
	d, err := proxy.SOCKS5("tcp", proxyURL.Host, auth, base)
	if err != nil {
		return nil, err
	}
	if cd, ok := d.(proxy.ContextDialer); ok {
		return cd.DialContext(ctx, network, address)
	}
	type closeWriter interface {
		Dial(network, addr string) (net.Conn, error)
	}
	return d.Dial(network, address)
}

type dialerFunc func(network, addr string) (net.Conn, error)

func (f dialerFunc) Dial(network, addr string) (net.Conn, error) {
	return f(network, addr)
}
