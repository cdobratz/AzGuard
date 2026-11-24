#!/usr/bin/env python3
"""Update version, urls, and hashes in the azguard Scoop manifest.

Leaves description, homepage, license, bin, checkver, and autoupdate as they
are. Hash values keep a sha256: prefix when the current hash uses one.
Exits 0 after printing "unchanged" or "updated".
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path


def version_of(tag: str) -> str:
    if not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+(?:[.+-][0-9A-Za-z.+-]+)?", tag):
        raise SystemExit(f"tag must look like v1.2.0, got {tag!r}")
    return tag[1:]


def checksums(path: Path) -> dict[str, str]:
    found: dict[str, str] = {}
    for raw in path.read_text().splitlines():
        line = raw.strip()
        if not line:
            continue
        parts = line.split()
        if len(parts) < 2:
            raise SystemExit(f"bad checksum line: {raw!r}")
        digest, name = parts[0], parts[-1]
        if not re.fullmatch(r"[0-9a-f]{64}", digest):
            raise SystemExit(f"bad sha256 for {name}: {digest!r}")
        found[name] = digest
    return found


def require(sums: dict[str, str], name: str) -> str:
    if name not in sums:
        raise SystemExit(f"checksums.txt has no {name}")
    return sums[name]


def hash_value(current: object, digest: str) -> str:
    if isinstance(current, str) and current.startswith("sha256:"):
        return "sha256:" + digest
    return digest


def update(manifest: dict, tag: str, sums: dict[str, str]) -> dict:
    version = version_of(tag)
    updated = json.loads(json.dumps(manifest))
    updated["version"] = version
    architecture = updated.setdefault("architecture", {})
    targets = {
        "64bit": f"azguard_{version}_windows_amd64.zip",
        "arm64": f"azguard_{version}_windows_arm64.zip",
    }
    for arch, filename in targets.items():
        entry = architecture.get(arch)
        if not isinstance(entry, dict):
            entry = {}
            architecture[arch] = entry
        entry["url"] = (
            "https://github.com/cdobratz/AzGuard/releases/download/"
            f"{tag}/{filename}"
        )
        entry["hash"] = hash_value(entry.get("hash"), require(sums, filename))
    return updated


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--tag", required=True)
    parser.add_argument("--checksums", required=True, type=Path)
    parser.add_argument("--manifest", required=True, type=Path)
    args = parser.parse_args()

    current = json.loads(args.manifest.read_text())
    revised = update(current, args.tag, checksums(args.checksums))
    if revised == current:
        print("unchanged")
        return
    text = json.dumps(revised, indent=2) + "\n"
    args.manifest.write_text(text)
    print("updated")


if __name__ == "__main__":
    main()
