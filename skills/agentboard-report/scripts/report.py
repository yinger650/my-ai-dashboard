#!/usr/bin/env python3
"""Send AgentBoard ingest events to https://board.yinger650.com (or AGENTBOARD_URL).

If AGENTBOARD_TOKEN is unset, exits 0 without sending (agents must not fail the task).
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import tempfile
import urllib.error
import urllib.request
import uuid
from datetime import datetime, timezone
from pathlib import Path

DEFAULT_URL = "https://board.yinger650.com"
DEFAULT_TTL = 180


def utc_now() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%f")[:-3] + "Z"


def env(name: str, default: str = "") -> str:
    return os.environ.get(name, default).strip()


def infer_provider() -> str:
    if env("AGENTBOARD_PROVIDER"):
        return env("AGENTBOARD_PROVIDER")
    if env("OPENCLAW_HOME") or env("OPENCLAW_STATE_DIR") or env("OPENCLAW_PROFILE"):
        return "openclaw"
    if env("CODEX_HOME") or env("CODEX_THREAD_ID"):
        return "codex"
    if env("CURSOR_AGENT") or env("CURSOR_TRACE_ID") or env("CURSOR_CLOUD_AGENT"):
        return "cursor"
    return "agent"


def service_meta(provider: str) -> tuple[str, str]:
    key = env("AGENTBOARD_SERVICE_KEY")
    name = env("AGENTBOARD_SERVICE_NAME")
    defaults = {
        "cursor": ("cursor", "Cursor Agent"),
        "codex": ("codex", "Codex"),
        "openclaw": ("openclaw", "OpenClaw"),
        "agent": ("agent", "Agent"),
    }
    dkey, dname = defaults.get(provider, defaults["agent"])
    return key or dkey, name or dname


def run_key_path() -> Path:
    base = env("AGENTBOARD_STATE_DIR") or os.environ.get("XDG_RUNTIME_DIR") or tempfile.gettempdir()
    ident = env("AGENTBOARD_RUN_FILE") or f"agentboard-run-{os.getppid()}-{os.getpid()}"
    return Path(base) / ident


def load_run_key() -> str:
    if env("AGENTBOARD_RUN_KEY"):
        return env("AGENTBOARD_RUN_KEY")
    p = run_key_path()
    if p.exists():
        return p.read_text(encoding="utf-8").strip()
    return ""


def save_run_key(key: str) -> None:
    p = run_key_path()
    try:
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(key, encoding="utf-8")
    except OSError:
        pass


def new_run_key() -> str:
    key = str(uuid.uuid4())
    save_run_key(key)
    return key


def envelope(event_type: str, service_key: str, payload: dict, run_key: str = "") -> dict:
    envl = {
        "schema_version": 1,
        "event_id": str(uuid.uuid4()),
        "event_type": event_type,
        "occurred_at": utc_now(),
        "service_key": service_key,
        "payload": payload,
    }
    if run_key:
        envl["run_key"] = run_key
    return envl


def post(url: str, token: str, events: list[dict], timeout: float, dry_run: bool) -> int:
    body = json.dumps({"events": events}, ensure_ascii=False).encode("utf-8")
    if dry_run:
        sys.stdout.write(body.decode("utf-8") + "\n")
        return 0
    req = urllib.request.Request(
        url.rstrip("/") + "/ingest/v1/events",
        data=body,
        method="POST",
        headers={
            "Authorization": "Bearer " + token,
            "Content-Type": "application/json",
            "User-Agent": "agentboard-report/1.0",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read()
            if os.environ.get("AGENTBOARD_VERBOSE"):
                sys.stderr.write(raw.decode("utf-8", "replace") + "\n")
            return 0
    except urllib.error.HTTPError as e:
        sys.stderr.write(f"agentboard-report: HTTP {e.code} {e.read()[:300]!r}\n")
        return 0 if env("AGENTBOARD_SOFT_FAIL", "1") != "0" else 1
    except Exception as e:  # noqa: BLE001 — never block the agent task
        sys.stderr.write(f"agentboard-report: {e}\n")
        return 0 if env("AGENTBOARD_SOFT_FAIL", "1") != "0" else 1


def heartbeat_events(service_key: str, name: str, provider: str, summary: str, ttl: int) -> list[dict]:
    return [
        envelope(
            "service.state",
            service_key,
            {
                "name": name,
                "type": "agent",
                "state": "running",
                "summary": summary or "alive",
                "severity": "normal",
                "ttl_seconds": ttl,
                "metadata": {"provider": provider},
            },
        ),
        envelope(
            "status.upsert",
            service_key,
            {
                "items": [
                    {
                        "key": "alive",
                        "label": "存活",
                        "value": True,
                        "value_type": "boolean",
                        "severity": "normal",
                        "display_format": "text",
                        "sort_order": 10,
                    },
                    {
                        "key": "provider",
                        "label": "Provider",
                        "value": provider,
                        "value_type": "string",
                        "severity": "info",
                        "display_format": "text",
                        "sort_order": 20,
                    },
                    {
                        "key": "last_heartbeat",
                        "label": "心跳",
                        "value": utc_now(),
                        "value_type": "string",
                        "severity": "normal",
                        "display_format": "text",
                        "sort_order": 30,
                    },
                ]
            },
        ),
    ]


def run_transition(service_key: str, name: str, provider: str, run_key: str, status: str, summary: str, extra: dict | None = None) -> dict:
    payload = {
        "service_name": name,
        "service_type": "agent",
        "status": status,
        "summary": summary,
        "provider": provider,
        "metadata": extra or {},
    }
    if status in ("queued", "running") and not extra:
        payload["started_at"] = utc_now()
    if status in ("succeeded", "failed", "cancelled", "timed_out"):
        payload["finished_at"] = utc_now()
    return envelope("run.transition", service_key, payload, run_key=run_key)


def build_events(cmd: str, args: argparse.Namespace, service_key: str, name: str, provider: str) -> list[dict]:
    ttl = args.ttl
    summary = args.message or ""
    if cmd == "heartbeat":
        return heartbeat_events(service_key, name, provider, summary or "heartbeat", ttl)
    if cmd == "start":
        rk = args.run_key or load_run_key() or new_run_key()
        text = summary or "task started"
        return heartbeat_events(service_key, name, provider, text, ttl) + [
            run_transition(service_key, name, provider, rk, "running", text),
        ]
    if cmd == "progress":
        rk = args.run_key or load_run_key() or new_run_key()
        return [
            envelope(
                "log.append",
                service_key,
                {"markdown": summary or "progress", "severity": "info", "source": provider},
                run_key=rk,
            )
        ]
    if cmd == "log":
        rk = args.run_key or load_run_key()
        return [
            envelope(
                "log.append",
                service_key,
                {"markdown": summary or "(empty)", "severity": args.severity, "source": provider},
                run_key=rk,
            )
        ]
    if cmd == "error":
        rk = args.run_key or load_run_key()
        text = summary or "error"
        return heartbeat_events(service_key, name, provider, text, ttl)[:1] + [
            envelope(
                "log.append",
                service_key,
                {"markdown": text, "severity": "error", "source": provider},
                run_key=rk,
            ),
            envelope(
                "collector.notice",
                service_key,
                {"severity": "error", "code": args.code or "agent_error", "markdown": text, "metadata": {"provider": provider}},
            ),
        ]
    if cmd == "succeed":
        rk = args.run_key or load_run_key() or new_run_key()
        text = summary or "succeeded"
        return heartbeat_events(service_key, name, provider, text, ttl) + [
            run_transition(service_key, name, provider, rk, "succeeded", text),
        ]
    if cmd == "fail":
        rk = args.run_key or load_run_key() or new_run_key()
        text = summary or "failed"
        return [
            envelope(
                "service.state",
                service_key,
                {
                    "name": name,
                    "type": "agent",
                    "state": "failed",
                    "summary": text,
                    "severity": "error",
                    "ttl_seconds": ttl,
                    "metadata": {"provider": provider},
                },
            ),
            envelope(
                "log.append",
                service_key,
                {"markdown": text, "severity": "error", "source": provider},
                run_key=rk,
            ),
            run_transition(service_key, name, provider, rk, "failed", text),
        ]
    if cmd == "notice":
        return [
            envelope(
                "collector.notice",
                service_key,
                {
                    "severity": args.severity,
                    "code": args.code or "notice",
                    "markdown": summary or "notice",
                    "metadata": {"provider": provider},
                },
            )
        ]
    if cmd == "dead":
        return [
            envelope(
                "service.state",
                service_key,
                {
                    "name": name,
                    "type": "agent",
                    "state": "stopped",
                    "summary": summary or "stopped",
                    "severity": "unknown",
                    "ttl_seconds": ttl,
                    "metadata": {"provider": provider},
                },
            )
        ]
    raise SystemExit(f"unknown command {cmd}")


def main() -> int:
    p = argparse.ArgumentParser(description="Report agent status to AgentBoard")
    p.add_argument(
        "command",
        choices=["ping", "heartbeat", "start", "progress", "log", "error", "succeed", "fail", "notice", "dead"],
    )
    p.add_argument("message", nargs="?", default="")
    p.add_argument("--severity", default="info")
    p.add_argument("--code", default="")
    p.add_argument("--run-key", default=env("AGENTBOARD_RUN_KEY"))
    p.add_argument("--ttl", type=int, default=int(env("AGENTBOARD_TTL_SECONDS") or DEFAULT_TTL))
    p.add_argument("--dry-run", action="store_true")
    p.add_argument("--timeout", type=float, default=8.0)
    args = p.parse_args()

    token = env("AGENTBOARD_TOKEN")
    url = env("AGENTBOARD_URL") or DEFAULT_URL
    if args.command == "ping":
        if args.dry_run:
            print(json.dumps({"url": url, "has_token": bool(token)}))
            return 0
        if not token:
            print("agentboard-report: AGENTBOARD_TOKEN unset; skip", file=sys.stderr)
            return 0
        req = urllib.request.Request(
            url.rstrip("/") + "/ingest/v1/ping",
            headers={"Authorization": "Bearer " + token},
        )
        try:
            with urllib.request.urlopen(req, timeout=args.timeout) as resp:
                print(resp.read().decode("utf-8", "replace"))
                return 0
        except Exception as e:  # noqa: BLE001
            print(f"agentboard-report: ping failed: {e}", file=sys.stderr)
            return 0

    if not token and not args.dry_run:
        return 0

    provider = infer_provider()
    service_key, name = service_meta(provider)
    events = build_events(args.command, args, service_key, name, provider)
    return post(url, token, events, args.timeout, args.dry_run)


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except KeyboardInterrupt:
        raise SystemExit(0)
