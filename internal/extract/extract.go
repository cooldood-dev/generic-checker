package extract

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"cookie_checker/internal/cookiefile"
)

type Options struct {
	Root           string
	OutDir         string
	RequireSession bool // if true, only write files with MSPAuth/MSPProf/etc
}

type Stats struct {
	Scanned int
	MSHits  int
	Written int
}

func under(path, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// ParseNetscapeMS keeps Netscape lines for Microsoft domains and returns
// session cookie names found among them.
func ParseNetscapeMS(text string) (lines []string, sessions []string) {
	seen := map[string]struct{}{}
	lines = make([]string, 0, 32)

	for len(text) > 0 {
		var raw string
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			raw = text[:i]
			text = text[i+1:]
		} else {
			raw = text
			text = ""
		}
		if n := len(raw); n > 0 && raw[n-1] == '\r' {
			raw = raw[:n-1]
		}

		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		working := line
		if working[0] == '#' {
			if len(working) > 10 && strings.EqualFold(working[:10], "#httponly_") {
				working = working[10:]
			} else {
				continue
			}
		}

		domain, name, ok := netscapeDomainName(working)
		if !ok || name == "" || !cookiefile.IsMSDomain(domain) {
			continue
		}
		lines = append(lines, line)
		if cookiefile.IsSessionCookieName(name) {
			seen[name] = struct{}{}
		}
	}

	if len(seen) > 0 {
		sessions = make([]string, 0, len(seen))
		for name := range seen {
			sessions = append(sessions, name)
		}
		sort.Strings(sessions)
	}
	return lines, sessions
}

func netscapeDomainName(line string) (domain, name string, ok bool) {
	// Need fields 0 (domain) and 5 (name); stop once name is found.
	field := 0
	start := 0
	for i := 0; i <= len(line); i++ {
		if i == len(line) || line[i] == '\t' {
			switch field {
			case 0:
				domain = line[start:i]
			case 5:
				name = line[start:i]
				return domain, name, true
			}
			field++
			start = i + 1
			if field > 5 {
				break
			}
		}
	}
	return "", "", false
}

func highestNumbered(outDir string) (int, error) {
	entries, err := os.ReadDir(outDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	maxN := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".txt") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSuffix(name, ".txt"))
		if err != nil || n <= 0 {
			continue
		}
		if n > maxN {
			maxN = n
		}
	}
	return maxN, nil
}

// Run walks every .txt under Root and writes numbered MS cookie files to OutDir.
func Run(opt Options) (Stats, error) {
	var st Stats
	root, err := filepath.Abs(opt.Root)
	if err != nil {
		return st, err
	}
	outDir, err := filepath.Abs(opt.OutDir)
	if err != nil {
		return st, err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return st, err
	}

	var skipDir string
	if under(outDir, root) {
		skipDir = outDir
	}

	maxN, err := highestNumbered(outDir)
	if err != nil {
		return st, err
	}
	nextN := maxN + 1

	const progressEvery = 5000
	fmt.Printf("extracting from %s ...\n", root)

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if skipDir != "" && under(path, skipDir) && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") || !strings.EqualFold(filepath.Ext(name), ".txt") {
			return nil
		}
		if skipDir != "" && under(path, skipDir) {
			return nil
		}

		st.Scanned++
		if st.Scanned%progressEvery == 0 {
			fmt.Printf("  extract %d scanned, %d written\n", st.Scanned, st.Written)
		}

		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lines, sessions := ParseNetscapeMS(string(b))
		if len(lines) == 0 {
			return nil
		}
		st.MSHits++
		if opt.RequireSession && len(sessions) == 0 {
			return nil
		}

		dest := filepath.Join(outDir, fmt.Sprintf("%d.txt", nextN))
		nextN++

		sess := "none"
		if len(sessions) > 0 {
			sess = strings.Join(sessions, ", ")
		}
		var sb strings.Builder
		sb.Grow(128 + len(lines)*64)
		fmt.Fprintf(&sb, "# Netscape HTTP Cookie File\n")
		fmt.Fprintf(&sb, "# source: %s\n", path)
		fmt.Fprintf(&sb, "# ms_cookies: %d\n", len(lines))
		fmt.Fprintf(&sb, "# session: %s\n", sess)
		for i, line := range lines {
			if i > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(line)
		}
		sb.WriteByte('\n')
		if err := os.WriteFile(dest, []byte(sb.String()), 0o644); err != nil {
			return err
		}
		st.Written++
		return nil
	})
	return st, err
}
