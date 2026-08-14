package auth

import (
	"os"
	"testing"
)

func TestExtractPPFTFromLiveSample(t *testing.T) {
	b, err := os.ReadFile("../tmp_loginprobe/login_sample.html")
	if err != nil {
		t.Skip("no live sample")
	}
	ppft := extractPPFT(string(b))
	post := extractURLPost(string(b))
	if ppft == "" {
		t.Fatal("PPFT empty on live sample")
	}
	if post == "" {
		t.Fatal("urlPost empty on live sample")
	}
	t.Logf("ppft len=%d urlPost=%s", len(ppft), post[:min(60, len(post))])
}

func TestExtractPPFTEscapedServerData(t *testing.T) {
	body := `","sFTName":"PPFT","sFTTag":"<input type=\"hidden\" name=\"PPFT\" id=\"i0327\" value=\"-DtoXL2h*gJ8HvdKkMV*o49kLNz8qvaxGRcxI4o3KbOzmhI3k*yXLDNXxWLC7Nn3UCFWBvxfdgB6k294nXgfty8swWZWcHvy9e7kGHBlw*etD5Tqd9SgRjdmGN6P7P1L!90bp5bnNrvIO!8I96N3LKJaf7T2teOof88G44YJmvbIScR5824xlIQaRW8tQmQ2wZHt5pUHF!ePzpCaP0B0bfixonF4SI07rBgiCXBn87T6uZGODH9c*LGI2vV8tD0UR6w$$\"/>","urlPost":"https://login.live.com/ppsecure/post.srf?client_id=000000004C12AE6F\u0026contextid=abc"`
	ppft := extractPPFT(body)
	if ppft == "" {
		t.Fatal("expected PPFT")
	}
	if ppft[:8] != "-DtoXL2h" {
		t.Fatalf("ppft=%q", ppft)
	}
	post := extractURLPost(body)
	if post == "" || post[:8] != "https://" {
		t.Fatalf("urlPost=%q", post)
	}
	if !containsAmp(post) {
		t.Fatalf("expected unescaped &: %q", post)
	}
}

func containsAmp(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '&' {
			return true
		}
	}
	return false
}
