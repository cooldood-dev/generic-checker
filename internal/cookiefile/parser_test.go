package cookiefile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsMSDomain(t *testing.T) {
	yes := []string{
		".live.com",
		"login.live.com",
		"HttpOnly_.login.live.com",
		".microsoft.com",
		"account.microsoft.com",
		"xbox.com",
		".xboxlive.com",
	}
	for _, d := range yes {
		if !IsMSDomain(d) {
			t.Fatalf("expected MS domain: %q", d)
		}
	}
	no := []string{
		"google.com",
		"notlive.com",
		"evil.com",
		"",
	}
	for _, d := range no {
		if IsMSDomain(d) {
			t.Fatalf("expected non-MS domain: %q", d)
		}
	}
}

func TestParseNetscapeMSSkipsOtherDomains(t *testing.T) {
	text := strings.Join([]string{
		"# Netscape HTTP Cookie File",
		".google.com\tTRUE\t/\tFALSE\t0\tNID\tabc",
		".live.com\tTRUE\t/\tTRUE\t0\tMSPAuth\ttoken",
		"login.live.com\tFALSE\t/\tTRUE\t0\tMSPProf\tprof",
		".example.com\tTRUE\t/\tFALSE\t0\tfoo\tbar",
	}, "\n")

	cs := ParseNetscapeMS(text)
	if len(cs) != 2 {
		t.Fatalf("got %d cookies, want 2: %+v", len(cs), cs)
	}
	if !HasSessionCookies(cs) {
		t.Fatal("expected session cookies")
	}
	filtered := FilterMS(cs)
	if len(filtered) != 2 {
		t.Fatalf("FilterMS changed count: %d", len(filtered))
	}
}

func TestFilterMSEmptyDomainSession(t *testing.T) {
	cs := []Cookie{
		{Name: "MSPAuth", Value: "x", Domain: ""},
		{Name: "random", Value: "y", Domain: ""},
		{Name: "foo", Value: "z", Domain: "google.com"},
	}
	out := FilterMS(cs)
	if len(out) != 1 || out[0].Name != "MSPAuth" {
		t.Fatalf("unexpected: %+v", out)
	}
}

func TestHitMSSession(t *testing.T) {
	if !hitMSSession([]byte("foo MSPAuth bar")) {
		t.Fatal("MSPAuth")
	}
	if !hitMSSession([]byte("__Host-MSAAUTH=x")) {
		t.Fatal("__Host-MSAAUTH")
	}
	if hitMSSession([]byte("MSP CID without auth")) {
		t.Fatal("false positive")
	}
}

func TestDiscoverParallel(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "hit1.txt"), []byte(".live.com\tTRUE\t/\tTRUE\t0\tMSPAuth\tx\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "miss.txt"), []byte(".google.com\tTRUE\t/\tFALSE\t0\tNID\ty\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "hit2.txt"), []byte("MSPProf=abc; other=1\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "passwords.txt"), []byte("MSPAuth=should-skip\n"), 0o644)

	files, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d matches: %v", len(files), files)
	}

	all, err := DiscoverWith(dir, DiscoverOptions{AllFiles: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("all-files: got %d, want 3: %v", len(all), all)
	}
}

func BenchmarkFilterMS(b *testing.B) {
	cs := make([]Cookie, 5000)
	for i := range cs {
		if i%50 == 0 {
			cs[i] = Cookie{Name: "MSPAuth", Value: "x", Domain: ".live.com"}
		} else {
			cs[i] = Cookie{Name: "c", Value: "v", Domain: ".tracker.example.com"}
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FilterMS(cs)
	}
}

func BenchmarkParseNetscapeMS(b *testing.B) {
	var lines []string
	lines = append(lines, "# Netscape HTTP Cookie File")
	for i := 0; i < 5000; i++ {
		if i%50 == 0 {
			lines = append(lines, ".live.com\tTRUE\t/\tTRUE\t0\tMSPAuth\ttoken")
		} else {
			lines = append(lines, ".ads.example.com\tTRUE\t/\tFALSE\t0\tcookie\tvalue")
		}
	}
	text := strings.Join(lines, "\n")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ParseNetscapeMS(text)
	}
}
