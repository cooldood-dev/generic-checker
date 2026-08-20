package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"cookie_checker/internal/license"
) // App is the Wails backend bound to the frontend.
type App struct {
	ctx       context.Context
	startTime time.Time

	mu         sync.Mutex
	cfg        CheckConfig
	running    bool
	cancel     context.CancelFunc
	resultsDir string

	// checkSem (capacity 1) serializes checker pipelines so the global state
	// used by runChecker is never mutated concurrently (manual runs and the
	// Telegram Auto cookie checks).
	checkSem chan struct{}

	Auth *license.Manager

	logMu     sync.Mutex
	logBuf    []map[string]string
	logFlush  *time.Ticker
	statsLast time.Time
}

// SystemStatus represents high-level system health metrics
type SystemStatus struct {
	Status      string `json:"status"`
	Uptime      string `json:"uptime"`
	LastChecked string `json:"lastChecked"`
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		startTime: time.Now(),
		cfg:       defaultCheckConfig(),
		checkSem:  make(chan struct{}, 1),
		Auth:      license.NewManager("."),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.Auth.Startup(ctx)
	a.loadConfigFile()
	a.logFlush = time.NewTicker(150 * time.Millisecond)
	go a.logFlusher()
}

func (a *App) logFlusher() {
	for range a.logFlush.C {
		a.flushLogs()
	}
}

func (a *App) flushLogs() {
	a.logMu.Lock()
	if len(a.logBuf) == 0 {
		a.logMu.Unlock()
		return
	}
	batch := a.logBuf
	a.logBuf = nil
	a.logMu.Unlock()
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "log:batch", batch)
	}
}

func configPath() string {
	return "config.json"
}

func (a *App) loadConfigFile() {
	b, err := os.ReadFile(configPath())
	if err != nil {
		return
	}
	var cfg CheckConfig
	if json.Unmarshal(b, &cfg) == nil {
		a.mu.Lock()
		a.cfg = cfg.normalized()
		a.mu.Unlock()
	}
}

func (a *App) persistConfigLocked() {
	b, err := json.MarshalIndent(a.cfg.normalized(), "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(configPath(), b, 0o644)
}

// GetSystemStatus returns the current system connection/health status
func (a *App) GetSystemStatus() SystemStatus {
	uptimeDuration := time.Since(a.startTime).Truncate(time.Second)
	return SystemStatus{
		Status:      "healthy",
		Uptime:      uptimeDuration.String(),
		LastChecked: time.Now().Format("15:04:05 PM"),
	}
}

// GetConfig returns the current checker configuration.
func (a *App) GetConfig() CheckConfig {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.normalized()
}

// SaveConfig persists configuration for the next StartChecking call.
func (a *App) SaveConfig(cfg CheckConfig) CheckConfig {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg = cfg.normalized()
	a.persistConfigLocked()
	a.emitLog("info", "Configuration saved.")
	return a.cfg
}

// IsChecking reports whether a run is in progress.
func (a *App) IsChecking() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

// StartChecking begins a checker run with the given config (or last saved).
func (a *App) StartChecking(cfg CheckConfig) error {
	// auth state check temporarily bypassed for testing
	// state := a.Auth.GetAuthenticationState()
	// if !state.Valid {
	// 	return fmt.Errorf("unauthorized: license is not valid (state: %+v)", state)
	// }

	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return fmt.Errorf("already checking")
	}
	normalized := cfg.normalized()
	saved := a.cfg.normalized()
	if normalized.CookiesDir == "" {
		normalized.CookiesDir = saved.CookiesDir
	}
	if normalized.ProxyFile == "" {
		normalized.ProxyFile = saved.ProxyFile
	}
	if normalized.Workers < 1 {
		normalized.Workers = saved.Workers
	}
	if normalized.TimeoutSec < 1 {
		normalized.TimeoutSec = saved.TimeoutSec
	}
	if normalized.DiscordWebhook == "" {
		normalized.DiscordWebhook = saved.DiscordWebhook
	}
	a.cfg = normalized
	a.persistConfigLocked()

	runCtx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.running = true
	a.mu.Unlock()

	a.emitLog("info", "Started checking pipeline.")

	go func() {
		// Serialize with Telegram Auto cookie checks; skip if one is running.
		select {
		case a.checkSem <- struct{}{}:
			defer func() { <-a.checkSem }()
		default:
			a.mu.Lock()
			a.running = false
			a.mu.Unlock()
			a.emitLog("error", "Another checking pipeline is already running.")
			return
		}

		hooks := CheckHooks{
			OnLog: func(level, message string) {
				a.emitLog(level, message)
			},
			OnStats: func(stats CheckStats) {
				// Throttle UI stats to ~4 Hz — high CPM was flooding the frontend.
				a.mu.Lock()
				if time.Since(a.statsLast) < 250*time.Millisecond {
					a.mu.Unlock()
					return
				}
				a.statsLast = time.Now()
				a.mu.Unlock()
				if a.ctx != nil {
					wailsruntime.EventsEmit(a.ctx, "stats:update", stats)
				}
			},
			OnHit: func(hit LiveHit) {
				if a.ctx != nil {
					wailsruntime.EventsEmit(a.ctx, "hit:new", hit)
				}
			},
			OnDone: func(stats CheckStats) {
				a.flushLogs()
				if a.ctx != nil {
					wailsruntime.EventsEmit(a.ctx, "stats:update", stats)
					wailsruntime.EventsEmit(a.ctx, "run:done", stats)
				}
			},
			PrintTerminal: false,
		}

		err := runChecker(runCtx, normalized, hooks)

		a.mu.Lock()
		a.running = false
		a.cancel = nil
		a.resultsDir = resultsDir
		a.mu.Unlock()

		if err != nil && runCtx.Err() == nil {
			a.emitLog("error", err.Error())
		} else if runCtx.Err() != nil {
			a.emitLog("warn", "Checking stopped.")
		} else {
			a.emitLog("success", "Checking run complete.")
		}
		a.flushLogs()
		if a.ctx != nil {
			wailsruntime.EventsEmit(a.ctx, "run:done", map[string]any{"ok": true})
		}
	}()

	return nil
}

