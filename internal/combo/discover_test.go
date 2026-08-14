package combo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverInDir(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "passwords.txt"), []byte(
		"Soft: Chrome\nHost: https://login.live.com/\nLogin: a@live.com\nPassword: secret\n\n",
	), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "combo.txt"), []byte("b@hotmail.com:pass2\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "ulp_dump.txt"), []byte("https://account.microsoft.com/:c@outlook.com:pass3\n"), 0o644)
	// Cookie dump — must be ignored by creds scan.
	_ = os.WriteFile(filepath.Join(dir, "1.txt"), []byte(
		".live.com\tTRUE\t/\tTRUE\t0\tMSPAuth\ttoken\n.live.com\tTRUE\t/\tTRUE\t0\tMSPProf\tprof\n",
	), 0o644)

	creds, err := DiscoverInDir(dir, LoadOptions{MSOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(creds) != 3 {
		t.Fatalf("got %d: %+v", len(creds), creds)
	}
}
