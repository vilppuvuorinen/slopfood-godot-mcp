"""Minimal client and lifecycle for `opencode serve`.

Only the endpoints this suite needs are wrapped: health, sessions, prompting,
MCP attachment, message history and instance disposal. The OpenAPI spec lives
at ``<base_url>/doc`` if more is ever required.
"""

from __future__ import annotations

import json
import os
import socket
import subprocess
import time
from pathlib import Path
from typing import Any

import httpx


class OpencodeError(RuntimeError):
    pass


def _free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


class OpencodeServer:
    """A headless opencode server with an isolated configuration directory."""

    def __init__(
        self,
        run_root: Path,
        binary: str = "opencode",
        port: int | None = None,
        startup_timeout_s: float = 30.0,
    ) -> None:
        self.run_root = run_root
        self.binary = binary
        self.port = port or _free_port()
        self.startup_timeout_s = startup_timeout_s
        self._proc: subprocess.Popen[bytes] | None = None
        self._log_path = run_root / "opencode-serve.log"
        self.config_dir = run_root / "oc-config"
        self.client: OpencodeClient | None = None

    @property
    def base_url(self) -> str:
        return f"http://127.0.0.1:{self.port}"

    def __enter__(self) -> OpencodeServer:
        self.start()
        return self

    def __exit__(self, *exc: object) -> None:
        self.stop()

    def start(self) -> None:
        # opencode reads global config from $XDG_CONFIG_HOME/opencode/opencode.json.
        # Point that at a temp dir so the host's global MCP servers cannot leak in,
        # but carry over the host's provider/model/auth-adjacent config so the
        # configured providers still resolve.
        config_path = self.config_dir / "opencode" / "opencode.json"
        config_path.parent.mkdir(parents=True, exist_ok=True)
        config_path.write_text(json.dumps(self._base_config(), indent=2) + "\n")
        env = os.environ.copy()
        env["XDG_CONFIG_HOME"] = str(self.config_dir)
        self._log_path.parent.mkdir(parents=True, exist_ok=True)
        log = self._log_path.open("wb")
        self._proc = subprocess.Popen(
            [
                self.binary,
                "serve",
                "--port",
                str(self.port),
                "--hostname",
                "127.0.0.1",
                "--pure",
                "--print-logs",
            ],
            stdout=log,
            stderr=subprocess.STDOUT,
            env=env,
        )
        self.client = OpencodeClient(self.base_url)
        self._wait_healthy()

    def _base_config(self) -> dict[str, Any]:
        """Host config minus MCP servers, with subagents disabled.

        The eval session must be deterministic, so the executor may not spawn
        subagents. Host MCP servers are dropped unconditionally; everything else
        (providers, models, agents) is preserved so the run needs no extra setup.
        """
        source = os.environ.get("EVALS_OPENCODE_CONFIG")
        candidates = (
            [Path(source).expanduser()]
            if source
            else [
                Path(os.environ.get("XDG_CONFIG_HOME") or Path.home() / ".config")
                / "opencode"
                / "opencode.json"
            ]
        )
        config: dict[str, Any] = {}
        for candidate in candidates:
            if not candidate.is_file():
                continue
            try:
                loaded = json.loads(candidate.read_text())
            except (OSError, json.JSONDecodeError):
                continue
            if isinstance(loaded, dict):
                config = loaded
                break
        config = dict(config)
        config.pop("mcp", None)
        config.setdefault("$schema", "https://opencode.ai/config.json")
        config["subagent_depth"] = 0
        return config

    def _wait_healthy(self) -> None:
        deadline = time.monotonic() + self.startup_timeout_s
        last: Exception | None = None
        while time.monotonic() < deadline:
            if self._proc is not None and self._proc.poll() is not None:
                raise OpencodeError(
                    f"opencode serve exited early (code {self._proc.returncode}); "
                    f"see {self._log_path}"
                )
            try:
                health = self.client.health()  # type: ignore[union-attr]
                if health.get("healthy"):
                    return
            except Exception as exc:  # noqa: BLE001 - retry any transport error
                last = exc
            time.sleep(0.2)
        raise OpencodeError(f"opencode serve did not become healthy; see {self._log_path}: {last}")

    def stop(self) -> None:
        if self.client is not None:
            self.client.close()
        if self._proc is not None and self._proc.poll() is None:
            self._proc.terminate()
            try:
                self._proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                self._proc.kill()


