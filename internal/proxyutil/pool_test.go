package proxyutil_test

import (
	"strings"
	"testing"

	"cookie_checker/internal/proxyutil"
)

func TestParseCoolDoodFormats(t *testing.T) {
	samples := []struct {
		raw, host, user, pass string
	}{
		{"proxy.ufpr.br:3128:danyellediosti@ufpr.br:ZJPrE9HL", "proxy.ufpr.br:3128", "danyellediosti@ufpr.br", "ZJPrE9HL"},
		{"proxy.ufpr.br:3128:mathi.oliveira@ufpr.br:#Madeus10", "proxy.ufpr.br:3128", "mathi.oliveira@ufpr.br", "#Madeus10"},
		{"proxy.ufpr.br:3128:sabrina.iancoski@ufpr.br:$tvv1420", "proxy.ufpr.br:3128", "sabrina.iancoski@ufpr.br", "$tvv1420"},
		{"proxy.ufpr.br:3128:lucasgodinho@ufpr.br:Ec9gpc4h@", "proxy.ufpr.br:3128", "lucasgodinho@ufpr.br", "Ec9gpc4h@"},
		{"proxy.ufpr.br:3128:taguchi@ufpr.br:%Igor1995", "proxy.ufpr.br:3128", "taguchi@ufpr.br", "%Igor1995"},
		{"px032004.pointtoserver.com:10780:purevpn0s13944929:e4GYmaf7s5y@", "px032004.pointtoserver.com:10780", "purevpn0s13944929", "e4GYmaf7s5y@"},
		{"px013301.pointtoserver.com:10780:purevpn0s13811607:Wb%lj!uEc5&a", "px013301.pointtoserver.com:10780", "purevpn0s13811607", "Wb%lj!uEc5&a"},
	}
	for _, s := range samples {
		u, err := proxyutil.Parse(s.raw)
		if err != nil {
			t.Fatalf("Parse(%q): %v", s.raw, err)
		}
		if u.Host != s.host {
			t.Fatalf("host: got %q want %q", u.Host, s.host)
		}
		user := u.User.Username()
		pass, _ := u.User.Password()
		if user != s.user || pass != s.pass {
			t.Fatalf("creds: got %q:%q want %q:%q\nURL=%s", user, pass, s.user, s.pass, u.String())
		}
	}
}

func TestLoadCoolDoodFile(t *testing.T) {
	p, err := proxyutil.LoadFile("../../cooldood_proxies.txt")
	if err != nil {
		t.Fatal(err)
	}
	if p.Len() < 700 {
		t.Fatalf("expected ~764 proxies, got %d", p.Len())
	}
}

func TestAtSignNotMisparsedAsURL(t *testing.T) {
	// Old bug: treating host:port:email@domain:pass as userinfo@host
	raw := "proxy.ufpr.br:3128:danyellediosti@ufpr.br:ZJPrE9HL"
	u, err := proxyutil.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(u.Host, "ufpr.br") && !strings.HasPrefix(u.Host, "proxy.ufpr.br") {
		t.Fatalf("misparsed host as %q", u.Host)
	}
	if u.User.Username() != "danyellediosti@ufpr.br" {
		t.Fatalf("username=%q", u.User.Username())
	}
}
