"""Loading and validation of suite, shape and task definitions."""

from __future__ import annotations

import dataclasses
from pathlib import Path
from typing import Any

import yaml


@dataclasses.dataclass(frozen=True)
class Check:
    """One deterministic assertion evaluated against the resulting project.

    ``weight`` and ``required`` shape how composite tasks are scored: a run
    earns the weight of every passing check, and a ``required`` check must pass
    regardless of the final weighted score. Both default to the binary
    behavior (weight 1, not required).
    """

    kind: str
    params: dict[str, Any]
    weight: float = 1.0
    required: bool = False

    @classmethod
    def from_mapping(cls, raw: dict[str, Any]) -> Check:
        if "kind" not in raw:
            raise ValueError(f"check is missing 'kind': {raw!r}")
        weight = float(raw.get("weight", 1.0))
        if weight < 0:
            raise ValueError(f"check weight must be non-negative: {raw!r}")
        required = bool(raw.get("required", False))
        params = {
            k: v for k, v in raw.items() if k not in ("kind", "weight", "required")
        }
        return cls(kind=str(raw["kind"]), params=params, weight=weight, required=required)


@dataclasses.dataclass(frozen=True)
class Expectation:
    """Per-shape trajectory expectations for a task.

    Tool names are the fully-qualified names opencode exposes: MCP tools are
    prefixed with the server name (``godot_add_node``), built-ins are bare
    (``edit``). An empty expectation asserts nothing.
    """

    used_any: tuple[str, ...] = ()
    used_all: tuple[str, ...] = ()
    used_none: tuple[str, ...] = ()

    @classmethod
    def from_mapping(cls, raw: dict[str, Any]) -> Expectation:
        def names(key: str) -> tuple[str, ...]:
            value = raw.get(key, [])
            if not isinstance(value, (list, tuple)):
                raise ValueError(f"expectation {key!r} must be a list, got {value!r}")
            return tuple(str(name) for name in value)

        return cls(
            used_any=names("used_any"),
            used_all=names("used_all"),
            used_none=names("used_none"),
        )


@dataclasses.dataclass(frozen=True)
class Task:
    id: str
    description: str
    fixture: str
    prompt: str
    checks: list[Check]
    rubric: str
    timeout_s: int | None
    expect: dict[str, Expectation]
    # Weighted fraction of check weight a run must earn to pass. ``None`` means
    # every check must pass (the binary default).
    min_score: float | None = None
    # True for tasks whose fixture is deliberately not yet a Godot project (e.g.
    # create_project); every other fixture must ship a project.godot.
    bootstraps_project: bool = False

    @classmethod
    def from_yaml(cls, path: Path) -> Task:
        data = yaml.safe_load(path.read_text()) or {}
        missing = [k for k in ("id", "fixture", "prompt", "checks") if k not in data]
        if missing:
            raise ValueError(f"{path}: missing required keys: {', '.join(missing)}")
        checks = [Check.from_mapping(c) for c in data["checks"]]
        min_score = float(data["min_score"]) if data.get("min_score") is not None else None
        if min_score is not None and not 0.0 <= min_score <= 1.0:
            raise ValueError(f"{path}: min_score must be between 0 and 1, got {min_score}")
        return cls(
            id=str(data["id"]),
            description=str(data.get("description", "")),
            fixture=str(data["fixture"]),
            prompt=str(data["prompt"]),
            checks=checks,
            rubric=str(data.get("rubric", "")),
            timeout_s=int(data["timeout_s"]) if data.get("timeout_s") else None,
            expect=_parse_expect(data.get("expect"), path),
            min_score=min_score,
            bootstraps_project=bool(data.get("bootstraps_project", False)),
        )


def _parse_expect(raw: Any, path: Path) -> dict[str, Expectation]:
    if raw is None:
        return {}
    if not isinstance(raw, dict):
        raise ValueError(f"{path}: 'expect' must be a mapping of shape -> expectation")
    expect: dict[str, Expectation] = {}
    for shape, spec in raw.items():
        if spec is None:
            spec = {}
        if not isinstance(spec, dict):
            raise ValueError(f"{path}: expectation for shape {shape!r} must be a mapping")
        expect[str(shape)] = Expectation.from_mapping(spec)
    return expect


