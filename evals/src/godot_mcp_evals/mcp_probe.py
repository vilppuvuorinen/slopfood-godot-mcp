"""Discover the MCP tools a server binary exposes, without needing Godot.

The eval harness must know the exact MCP tool names so it can build complete
boolean tool maps. The server's MCP stdio transport speaks newline-delimited
JSON-RPC, so a short initialize + tools/list handshake is enough.
"""

from __future__ import annotations

import json
import os
import queue
import subprocess
import tempfile
import threading
from pathlib import Path
from typing import Any


class McpProbeError(RuntimeError):
    pass


def discover_mcp_tools(binary: Path, timeout_s: float = 20.0) -> list[str]:
    with (
        tempfile.TemporaryDirectory(prefix="evals-mcp-") as project,
        tempfile.TemporaryDirectory(prefix="evals-rt-") as runtime,
    ):
        env = {**os.environ, "XDG_RUNTIME_DIR": runtime}
        proc = subprocess.Popen(
            [str(binary), "--project", project, "--idle-timeout", "1s"],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            text=True,
            env=env,
        )
        try:
            return _handshake(proc, timeout_s)
        finally:
            proc.terminate()
            try:
                proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                proc.kill()


def _handshake(proc: subprocess.Popen[str], timeout_s: float) -> list[str]:
    assert proc.stdin is not None and proc.stdout is not None
    responses: queue.Queue[dict[str, Any] | None] = queue.Queue()
    reader = threading.Thread(target=_read_lines, args=(proc.stdout, responses), daemon=True)
    reader.start()

    def send(payload: dict[str, Any]) -> None:
        proc.stdin.write(json.dumps(payload) + "\n")
        proc.stdin.flush()

    def wait_for(request_id: int) -> dict[str, Any]:
        while True:
            try:
                message = responses.get(timeout=timeout_s)
            except queue.Empty as exc:
                raise McpProbeError("timed out waiting for the MCP server") from exc
            if message is None:
                raise McpProbeError("MCP server closed the connection")
            if message.get("id") == request_id:
                return message

    send(
        {
            "jsonrpc": "2.0",
            "id": 1,
            "method": "initialize",
            "params": {
                "protocolVersion": "2024-11-05",
                "capabilities": {},
                "clientInfo": {"name": "godot-mcp-evals", "version": "0.0.1"},
            },
        }
    )
    wait_for(1)
    send({"jsonrpc": "2.0", "method": "notifications/initialized"})
    send({"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}})
    listing = wait_for(2)
    tools = listing.get("result", {}).get("tools", [])
    names = sorted(str(tool["name"]) for tool in tools)
    if not names:
        raise McpProbeError("MCP server exposed no tools")
    return names


def _read_lines(stream: Any, out: queue.Queue[dict[str, Any] | None]) -> None:
    for line in stream:
        line = line.strip()
        if not line:
            continue
        try:
            out.put(json.loads(line))
        except json.JSONDecodeError:
            continue
    out.put(None)
