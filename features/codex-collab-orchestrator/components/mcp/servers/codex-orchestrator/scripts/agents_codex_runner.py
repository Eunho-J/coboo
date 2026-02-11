#!/usr/bin/env python3
"""
Best-effort child/merge runner for tmux panes.

Primary target:
- OpenAI Agents SDK + codex mcp-server integration.

Fallback:
- If Agents SDK is not available in runtime, execute `codex --no-alt-screen`.
"""

from __future__ import annotations

import argparse
import os
import shlex
import subprocess
import sys
from dataclasses import dataclass


@dataclass
class RunnerArgs:
    mode: str
    session_id: int
    thread_id: int
    role: str
    initial_prompt: str


def parse_args() -> RunnerArgs:
    parser = argparse.ArgumentParser(description="Agents SDK codex MCP runner")
    parser.add_argument("--mode", default="child")
    parser.add_argument("--session-id", type=int, required=True)
    parser.add_argument("--thread-id", type=int, required=True)
    parser.add_argument("--role", required=True)
    parser.add_argument("--initial-prompt", default="")
    parsed = parser.parse_args()
    return RunnerArgs(
        mode=parsed.mode,
        session_id=parsed.session_id,
        thread_id=parsed.thread_id,
        role=parsed.role,
        initial_prompt=parsed.initial_prompt,
    )


def print_banner(args: RunnerArgs) -> None:
    print(
        f"[agents-runner] mode={args.mode} session_id={args.session_id} "
        f"thread_id={args.thread_id} role={args.role}",
        flush=True,
    )


def exec_codex_fallback(initial_prompt: str) -> int:
    command = ["codex", "--no-alt-screen"]
    if initial_prompt.strip():
        command.append(initial_prompt.strip())
    print(
        f"[agents-runner] fallback -> {' '.join(shlex.quote(part) for part in command)}",
        flush=True,
    )
    os.execvp(command[0], command)
    return 127


def try_agents_sdk(initial_prompt: str) -> int:
    """
    Placeholder hook:
    - The runtime can provide a richer Agents SDK implementation here.
    - Until then, fallback keeps worker execution functional.
    """
    try:
        __import__("agents")
    except Exception:
        return exec_codex_fallback(initial_prompt)

    print(
        "[agents-runner] agents sdk detected, but no direct runtime adapter is bundled. "
        "falling back to codex cli execution.",
        flush=True,
    )
    return exec_codex_fallback(initial_prompt)


def main() -> int:
    args = parse_args()
    print_banner(args)

    # Keep metadata available for wrapper scripts if needed.
    os.environ["COBOO_RUNNER_MODE"] = args.mode
    os.environ["COBOO_SESSION_ID"] = str(args.session_id)
    os.environ["COBOO_THREAD_ID"] = str(args.thread_id)
    os.environ["COBOO_THREAD_ROLE"] = args.role

    return try_agents_sdk(args.initial_prompt)


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except KeyboardInterrupt:
        # Respect tmux interrupt handling semantics.
        subprocess.run(["printf", "[agents-runner] interrupted\n"], check=False)
        raise
