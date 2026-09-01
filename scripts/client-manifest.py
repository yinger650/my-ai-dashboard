#!/usr/bin/env python3
"""Write manifest.json and SHA256SUMS for linux board-client release assets."""
from __future__ import annotations

import hashlib
import json
import os
import sys
from pathlib import Path

NAMES = (
    "board-client-linux-amd64",
    "board-client-linux-arm64",
)


def main() -> int:
    out_dir = Path(sys.argv[1] if len(sys.argv) > 1 else "dist/client")
    prefix = "board-client-"
    manifest = {
        "schema": 1,
        "name": "board-client",
        "version": os.environ.get("VERSION") or "dev",
        "commit": os.environ.get("COMMIT") or "unknown",
        "build_time": os.environ.get("BUILD_TIME") or "",
        "binaries": {},
    }
    lines: list[str] = []
    for name in NAMES:
        path = out_dir / name
        if not path.is_file():
            print(f"missing {path}", file=sys.stderr)
            return 1
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        platform = name[len(prefix) :]
        manifest["binaries"][platform] = {
            "name": name,
            "sha256": digest,
            "size": path.stat().st_size,
        }
        lines.append(f"{digest}  {name}")
    (out_dir / "SHA256SUMS").write_text("\n".join(lines) + "\n", encoding="utf-8")
    (out_dir / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(manifest, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