// StopChecking cancels the active run.
func (a *App) StopChecking() {
	a.mu.Lock()
	cancel := a.cancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
		a.emitLog("info", "Stop requested.")
		a.flushLogs()
	}
}

// OpenResults opens the latest results directory in the OS file manager.
func (a *App) OpenResults() error {
	a.mu.Lock()
	dir := a.resultsDir
	a.mu.Unlock()
	if dir == "" {
		dir = resultsDir
	}
	if dir == "" {
		entries, err := os.ReadDir(resultsRoot)
		if err != nil || len(entries) == 0 {
			return fmt.Errorf("no results directory yet")
		}
		dir = filepath.Join(resultsRoot, entries[len(entries)-1].Name())
	}
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("results not found: %s", dir)
	}
	a.emitLog("info", "Opened results: "+dir)
	a.flushLogs()
	return openPath(dir)
}

func openPath(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}

func (a *App) emitLog(level, message string) {
	a.logMu.Lock()
	a.logBuf = append(a.logBuf, map[string]string{
		"id":        fmt.Sprintf("%d", time.Now().UnixNano()),
		"timestamp": time.Now().Format("15:04:05"),
		"level":     level,
		"message":   message,
	})
	// Cap buffer so a stuck flusher can't blow memory.
	if len(a.logBuf) > 500 {
		a.logBuf = a.logBuf[len(a.logBuf)-300:]
	}
	a.logMu.Unlock()
}

// MinimiseWindow minimises the application window
func (a *App) MinimiseWindow() {
	if a.ctx != nil {
		wailsruntime.WindowMinimise(a.ctx)
	}
}

// ToggleMaximiseWindow toggles window maximisation
func (a *App) ToggleMaximiseWindow() {
	if a.ctx != nil {
		wailsruntime.WindowToggleMaximise(a.ctx)
	}
}

// CloseWindow closes the application
func (a *App) CloseWindow() {
	if a.ctx != nil {
		wailsruntime.Quit(a.ctx)
	}
}

// GetAuthenticationState returns the current auth state
func (a *App) GetAuthenticationState() license.AuthState {
	return a.Auth.GetAuthenticationState()
}

// ActivateLicense attempts to bind a new license key
func (a *App) ActivateLicense(key string) error {
	return a.Auth.ActivateLicense(key)
}

// LogoutLicense revokes local auth state
func (a *App) LogoutLicense() error {
	return a.Auth.Logout()
}
