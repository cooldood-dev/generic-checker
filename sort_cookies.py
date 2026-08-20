#!/usr/bin/env python3
"""
Sort Microsoft cookies out of stealer dumps (Python port of `go run . extract`).

Walks cookie-like files under an input directory, keeps Microsoft cookies,
and writes numbered Netscape files: 1.txt, 2.txt, ...
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

MS_DOMAINS = frozenset(
    {
        "live.com",
        "microsoft.com",
        "xbox.com",
        "xboxlive.com",
        "office.com",
        "microsoftonline.com",
        "msn.com",
        "outlook.com",
        "hotmail.com",
        "passport.com",
        "account.microsoft",
        "login.microsoft",
        "bing.com",
    }
)

SESSION_NAMES = frozenset(
    {
        "MSPAuth",
        "MSPProf",
        "RPSSecAuth",
        "__Host-MSAAUTH",
        "__Host-MSAAUTHCOOKIE",
        "MSPCID",
        "WLSSC",
    }
)

COOKIE_EXTS = frozenset({".txt", ".cookies", ".cookie", ".json", ".jsonl"})
COOKIE_NAMES = frozenset({"cookies", "cookie", "cookies.txt", "cookie.txt"})
PROGRESS_EVERY = 5000


def normalize_domain(domain: str) -> str:
    if not domain:
        return ""
    if domain.startswith("."):
        domain = domain[1:]
    if len(domain) >= 9 and domain[:9].lower() == "httponly_":
        domain = domain[9:]
        if domain.startswith("."):
            domain = domain[1:]
    return domain.lower()


def is_ms_domain(domain: str) -> bool:
    d = normalize_domain(domain)
    if not d:
        return False
    while True:
        if d in MS_DOMAINS:
            return True
        i = d.find(".")
        if i < 0:
            return False
        d = d[i + 1 :]


def is_session_name(name: str) -> bool:
    return name in SESSION_NAMES


def keep_empty_domain(name: str) -> bool:
    if is_session_name(name) or name.startswith("MSP"):
        return True
    return "msa" in name.lower()


def under(path: Path, parent: Path) -> bool:
    try:
        path.resolve().relative_to(parent.resolve())
        return True
    except ValueError:
        return False


def highest_numbered(out_dir: Path) -> int:
    max_n = 0
    if not out_dir.is_dir():
        return 0
    for p in out_dir.iterdir():
        if not p.is_file() or p.suffix.lower() != ".txt":
            continue
        try:
            n = int(p.stem)
        except ValueError:
            continue
        if n > max_n:
            max_n = n
    return max_n


def read_text(path: Path) -> str:
    data = path.read_bytes()
    if not data:
        return ""
    if data.startswith(b"\xff\xfe") or data.startswith(b"\xfe\xff"):
        return data.decode("utf-16", errors="replace")
    if data.startswith(b"\xef\xbb\xbf"):
        return data.decode("utf-8-sig", errors="replace")
    if data.count(b"\x00") > max(16, len(data) // 8):
        try:
            return data.decode("utf-16-le", errors="replace")
        except UnicodeDecodeError:
            pass
    try:
        return data.decode("utf-8")
    except UnicodeDecodeError:
        return data.decode("cp1252", errors="replace")


def looks_like_cookie_name(name: str) -> bool:
    if not name or len(name) >= 128:
        return False
    for c in name:
        if not (c.isalnum() or c in "_.-"):
            return False
    return True


def to_netscape_line(domain: str, name: str, value: str, path: str = "/") -> str:
    if not domain:
        domain = ".live.com"
    if not path:
        path = "/"
    value = value.replace("\r", "").replace("\n", "").replace("\0", "")
    return f"{domain}\tTRUE\t{path}\tTRUE\t0\t{name}\t{value}"


def netscape_fields(line: str):
    if "\t" in line:
        parts = line.split("\t")
    else:
        parts = line.split()
        if len(parts) != 7:
            return None
    if len(parts) < 7:
        return None
    domain, _flag, path, _secure, _exp, name = parts[:6]
    value = "\t".join(parts[6:]) if "\t" in line else parts[6]
    if not name:
        return None
    return domain, path or "/", name, value


def parse_netscape_ms(text: str):
    lines = []
    seen = set()
    for raw in text.splitlines():
        line = raw.strip()
        if not line:
            continue
        working = line
        if working[0] == "#":
            if len(working) > 10 and working[:10].lower() == "#httponly_":
                working = working[10:]
            else:
                continue
        fields = netscape_fields(working)
        if not fields:
            continue
        domain, path, name, value = fields
        if not value or not is_ms_domain(domain):
            continue
        lines.append(to_netscape_line(domain, name, value, path))
        if is_session_name(name):
            seen.add(name)
    return lines, sorted(seen)


def parse_json_ms(text: str):
    text = text.strip()
    if not text or text[0] not in "{[":
        return [], []
    try:
        data = json.loads(text)
    except json.JSONDecodeError:
        return [], []

    items = []
    if isinstance(data, list):
        items = [x for x in data if isinstance(x, dict)]
    elif isinstance(data, dict):
        raw = data.get("cookies")
        if isinstance(raw, list):
            items = [x for x in raw if isinstance(x, dict)]
        else:
            lines, seen = [], set()
            for name, value in data.items():
                if not isinstance(value, str) or not looks_like_cookie_name(str(name)):
                    continue
                if not (is_session_name(str(name)) or keep_empty_domain(str(name))):
                    continue
                lines.append(to_netscape_line(".live.com", str(name), value))
                if is_session_name(str(name)):
                    seen.add(str(name))
            return lines, sorted(seen)
    else:
        return [], []

    lines, seen = [], set()
    for item in items:
        name = str(item.get("name") or item.get("Name") or "")
        value = item.get("value") if "value" in item else item.get("Value")
        if value is None:
            continue
        value = str(value)
        if not name or not value:
            continue
        domain = str(item.get("domain") or item.get("Domain") or ".live.com")
        path = str(item.get("path") or item.get("Path") or "/")
        if domain:
            if not is_ms_domain(domain):
                continue
        elif not keep_empty_domain(name):
            continue
        lines.append(to_netscape_line(domain or ".live.com", name, value, path))
        if is_session_name(name):
            seen.add(name)
    return lines, sorted(seen)


def parse_header_ms(text: str):
    lines, seen = [], set()
    for chunk in text.replace("\r", "\n").replace(";", "\n").split("\n"):
        chunk = chunk.strip()
        if not chunk or "=" not in chunk:
            continue
        if chunk.lower().startswith("cookie:"):
            chunk = chunk.split(":", 1)[1].strip()
        name, _, value = chunk.partition("=")
        name, value = name.strip(), value.strip()
        if not looks_like_cookie_name(name) or not value:
            continue
        if not (is_session_name(name) or keep_empty_domain(name)):
            continue
        lines.append(to_netscape_line(".live.com", name, value))
        if is_session_name(name):
            seen.add(name)
    return lines, sorted(seen)


def extract_ms(text: str):
    lines, sessions = parse_netscape_ms(text)
    if lines:
        return lines, sessions
    lines, sessions = parse_json_ms(text)
    if lines:
        return lines, sessions
    lower_head = text[:65536].lower()
    markers = (
        "mspauth", "mspprof", "rpssecauth", "__host-msaauth",
        "mspcid", "wlssc", "login.live.com", "microsoftonline.com",
    )
    if any(m in lower_head for m in markers):
        return parse_header_ms(text)
    return [], []


def is_cookie_candidate(path: Path) -> bool:
    name = path.name
    if name.startswith("."):
        return False
    lower = name.lower()
    if lower in {
        "readme", "passwords.txt", "information.txt", "unfireproof.txt",
        "log_info.json", "pc_info.json", "system_info.json",
        "userinformation.json", "tlauncherprofiles.json",
    } or lower.startswith("readme.") or lower.endswith("passwords.txt"):
        return False
    ext = path.suffix.lower()
    if ext in COOKIE_EXTS:
        return True
    if ext == "" and lower in COOKIE_NAMES:
        return True
    return False


def write_cookie_file(dest: Path, source: Path, lines, sessions) -> None:
    sess = ", ".join(sessions) if sessions else "none"
    parts = [
        "# Netscape HTTP Cookie File",
        f"# source: {source}",
        f"# ms_cookies: {len(lines)}",
        f"# session: {sess}",
        *lines,
        "",
    ]
    dest.write_text("\n".join(parts), encoding="utf-8", errors="replace")


def run(root: Path, out_dir: Path, require_session: bool) -> None:
    root = root.resolve()
    out_dir = out_dir.resolve()
    out_dir.mkdir(parents=True, exist_ok=True)
    skip_under = out_dir if under(out_dir, root) else None
    next_n = highest_numbered(out_dir) + 1
    scanned = written = empty = unreadable = no_ms = skipped_no_session = 0
    print(f"extracting from {root} ...", flush=True)

    for path in root.rglob("*"):
        if skip_under is not None and under(path, skip_under) and path != root:
            continue
        if not path.is_file() or not is_cookie_candidate(path):
            continue
        scanned += 1
        if scanned % PROGRESS_EVERY == 0:
            print(
                f"  extract {scanned} scanned, {written} written "
                f"(no_ms={no_ms}, empty={empty}, skip_session={skipped_no_session})",
                flush=True,
            )
        try:
            text = read_text(path)
        except OSError:
            unreadable += 1
            continue
        if not text.strip():
            empty += 1
            continue
        lines, sessions = extract_ms(text)
        if not lines:
            no_ms += 1
            continue
        if require_session and not sessions:
            skipped_no_session += 1
            continue
        dest = out_dir / f"{next_n}.txt"
        next_n += 1
        try:
            write_cookie_file(dest, path, lines, sessions)
        except OSError as e:
            print(f"write failed {dest}: {e}", file=sys.stderr)
            break
        written += 1

    print(
        f"done: wrote {written} -> {out_dir}\n"
        f"  scanned={scanned} no_ms={no_ms} empty={empty} "
        f"unreadable={unreadable} skip_session={skipped_no_session}",
        flush=True,
    )


def main() -> int:
    ap = argparse.ArgumentParser(
        description="Crawl dumps and write Microsoft cookies as 1.txt, 2.txt, ..."
    )
    ap.add_argument("dump_dir", nargs="?", default=".", help="directory to crawl")
    ap.add_argument("-o", "--out", default="cookies", help="output folder")
    ap.add_argument(
        "--all-ms",
        action="store_true",
        help="keep files with any MS-domain cookies (default: session required)",
    )
    args = ap.parse_args()
    root = Path(args.dump_dir)
    if not root.is_dir():
        print(f"Not a directory: {root}", file=sys.stderr)
        return 1
    run(root=root, out_dir=Path(args.out), require_session=not args.all_ms)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
