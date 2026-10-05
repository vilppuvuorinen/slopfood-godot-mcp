"""Keep each run inside its own project directory.

A run's project lives in a private parent directory holding nothing else, so
``..`` shows the agent no sibling runs, no judge sessions, no server config and
no server log. This module also lexically flags tool calls that still reach
outside the project; the runner reports those as a trajectory escape so an
isolated run is measured, not assumed.
"""

from __future__ import annotations

import os
import re
import tempfile
from pathlib import Path
from typing import Any

# opencode built-ins whose target path we can inspect. The MCP ``godot_*`` tools
# are confined by the server, and ``task`` subagents are disabled, so neither
# needs checking here.
FILE_PATH_TOOLS = frozenset({"read", "write", "edit", "glob", "grep", "apply_patch"})

# Argument keys that can carry a filesystem location. ``include`` is a grep
# filter; it is path-like enough that a ``../`` in it is worth flagging.
_PATH_ARG_KEYS = (
    "filePath",
    "path",
    "directory",
    "cwd",
    "include",
    "file",
    "fileName",
    "filename",
)

# Device pseudo-files are not a meaningful escape.
_DEV_WHITELIST = frozenset({"/dev/null", "/dev/stdin", "/dev/stdout", "/dev/stderr"})

RES_SCHEME = "res://"
USER_SCHEME = "user://"

# Shell tokens: anything between whitespace and metacharacters. Quoting is not
# modelled, which is fine for a lexical tripwire.
_BASH_TOKEN = re.compile(r"[^\s'\"`|;&()<>]+")


def run_parent() -> Path:
    """Create an empty parent directory for exactly one run's project."""
    base = "/tmp" if Path("/tmp").is_dir() else None
    return Path(tempfile.mkdtemp(prefix="godot-mcp-evals-run-", dir=base))


def instruction(project: Path) -> str:
    """A system instruction that makes the project boundary explicit."""
    return (
        f"Your project directory is {project}. Work only inside it. Do not list, "
        "read, write, or run commands against anything outside it, including its "
        "parent or sibling directories, /tmp, the harness, or other runs. Resolve "
        "every path within the project."
    )


def escapes(project: Path, call_log: list[dict[str, Any]] | None) -> list[str]:
    """Describe every call in ``call_log`` that reaches outside ``project``."""
    root = os.path.normpath(str(project))
    found: list[str] = []
    for call in call_log or []:
        name = str(call.get("name") or "")
        args = call.get("args")
        if not isinstance(args, dict):
            args = {}
        offenders: list[str] = []
        if name in FILE_PATH_TOOLS:
            offenders = _file_tool_escapes(root, args)
        elif name == "bash":
            command = args.get("command")
            if isinstance(command, str):
                offenders = _bash_escapes(root, command)
        if offenders:
            found.append(f"{name}: {', '.join(sorted(set(offenders)))}")
    return found


def _file_tool_escapes(root: str, args: dict[str, Any]) -> list[str]:
    offenders: list[str] = []
    for key in _PATH_ARG_KEYS:
        raw = args.get(key)
        if isinstance(raw, str) and raw.strip() and not _path_inside(root, raw.strip()):
            offenders.append(raw.strip())
    return offenders


def _bash_escapes(root: str, command: str) -> list[str]:
    offenders: list[str] = []
    for token in _BASH_TOKEN.findall(command):
        candidate = token
        if "=" in candidate:
            candidate = candidate.split("=", 1)[1]
        candidate = candidate.rstrip(",;:")
        if candidate and _outside_path(root, candidate):
            offenders.append(candidate)
    return offenders


def _path_inside(root: str, raw: str) -> bool:
    if raw.startswith(RES_SCHEME) or raw.startswith(USER_SCHEME):
        # Virtual Godot paths resolve against the project inside the server.
        return True
    if raw in _DEV_WHITELIST:
        return True
    return _within(root, _resolve(root, raw))


def _outside_path(root: str, token: str) -> bool:
    if token.startswith((RES_SCHEME, USER_SCHEME)):
        return False
    if token in _DEV_WHITELIST:
        return False
    if token == "~" or token.startswith("~/") or token.startswith("$HOME"):
        return True
    if os.path.isabs(token):
        return not _within(root, os.path.normpath(token))
    if ".." in token.split("/"):
        return not _within(root, _resolve(root, token))
    return False


def _resolve(root: str, raw: str) -> str:
    if raw == "~" or raw.startswith("~/"):
        raw = os.path.expanduser(raw)
    elif raw == "$HOME" or raw.startswith("$HOME/"):
        raw = os.path.expanduser("~") + raw[len("$HOME") :]
    if not os.path.isabs(raw):
        raw = os.path.join(root, raw)
    return os.path.normpath(raw)


def _within(root: str, candidate: str) -> bool:
    return candidate == root or candidate.startswith(root + os.sep)
