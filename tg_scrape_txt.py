#!/usr/bin/env python3
"""
Download every .txt document posted in Telegram channels/groups
your account can see, from the last N hours.

Uses asyncio concurrency (not OS threads) — required by Telethon —
with capped parallelism + FloodWait handling so we stay within Telegram limits.

Setup:
  1. pip install telethon
  2. api_id + api_hash from https://my.telegram.org/apps
  3. python tg_scrape_txt.py --api-id ... --api-hash ...
"""

from __future__ import annotations

import argparse
import asyncio
import os
import re
import sys
import time
from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone
from pathlib import Path

try:
    from telethon import TelegramClient
    from telethon.errors import FloodWaitError, RPCError
    from telethon.tl.types import (
        Channel,
        Chat,
        DocumentAttributeFilename,
        InputMessagesFilterDocument,
        MessageMediaDocument,
        User,
    )
except ImportError:
    print("Install Telethon first:  pip install telethon", file=sys.stderr)
    raise SystemExit(1)

SAFE_NAME = re.compile(r"[^\w.\-]+", re.UNICODE)


def safe_filename(name: str) -> str:
    name = name.strip().replace("\0", "")
    name = SAFE_NAME.sub("_", name)
    name = name.strip("._") or "file"
    if not name.lower().endswith(".txt"):
        name += ".txt"
    return name[:180]


def doc_filename(message) -> str | None:
    media = message.media
    if not isinstance(media, MessageMediaDocument) or media.document is None:
        return None
    for attr in media.document.attributes or []:
        if isinstance(attr, DocumentAttributeFilename) and attr.file_name:
            return attr.file_name
    if (media.document.mime_type or "").startswith("text/"):
        return f"msg_{message.id}.txt"
    return None


def doc_size(message) -> int:
    media = message.media
    if not isinstance(media, MessageMediaDocument) or media.document is None:
        return 0
    return int(getattr(media.document, "size", 0) or 0)


def is_txt_name(name: str) -> bool:
    return name.lower().endswith(".txt")


def msg_aware(dt: datetime) -> datetime:
    if dt.tzinfo is None:
        return dt.replace(tzinfo=timezone.utc)
    return dt


@dataclass
class Stats:
    channels: int = 0
    found: int = 0
    saved: int = 0
    skipped: int = 0
    skipped_large: int = 0
    failed: int = 0
    lock: asyncio.Lock = field(default_factory=asyncio.Lock)

    async def add(self, **kwargs: int) -> None:
        async with self.lock:
            for k, v in kwargs.items():
                setattr(self, k, getattr(self, k) + v)


async def with_flood_wait(coro_factory, *, label: str = ""):
    """Retry on FloodWait / transient RPC errors without busy-looping."""
    while True:
        try:
            return await coro_factory()
        except FloodWaitError as e:
            wait = int(e.seconds) + 1
            print(f"  FloodWait {wait}s {label}".rstrip())
            await asyncio.sleep(wait)
        except RPCError as e:
            # soft retry a couple transient cases
            msg = str(e).lower()
            if "timeout" in msg or "wait" in msg:
                await asyncio.sleep(2)
                continue
            raise


async def download_one(
    client: TelegramClient,
    message,
    dest: Path,
    dl_sem: asyncio.Semaphore,
    stats: Stats,
    delay: float,
) -> None:
    async with dl_sem:
        dest.parent.mkdir(parents=True, exist_ok=True)

        async def _do():
            return await client.download_media(message, file=str(dest))

        try:
            path = await with_flood_wait(_do, label=f"dl {dest.name}")
            if path:
                await stats.add(saved=1)
                print(f"  saved {Path(path).name}")
            if delay > 0:
                await asyncio.sleep(delay)
        except Exception as e:
            await stats.add(failed=1)
            print(f"  FAIL {dest.name}: {e}")


async def scrape_channel(
    client: TelegramClient,
    entity,
    title: str,
    chat_dir: Path,
    since: datetime,
    args: argparse.Namespace,
    chan_sem: asyncio.Semaphore,
    dl_sem: asyncio.Semaphore,
    stats: Stats,
) -> None:
    async with chan_sem:
        await stats.add(channels=1)
        n = stats.channels
        print(f"[{n}] {title}")
        downloads: list[asyncio.Task] = []

        try:
            async def _iter():
                # Document filter = far fewer messages to walk than full history.
                return client.iter_messages(
                    entity,
                    filter=InputMessagesFilterDocument,
                )

            it = await with_flood_wait(_iter, label=f"open {title}")
            async for message in it:
                if message.date is None:
                    continue
                if msg_aware(message.date) < since:
                    break

                name = doc_filename(message)
                if not name or not is_txt_name(name):
                    continue

                size = doc_size(message)
                if size > args.max_bytes:
                    await stats.add(skipped_large=1)
                    mb = size / (1024 * 1024)
                    print(f"  skip large {name} ({mb:.1f} MB > {args.max_bytes // (1024*1024)} MB)")
                    continue

                await stats.add(found=1)
                dest = chat_dir / f"{message.id}_{safe_filename(name)}"
                if dest.exists() and not args.force:
                    await stats.add(skipped=1)
                    continue

                downloads.append(
                    asyncio.create_task(
                        download_one(client, message, dest, dl_sem, stats, args.delay)
                    )
                )
                # Bound in-flight download tasks per channel to avoid huge queues.
                if len(downloads) >= args.downloads * 3:
                    await asyncio.gather(*downloads)
                    downloads.clear()

            if downloads:
                await asyncio.gather(*downloads)
        except Exception as e:
            print(f"  channel error ({title}): {e}")


