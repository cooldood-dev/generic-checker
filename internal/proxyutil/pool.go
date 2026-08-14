package proxyutil

import (
	"bufio"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
)

// Pool round-robins proxy URLs. Assign one proxy per check and keep it for the
// full Microsoft → Xbox → Minecraft request chain (sticky session).
type Pool struct {
	urls []*url.URL
	idx  atomic.Uint64
}

func LoadFile(path string) (*Pool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var urls []*url.URL
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		u, err := Parse(line)
		if err != nil {
			return nil, fmt.Errorf("proxies line %d: %w", lineNo, err)
		}
		urls = append(urls, u)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("no proxies found in %s", path)
	}
	return &Pool{urls: urls}, nil
}

func Parse(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, `"'`)
	if raw == "" {
		return nil, fmt.Errorf("empty proxy")
	}

	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return nil, err
		}
		if u.Host == "" {
			return nil, fmt.Errorf("invalid proxy URL %q", raw)
		}
		switch strings.ToLower(u.Scheme) {
		case "http", "https", "socks5", "socks5h":
			return u, nil
		default:
			return nil, fmt.Errorf("unsupported scheme %q (use http, https, socks5)", u.Scheme)
		}
	}

	// Colon-based credentials (host:port:user:pass / user:pass:host:port) must be
	// detected BEFORE the "@" URL check, because usernames/passwords often contain
	// "@" (e.g. email addresses) or other reserved characters.
	if parts := strings.Split(raw, ":"); len(parts) == 4 {
		if isPort(parts[1]) {
			// host:port:user:pass
			return &url.URL{
				Scheme: "http",
				User:   url.UserPassword(parts[2], parts[3]),
				Host:   net.JoinHostPort(parts[0], parts[1]),
			}, nil
		}
		if isPort(parts[3]) {
			// user:pass:host:port
			return &url.URL{
				Scheme: "http",
				User:   url.UserPassword(parts[0], parts[1]),
				Host:   net.JoinHostPort(parts[2], parts[3]),
			}, nil
		}
	}

	// user:pass@host:port
	if strings.Contains(raw, "@") {
		return validate(url.Parse("http://" + raw))
	}

	if parts := strings.Split(raw, ":"); len(parts) == 2 {
		// host:port
		if !isPort(parts[1]) {
			return nil, fmt.Errorf("invalid host:port %q", raw)
		}
		return validate(url.Parse("http://" + raw))
	}

	return nil, fmt.Errorf("unrecognized proxy format %q (examples: http://user:pass@host:port, host:port, host:port:user:pass, socks5://host:port)", raw)
}

func isPort(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n > 0 && n < 65536
}

func validate(u *url.URL, err error) (*url.URL, error) {
	if err != nil {
		return nil, err
	}
	if u.Host == "" {
		return nil, fmt.Errorf("invalid proxy host")
	}
	return u, nil
}

func (p *Pool) Next() *url.URL {
	if p == nil || len(p.urls) == 0 {
		return nil
	}
	i := p.idx.Add(1) - 1
	return p.urls[i%uint64(len(p.urls))]
}

func (p *Pool) Len() int {
	if p == nil {
		return 0
	}
	return len(p.urls)
}