class OpencodeClient:
    def __init__(self, base_url: str, timeout_s: float = 60.0) -> None:
        self.base_url = base_url.rstrip("/")
        self._client = httpx.Client(base_url=self.base_url, timeout=timeout_s)

    def close(self) -> None:
        self._client.close()

    def __enter__(self) -> OpencodeClient:
        return self

    def __exit__(self, *exc: object) -> None:
        self.close()

    # -- low level ---------------------------------------------------------

    def _request(
        self,
        method: str,
        path: str,
        *,
        directory: str | None = None,
        json_body: dict[str, Any] | None = None,
        timeout_s: float | None = None,
    ) -> Any:
        params = {"directory": directory} if directory else None
        try:
            response = self._client.request(
                method,
                path,
                params=params,
                json=json_body,
                timeout=timeout_s,
            )
        except httpx.HTTPError as exc:
            raise OpencodeError(f"{method} {path}: {exc}") from exc
        if response.status_code >= 400:
            raise OpencodeError(
                f"{method} {path} -> {response.status_code}: {response.text[:500]}"
            )
        if not response.content:
            return None
        return response.json()

    # -- endpoints ---------------------------------------------------------

    def health(self) -> dict[str, Any]:
        return self._request("GET", "/global/health", timeout_s=5.0)

    def tool_ids(self, directory: str | None = None) -> list[str]:
        ids = self._request("GET", "/experimental/tool/ids", directory=directory)
        return [str(name) for name in (ids or [])]

    def create_session(self, directory: str, title: str) -> str:
        session = self._request(
            "POST", "/session", directory=directory, json_body={"title": title}
        )
        session_id = session.get("id")
        if not session_id:
            raise OpencodeError(f"session create returned no id: {session!r}")
        return str(session_id)

    def prompt(
        self,
        session_id: str,
        directory: str,
        *,
        model: dict[str, str],
        agent: str,
        tools: dict[str, bool],
        parts: list[dict[str, Any]],
        timeout_s: float,
        system: str | None = None,
        output_format: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        body: dict[str, Any] = {
            "model": model,
            "agent": agent,
            "tools": tools,
            "parts": parts,
        }
        if system:
            body["system"] = system
        if output_format:
            body["format"] = output_format
        return self._request(
            "POST",
            f"/session/{session_id}/message",
            directory=directory,
            json_body=body,
            timeout_s=timeout_s,
        )

    def messages(self, session_id: str, directory: str) -> list[dict[str, Any]]:
        data = self._request("GET", f"/session/{session_id}/message", directory=directory)
        if isinstance(data, dict):  # tolerate a wrapped shape across versions
            data = data.get("data", [])
        return list(data or [])

    def diff(self, session_id: str, directory: str) -> list[dict[str, Any]]:
        data = self._request("GET", f"/session/{session_id}/diff", directory=directory)
        return list(data or [])

    def add_mcp(self, directory: str, name: str, config: dict[str, Any]) -> dict[str, Any]:
        return self._request(
            "POST", "/mcp", directory=directory, json_body={"name": name, "config": config}
        )

    def mcp_status(self, directory: str) -> dict[str, Any]:
        return self._request("GET", "/mcp", directory=directory) or {}

    def wait_mcp(self, directory: str, name: str, timeout_s: float = 30.0) -> None:
        deadline = time.monotonic() + timeout_s
        last: Any = None
        while time.monotonic() < deadline:
            status = self.mcp_status(directory).get(name, {})
            state = status.get("status")
            last = status
            if state == "connected":
                return
            if state == "failed":
                raise OpencodeError(f"MCP {name!r} failed to connect: {status.get('error')}")
            time.sleep(0.2)
        raise OpencodeError(f"MCP {name!r} did not connect in time: {last!r}")

    def dispose(self, directory: str) -> None:
        self._request("POST", "/instance/dispose", directory=directory, timeout_s=30.0)


def parse_model(spec: str) -> dict[str, str]:
    """Split a ``provider/model`` string, preserving any slashes in the model."""
    provider, _, model = spec.partition("/")
    if not provider or not model:
        raise OpencodeError(f"model must be 'provider/model', got {spec!r}")
    return {"providerID": provider, "modelID": model}
