"""Per-run project sandboxes and short-lived runtime directories."""

from __future__ import annotations

import shutil
import tempfile
import time
from pathlib import Path


def materialize(fixture_dir: Path, dest: Path) -> None:
    """Copy a fixture into a fresh project directory."""
    if dest.exists():
        shutil.rmtree(dest)
    shutil.copytree(fixture_dir, dest)


def short_runtime_dir() -> Path:
    """Create a short XDG_RUNTIME_DIR for the MCP daemon socket.

    Unix socket paths are length-capped, so use /tmp rather than the longer
    per-user temp directory when available.
    """
    base = "/tmp" if Path("/tmp").is_dir() else None
    return Path(tempfile.mkdtemp(prefix="slopfood-ev-rt-", dir=base))


def remove_tree(path: Path, attempts: int = 8, delay_s: float = 0.5) -> None:
    """Remove a directory, retrying while the engine may still be writing."""
    for attempt in range(attempts):
        try:
            shutil.rmtree(path)
            return
        except FileNotFoundError:
            return
        except OSError:
            if attempt == attempts - 1:
                raise
            time.sleep(delay_s)
