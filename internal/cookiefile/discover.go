package cookiefile

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	// Most extract/header dumps are tiny; one small read finds MSPAuth.
	scanReadSmall = 64 << 10 // 64 KiB
	// Large Netscape tables only: keep scanning a bit deeper if the head looks like cookies.
	scanReadMax = 256 << 10 // 256 KiB
)

// Strong MSA session cookie name markers.
var msSessionMarkers = [][]byte{
	[]byte("MSPAuth"),
	[]byte("MSPProf"),
	[]byte("RPSSecAuth"),
	[]byte("__Host-MSAAUTH"),
}

// Looser MS signals: login domains / related cookie names (many dumps lack MSPAuth in the head).
var msLooseMarkers = [][]byte{
	[]byte("login.live.com"),
	[]byte(".live.com"),
	[]byte("account.live.com"),
	[]byte("login.microsoftonline.com"),
	[]byte("microsoftonline.com"),
	[]byte("microsoft.com"),
	[]byte("xboxlive.com"),
	[]byte(".xbox.com"),
	[]byte("MSPCID"),
	[]byte("WLSSC"),
	[]byte("__Host-MSAAUTHCOOKIE"),
}

var discoverBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, scanReadMax)
		return &b
	},
}

type candidate struct {
	path string
	size int64 // 0 if unknown
}

// DiscoverOptions controls how cookie files are selected before checking.
type DiscoverOptions struct {
	// AllFiles skips the MSPAuth/MSPProf content prefilter and returns every
	// cookie-like file. Each file is still filtered to MS cookies at check time.
	AllFiles bool
}

// Discover walks dir and returns paths that look like Microsoft session dumps.
func Discover(dir string) ([]string, error) {
	return DiscoverWith(dir, DiscoverOptions{})
}

// DiscoverWith walks dir recursively. Phase 1: cheap name/ext filter.
// Phase 2 (unless AllFiles): parallel content scan for MSA session markers.
func DiscoverWith(dir string, opt DiscoverOptions) ([]string, error) {
	candidates := make([]candidate, 0, 4096)
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != dir {
				// Only skip our results tree + obvious junk.
				// Do NOT skip "hits" — input dumps are often under cookies/hits/.
				switch strings.ToLower(name) {
				case "results", "node_modules", ".git",
					"wallets", "autofills", "extensions", "history", "downloads",
					"plugins", "local extension settings", "indexeddb":
					return filepath.SkipDir
				}
			}
			return nil
		}

		name := d.Name()
		lower := strings.ToLower(name)
		if strings.HasPrefix(name, ".") {
			return nil
		}
		// Stealer-log junk that is never an MS cookie dump.
		if lower == "readme" || strings.HasPrefix(lower, "readme.") ||
			lower == "passwords.txt" || lower == "information.txt" ||
			lower == "unfireproof.txt" || strings.HasSuffix(lower, "passwords.txt") ||
			lower == "log_info.json" || lower == "pc_info.json" ||
			lower == "system_info.json" || lower == "userinformation.json" ||
			lower == "tlauncherprofiles.json" {
			return nil
		}
		if _, skip := skipExt[strings.ToLower(filepath.Ext(path))]; skip {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(name))
		switch ext {
		case "", ".txt", ".cookies", ".cookie", ".json", ".jsonl":
		default:
			return nil
		}

		var size int64
		if info, err := d.Info(); err == nil {
			size = info.Size()
			if size == 0 {
				return nil
			}
		}
		candidates = append(candidates, candidate{path: path, size: size})
		return nil
	})
	if err != nil {
		return nil, err
	}

	total := len(candidates)
	if total == 0 {
		fmt.Printf("scan done: 0 files\n")
		return nil, nil
	}

	if opt.AllFiles {
		files := make([]string, len(candidates))
		for i, c := range candidates {
			files[i] = c.path
		}
		fmt.Printf("scan done: %d files (all cookie-like, no session prefilter)\n", len(files))
		return files, nil
	}

	// IO-bound on SSD/NVMe — more readers than ai_checker's CPU*2/32 cap.
	workers := runtime.NumCPU() * 4
	if workers < 8 {
		workers = 8
	}
	if workers > 96 {
		workers = 96
	}
	if workers > total {
		workers = total
	}

	jobs := make(chan int, workers*4)
	matched := make([]bool, total)
	var (
		wg      sync.WaitGroup
		done    atomic.Uint64
		matches atomic.Uint64
	)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				if hasMSSession(candidates[idx]) {
					matched[idx] = true
					matches.Add(1)
				}
				n := done.Add(1)
				if n%5000 == 0 || n == uint64(total) {
					fmt.Printf("\rscan %d/%d files (%d matches)   ", n, total, matches.Load())
				}
			}
		}()
	}
	for i := range candidates {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	files := make([]string, 0, int(matches.Load()))
	for i, ok := range matched {
		if ok {
			files = append(files, candidates[i].path)
		}
	}
	fmt.Printf("\rscan done: %d files, %d with MS cookies          \n", total, len(files))
	return files, nil
}

// HasMSSession reports whether the file contains Microsoft session markers.
func HasMSSession(path string) bool {
	return hasMSSession(candidate{path: path})
}

func hasMSSession(c candidate) bool {
	f, err := os.Open(c.path)
	if err != nil {
		return false
	}
	defer f.Close()

	bp := discoverBufPool.Get().(*[]byte)
	buf := *bp
	defer discoverBufPool.Put(bp)

	// Cap by known size so tiny extract files do a short read.
	limit := scanReadSmall
	if c.size > 0 && int(c.size) < limit {
		limit = int(c.size)
	}
	n, _ := io.ReadFull(f, buf[:limit])
	if n == 0 {
		return false
	}
	sample := buf[:n]
	if hitMSSession(sample) {
		return true
	}
	// Short file or known size fully consumed — nothing deeper.
	if c.size > 0 && c.size <= int64(n) {
		return false
	}
	if n < limit {
		return false
	}
	// Deepen scan if it's Netscape (tabs), JSON (curly/square brackets), or standard headers (semicolons).
	if !bytes.Contains(sample, []byte{'\t'}) && !bytes.Contains(sample, []byte{'{'}) && !bytes.Contains(sample, []byte{'['}) && !bytes.Contains(sample, []byte{';'}) {
		return false
	}

	// Read further into large cookie tables (still capped well below 1MiB).
	for n < scanReadMax {
		want := scanReadMax - n
		if want > scanReadSmall {
			want = scanReadSmall
		}
		m, rerr := f.Read(buf[n : n+want])
		if m > 0 {
			n += m
			if hitMSSession(buf[:n]) {
				return true
			}
		}
		if rerr != nil {
			break
		}
		if m == 0 {
			break
		}
	}
	return false
}

func hitMSSession(sample []byte) bool {
	if len(sample) == 0 {
		return false
	}
	for _, m := range msSessionMarkers {
		if bytes.Contains(sample, m) {
			return true
		}
	}
	for _, m := range msLooseMarkers {
		if bytes.Contains(sample, m) {
			return true
		}
	}
	return false
}
