package combo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSplitCombo(t *testing.T) {
	email, pass, ok := splitCombo("a@b.com:secret:extra")
	if !ok || email != "a@b.com" || pass != "secret:extra" {
		t.Fatalf("got %q %q %v", email, pass, ok)
	}
	if _, _, ok := splitCombo("D&>W p>A@Uk:binary\x00junk"); ok {
		t.Fatal("accepted binary garbage")
	}
	if _, _, ok := splitCombo("not-an-email:pass"); ok {
		t.Fatal("accepted invalid email")
	}
}

func TestSplitULP(t *testing.T) {
	u, login, pass, ok := splitULP("https://login.live.com/:user@mail.com:p@ssword")
	if !ok {
		t.Fatal("expected ok")
	}
	if u != "https://login.live.com/" || login != "user@mail.com" || pass != "p@ssword" {
		t.Fatalf("got %q %q %q", u, login, pass)
	}
}

func TestLoadULPFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ulp.txt")
	body := "" +
		"https://login.live.com/:a@live.com:pass1\n" +
		"https://google.com/:b@gmail.com:pass2\n" +
		"Soft: Chrome\n" +
		"Host: https://account.microsoft.com/\n" +
		"Login: c@outlook.com\n" +
		"Password: pass3\n" +
		"\n" +
		"d@hotmail.com:pass4\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	creds, err := LoadULPFile(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(creds) != 3 {
		t.Fatalf("ms-only got %d: %+v", len(creds), creds)
	}
}

func TestLoadComboFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "combo.txt")
	if err := os.WriteFile(path, []byte("a@b.com:one\n# skip\nbadline\nc@d.com:two:three\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	creds, err := LoadComboFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(creds) != 2 {
		t.Fatalf("got %d", len(creds))
	}
	if creds[1].Password != "two:three" {
		t.Fatalf("password=%q", creds[1].Password)
	}
}

func TestLoadAnyMixedFormats(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mixed.txt")
	body := "" +
		"plain@live.com:comboPass\n" +
		"https://login.live.com/:ulp@live.com:ulpPass\n" +
		"Soft: Chrome\nHost: https://xbox.com/\nLogin: block@outlook.com\nPassword: blockPass\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	creds, err := LoadAnyFile(path, LoadOptions{MSOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(creds) != 3 {
		t.Fatalf("got %d: %+v", len(creds), creds)
	}
}

func TestMerge(t *testing.T) {
	a := []Cred{{Email: "a@b.com", Password: "1"}, {Email: "c@d.com", Password: "2"}}
	b := []Cred{{Email: "a@b.com", Password: "1"}, {Email: "e@f.com", Password: "3"}}
	out := Merge(a, b)
	if len(out) != 3 {
		t.Fatalf("got %d", len(out))
	}
}