@dataclasses.dataclass(frozen=True)
class Shape:
    """A named set of enabled tools, resolved to a complete boolean map."""

    name: str
    description: str
    tools: dict[str, bool]
    reps: int | None = None

    def enabled(self) -> list[str]:
        return sorted(name for name, on in self.tools.items() if on)


@dataclasses.dataclass(frozen=True)
class Suite:
    executor: str
    judge: str
    reps: int
    timeout_s: int
    judge_enabled: bool
    shapes: list[str]
    tasks: str | list[str]
    jobs: int = 4


def load_tasks(tasks_dir: Path) -> dict[str, Task]:
    tasks = {}
    for path in sorted(tasks_dir.glob("*.yaml")):
        task = Task.from_yaml(path)
        if task.id in tasks:
            raise ValueError(f"duplicate task id {task.id!r}")
        tasks[task.id] = task
    if not tasks:
        raise ValueError(f"no task definitions found in {tasks_dir}")
    return tasks


def load_suite(path: Path) -> Suite:
    data = yaml.safe_load(path.read_text()) or {}
    models = data.get("models", {})
    return Suite(
        executor=str(models.get("executor", "opencode-go/deepseek-v4.1-flash")),
        judge=str(models.get("judge", models.get("executor", "opencode-go/deepseek-v4.1-flash"))),
        reps=int(data.get("reps", 3)),
        timeout_s=int(data.get("timeout_s", 300)),
        judge_enabled=bool(data.get("judge", True)),
        shapes=[str(s) for s in data.get("shapes", [])],
        tasks=data.get("tasks", "all"),
        jobs=max(1, int(data.get("jobs", 4))),
    )


def resolve_shape_tools(
    mcp_spec: Any,
    builtin: list[str],
    all_mcp: list[str],
    all_builtin: list[str],
) -> dict[str, bool]:
    """Turn a shape's ``mcp`` selector and builtin allowlist into a full map.

    The map always contains every known MCP and built-in tool, so senders can
    pass it verbatim to opencode without relying on default enablement.
    """
    known_mcp = set(all_mcp)
    selected = _select_mcp(mcp_spec, all_mcp)
    unknown = selected - known_mcp
    if unknown:
        raise ValueError(f"shape selects unknown MCP tools: {sorted(unknown)}")

    known_builtin = set(all_builtin)
    selected_builtin = set(builtin)
    unknown_builtin = selected_builtin - known_builtin
    if unknown_builtin:
        raise ValueError(f"shape selects unknown built-in tools: {sorted(unknown_builtin)}")

    tools: dict[str, bool] = {}
    for name in all_mcp:
        tools[f"godot_{name}"] = name in selected
    for name in all_builtin:
        tools[name] = name in selected_builtin
    return tools


def _select_mcp(spec: Any, all_mcp: list[str]) -> set[str]:
    if spec in (None, "none"):
        return set()
    if spec == "all":
        return set(all_mcp)
    if isinstance(spec, str) and spec.startswith("all-except:"):
        excluded = {part.strip() for part in spec[len("all-except:") :].split(",") if part.strip()}
        unknown = excluded - set(all_mcp)
        if unknown:
            raise ValueError(f"shape excludes unknown MCP tools: {sorted(unknown)}")
        return set(all_mcp) - excluded
    if isinstance(spec, (list, tuple)):
        return {str(name) for name in spec}
    raise ValueError(f"unsupported mcp selector: {spec!r}")


def load_shapes(
    path: Path,
    all_mcp: list[str],
    all_builtin: list[str],
) -> dict[str, Shape]:
    data = yaml.safe_load(path.read_text()) or {}
    raw_shapes = data.get("shapes", {})
    shapes: dict[str, Shape] = {}
    for name, spec in raw_shapes.items():
        tools = resolve_shape_tools(
            spec.get("mcp", "none"),
            list(spec.get("builtin", [])),
            all_mcp,
            all_builtin,
        )
        shapes[str(name)] = Shape(
            name=str(name),
            description=str(spec.get("description", "")),
            tools=tools,
            reps=int(spec["reps"]) if spec.get("reps") is not None else None,
        )
    if not shapes:
        raise ValueError(f"no shapes defined in {path}")
    return shapes