async def scrape(args: argparse.Namespace) -> int:
    api_id = args.api_id or int(os.environ.get("TG_API_ID", "0") or "0")
    api_hash = args.api_hash or os.environ.get("TG_API_HASH", "")
    if not api_id or not api_hash:
        print(
            "Missing API credentials.\n"
            "  Set TG_API_ID / TG_API_HASH, or pass --api-id / --api-hash\n"
            "  Create an app at https://my.telegram.org/apps",
            file=sys.stderr,
        )
        return 2

    out_dir = Path(args.out).resolve()
    out_dir.mkdir(parents=True, exist_ok=True)
    since = datetime.now(timezone.utc) - timedelta(hours=args.hours)

    # connection_retries helps on flaky networks; flood sleep is handled explicitly.
    client = TelegramClient(
        args.session,
        api_id,
        api_hash,
        connection_retries=5,
        retry_delay=1,
        auto_reconnect=True,
        request_retries=3,
    )
    if args.phone:
        await client.start(phone=args.phone)
    else:
        await client.start()

    me = await client.get_me()
    print(f"logged in as {me.first_name} (@{me.username or me.id})")
    print(
        f"scan .txt since {since.isoformat()} -> {out_dir}\n"
        f"concurrency: channels={args.workers} downloads={args.downloads} "
        f"delay={args.delay}s max={args.max_bytes // (1024*1024)}MB"
    )

    stats = Stats()
    chan_sem = asyncio.Semaphore(args.workers)
    dl_sem = asyncio.Semaphore(args.downloads)

    targets: list[tuple] = []
    async for dialog in client.iter_dialogs():
        entity = dialog.entity
        if isinstance(entity, User):
            continue
        if isinstance(entity, Chat) and not args.include_groups:
            continue
        if isinstance(entity, Channel):
            if not (entity.broadcast or entity.megagroup or args.include_groups):
                continue
        elif not args.include_groups:
            continue

        title = dialog.name or str(dialog.id)
        chat_dir = out_dir / safe_filename(title).removesuffix(".txt")
        targets.append((entity, title, chat_dir))

    print(f"queued {len(targets)} chats")
    t0 = time.perf_counter()

    await asyncio.gather(
        *[
            scrape_channel(
                client, entity, title, chat_dir, since, args, chan_sem, dl_sem, stats
            )
            for entity, title, chat_dir in targets
        ]
    )

    await client.disconnect()
    elapsed = time.perf_counter() - t0
    print(
        f"\ndone in {elapsed:.1f}s | channels={stats.channels} "
        f"found={stats.found} saved={stats.saved} skipped={stats.skipped} "
        f"too_large={stats.skipped_large} failed={stats.failed}"
    )
    return 0


def main() -> int:
    ap = argparse.ArgumentParser(
        description="Fast concurrent download of .txt files from Telegram channels"
    )
    ap.add_argument("--api-id", type=int, default=0, help="Telegram api_id")
    ap.add_argument("--api-hash", default="", help="Telegram api_hash")
    ap.add_argument("--phone", default="", help="Phone with country code, e.g. +1...")
    ap.add_argument("--session", default="tg_scrape", help="Session file basename")
    ap.add_argument("-o", "--out", default="tg_txt_downloads", help="Output directory")
    ap.add_argument("--hours", type=float, default=24.0, help="Look-back hours (default 24)")
    ap.add_argument(
        "--workers",
        type=int,
        default=4,
        help="Parallel channel scans (default 4; raise carefully, max ~8)",
    )
    ap.add_argument(
        "--downloads",
        type=int,
        default=6,
        help="Parallel file downloads (default 6; raise carefully, max ~10)",
    )
    ap.add_argument(
        "--delay",
        type=float,
        default=0.05,
        help="Small pause after each download (default 0.05s)",
    )
    ap.add_argument(
        "--include-groups",
        action="store_true",
        help="Also scan basic groups",
    )
    ap.add_argument("--force", action="store_true", help="Re-download existing files")
    ap.add_argument(
        "--max-mb",
        type=float,
        default=1024.0,
        help="Skip files larger than this many MB (default 1024 = 1 GB)",
    )
    args = ap.parse_args()

    # Hard caps — going higher trips FloodWait constantly and is slower overall.
    args.workers = max(1, min(args.workers, 8))
    args.downloads = max(1, min(args.downloads, 10))
    args.delay = max(0.0, args.delay)
    args.max_bytes = int(max(1.0, args.max_mb) * 1024 * 1024)

    return asyncio.run(scrape(args))


if __name__ == "__main__":
    raise SystemExit(main())
