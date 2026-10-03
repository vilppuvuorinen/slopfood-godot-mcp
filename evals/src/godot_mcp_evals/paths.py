"""Repository and tool discovery shared by the harness."""

from __future__ import annotations

import os
import shutil
from pathlib import Path

DEFAULT_MCP_BIN = "bin/slopfood-godot-mcp-dev"


class DiscoveryError(RuntimeError):
    pass


def repo_root() -> Path:
    """Locate the checkout root, preferring an explicit override."""
    override = os.environ.get("GODOT_MCP_REPO")
    if override:
        return Path(override).expanduser().resolve()
    here = Path(__file__).resolve()
    for parent in here.parents:
        if (parent / "go.mod").is_file() and (parent / "evals").is_dir():
            return parent
    raise DiscoveryError("could not locate the repository root; set GODOT_MCP_REPO")


def godot_binary(repo: Path | None = None) -> Path:
    """Return the pinned Godot engine, honouring GODOT_PATH."""
    if override := os.environ.get("GODOT_PATH"):
        path = Path(override).expanduser()
        if not path.is_file():
            raise DiscoveryError(f"GODOT_PATH is not a file: {path}")
        return path
    root = repo or repo_root()
    candidates: list[Path] = []
    for minor_dir in sorted((root / ".godot").glob("[0-9]*"), reverse=True):
        candidates.extend(sorted(minor_dir.glob("Godot_v*_linux*")))
    for candidate in candidates:
        if candidate.is_file() and os.access(candidate, os.X_OK):
            return candidate
    raise DiscoveryError(
        f"no Godot binary under {root / '.godot'}; run 'make godot' or set GODOT_PATH"
    )


def mcp_binary(repo: Path | None = None) -> Path:
    """Return the MCP server binary under test, honouring GODOT_MCP_BIN."""
    if override := os.environ.get("GODOT_MCP_BIN") or os.environ.get("MCP_BIN"):
        path = Path(override).expanduser()
        if not path.is_file():
            raise DiscoveryError(f"MCP binary is not a file: {path}")
        return path
    root = repo or repo_root()
    path = root / DEFAULT_MCP_BIN
    if not path.is_file():
        raise DiscoveryError(f"MCP binary not found at {path}; run 'make evals' which builds it")
    return path


def opencode_binary() -> str:
    binary = os.environ.get("OPENCODE_BIN", "opencode")
    resolved = shutil.which(binary)
    if not resolved and not Path(binary).is_file():
        raise DiscoveryError(f"opencode executable not found: {binary}")
    return resolved or binary
