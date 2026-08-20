# Generic Checker

Wails (Go + React) desktop app for checking Microsoft session cookies / credentials against Minecraft auth, with optional Hypixel status.

## Requirements

- Go 1.21+
- Node.js 18+
- [Wails v2](https://wails.io) (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)
- WebView2 (Windows)

## Setup

```bash
cp config.example.json config.json
cp proxies.example.txt proxies.txt
# edit proxies.txt with your proxies

cd frontend && npm install && cd ..
```

## Build / run

```bash
# GUI (production build)
wails build
./build/bin/generic-checker   # or generic-checker.exe on Windows

# GUI (dev)
wails dev

# CLI (same binary, with flags)
./generic-checker -dir cookies -proxies proxies.txt -w 100
```

Put cookie dumps under `cookies/`. Run outputs go to `results/` (gitignored).

## Helpers

- `sort_cookies.py` — extract MS Netscape cookies from dump folders into numbered files
- `tg_scrape_txt.py` — Telethon helper to pull `.txt` files from Telegram channels

## License

Private / unlicensed unless you add one.
