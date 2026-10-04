"""Suite orchestration: sandboxes, opencode sessions, verification and scoring."""

from __future__ import annotations

import concurrent.futures
import dataclasses
import fnmatch
import os
import time
from pathlib import Path
from typing import Any

from rich.console import Console

from . import metrics as metrics_mod
from . import report, verifier
from .config import (
    FILE_EDIT_TOOLS,
    Shape,
    Suite,
    Task,
    load_shapes,
    resolve_expectation,
)
from .judge import judge_run
from .mcp_probe import discover_mcp_tools
from .opencode import OpencodeClient, OpencodeError, OpencodeServer, parse_model
from .sandbox import materialize, remove_tree, short_runtime_dir

DEFAULT_BUILTIN_TOOLS = [
    "bash",
    "edit",
    "write",
    "read",
    "glob",
    "grep",
    "task",
    "webfetch",
    "websearch",
    "todowrite",
    "skill",
    "question",
    "apply_patch",
]


@dataclasses.dataclass
class RunOptions:
    tasks: dict[str, Task]
    suite: Suite
    selected_tasks: list[str]
    selected_shapes: list[str]
    out_dir: Path
    work_root: Path
    fixtures_dir: Path
    toolsets_path: Path
    mcp_bin: Path
    godot: Path
    opencode_bin: str
    godot_data_home: Path
    judge_enabled: bool
    judge_dir: Path
    keep_work: bool
    dry_run: bool
    executor_spec: str | None = None
    judge_spec: str | None = None
    reps: int | None = None
    jobs: int | None = None
    server_max_runs: int | None = None


@dataclasses.dataclass(frozen=True)
class _PlannedRun:
    """One (task, shape, repetition) cell, awaiting execution."""

    index: int
    task: Task
    shape: Shape
    rep: int
    run_id: str


def run_suite(options: RunOptions, console: Console | None = None) -> list[dict[str, Any]]:
    console = console or Console()
    all_mcp = discover_mcp_tools(options.mcp_bin)
    # Probe built-ins on a throwaway server: the dynamic list feeds shape
    # resolution and paying one extra startup keeps the batch servers below
    # free of any work that is not a run.
    with OpencodeServer(
        options.work_root, binary=options.opencode_bin, startup_timeout_s=40
    ) as probe:
        assert probe.client is not None
        all_builtin = _builtin_tools(probe.client, options.work_root, console)
    shapes = load_shapes(options.toolsets_path, all_mcp, all_builtin)

    executor_spec = options.executor_spec or options.suite.executor
    judge_spec = options.judge_spec or options.suite.judge
    executor_model = parse_model(executor_spec)
    judge_model = parse_model(judge_spec)

    _validate_selection(options, shapes)
    _validate_expectations(options, shapes)
    _validate_targets(options, shapes)

    reps_by_shape = {
        name: effective_reps(shapes[name], options.reps, options.suite.reps)
        for name in options.selected_shapes
    }
    plan = _plan_runs(options, shapes, reps_by_shape)

    if options.dry_run:
        _print_plan(options, shapes, executor_spec, judge_spec, reps_by_shape, console)
        return []

    disabled = {name: False for name in next(iter(shapes.values())).tools}
    options.judge_dir.mkdir(parents=True, exist_ok=True)
    (options.out_dir / "verify").mkdir(parents=True, exist_ok=True)

    jobs = max(1, options.jobs or options.suite.jobs)
    # opencode serve keeps every session alive for the life of the process, so
    # peak memory grows with the number of runs. Recycle it every batch.
    server_max_runs = max(1, options.server_max_runs or options.suite.server_max_runs)
    records = _execute_plan(
        plan,
        options=options,
        executor_model=executor_model,
        judge_model=judge_model,
        disabled=disabled,
        console=console,
        jobs=jobs,
        server_max_runs=server_max_runs,
    )

    report.write_reports(
        options.out_dir,
        records,
        {
            "executor": executor_spec,
            "judge": judge_spec,
            "reps": reps_by_shape,
            "shapes": options.selected_shapes,
            "tasks": options.selected_tasks,
            "jobs": jobs,
            "server_max_runs": server_max_runs,
        },
    )
    report.print_summary(records, console)
    console.print(f"\nWrote [bold]{options.out_dir}[/]")
    return records


def effective_reps(shape: Shape, cli_reps: int | None, suite_reps: int) -> int:
    """Resolve a shape's repetition count.

    A shape's own ``reps`` override wins, then an explicit ``--reps``, then the
    suite default.
    """
    if shape.reps is not None:
        return shape.reps
    if cli_reps is not None:
        return cli_reps
    return suite_reps


def _plan_runs(
    options: RunOptions, shapes: dict[str, Shape], reps_by_shape: dict[str, int]
) -> list[_PlannedRun]:
    """Expand the task x shape x repetition matrix into a flat, ordered plan."""
    plan: list[_PlannedRun] = []
    for task_id in options.selected_tasks:
        task = options.tasks[task_id]
        for shape_name in options.selected_shapes:
            shape = shapes[shape_name]
            for rep in range(1, reps_by_shape[shape_name] + 1):
                plan.append(
                    _PlannedRun(
                        index=len(plan) + 1,
                        task=task,
                        shape=shape,
                        rep=rep,
                        run_id=f"{task_id}__{shape_name}__r{rep}",
                    )
                )
    return plan


def _plan_batches(
    plan: list[_PlannedRun], server_max_runs: int
) -> list[list[_PlannedRun]]:
    """Split a plan into batches of at most ``server_max_runs`` cells.

    A batch is the unit of opencode-serve lifetime: the server is started
    before the batch and stopped after it, so peak server memory is bounded by
    the batch size rather than the whole suite.
    """
    size = max(1, server_max_runs)
    return [plan[start : start + size] for start in range(0, len(plan), size)]


def _execute_plan(
    plan: list[_PlannedRun],
    *,
    options: RunOptions,
    executor_model: dict[str, str],
    judge_model: dict[str, str],
    disabled: dict[str, bool],
    console: Console,
    jobs: int,
    server_max_runs: int,
) -> list[dict[str, Any]]:
    """Run every planned cell, up to ``jobs`` concurrently.

    Each cell owns its project, runtime and judge directories, so the only
    shared state within a batch is the opencode server (which scopes instances
    per directory) and the records list, which is written by index from this
    thread alone. The server is recycled between batches.
    """
    records: list[dict[str, Any] | None] = [None] * len(plan)
    total = len(plan)
    batches = _plan_batches(plan, server_max_runs)
    console.rule(
        f"[bold]Running {total} session(s) with {jobs} job(s), "
        f"server recycled every {server_max_runs}[/]"
    )
    done = 0
    for batch in batches:
        with OpencodeServer(
            options.work_root, binary=options.opencode_bin, startup_timeout_s=40
        ) as server:
            assert server.client is not None
            client = server.client

            def work(item: _PlannedRun, client: OpencodeClient = client) -> dict[str, Any]:
                return _run_once(
                    options=options,
                    client=client,
                    task=item.task,
                    shape=item.shape,
                    rep=item.rep,
                    run_id=item.run_id,
                    executor_model=executor_model,
                    judge_model=judge_model,
                    judge_enabled=options.judge_enabled,
                    disabled=disabled,
                    timeout_s=item.task.timeout_s or options.suite.timeout_s,
                    console=console,
                )

            with concurrent.futures.ThreadPoolExecutor(max_workers=jobs) as pool:
                futures = {pool.submit(work, item): item for item in batch}
                for future in concurrent.futures.as_completed(futures):
                    done += 1
                    item = futures[future]
                    record = future.result()
                    records[item.index - 1] = record
                    status = "PASS" if record["passed"] else "FAIL"
                    console.print(f"[bold]{done}/{total}[/] {status} {record['run_id']}")
    return [record for record in records if record is not None]


def _builtin_tools(
    client: OpencodeClient, directory: Path, console: Console | None = None
) -> list[str]:
    try:
        ids = [name for name in client.tool_ids(str(directory)) if name != "invalid"]
    except OpencodeError as exc:
        if console is not None:
            console.print(
                f"[yellow]warning:[/] could not enumerate built-in tools ({exc}); "
                "falling back to the static list"
            )
        return list(DEFAULT_BUILTIN_TOOLS)
    return ids or list(DEFAULT_BUILTIN_TOOLS)


def _validate_selection(options: RunOptions, shapes: dict[str, Shape]) -> None:
    for name in options.selected_shapes:
        if name not in shapes:
            raise ValueError(f"unknown shape {name!r}; known: {sorted(shapes)}")
    for name in options.selected_tasks:
        if name not in options.tasks:
            raise ValueError(f"unknown task {name!r}; known: {sorted(options.tasks)}")


def _validate_expectations(options: RunOptions, shapes: dict[str, Shape]) -> None:
    """Fail loudly when a task's trajectory expectation can never be met.

    The suite mandates no tool calls, so the only expectation is the editability
    gate. ``file_edit_none`` in a shape with no file-editing tool would be
    vacuously true and silently measure nothing, so reject it.
    """
    for name, shape in shapes.items():
        if shape.expect_alias and shape.expect_alias not in shapes:
            raise ValueError(
                f"shape {name!r} aliases unknown shape {shape.expect_alias!r}; "
                f"known: {sorted(shapes)}"
            )
    for task_id in options.selected_tasks:
        task = options.tasks[task_id]
        for shape_name in task.expect:
            if shape_name not in shapes:
                raise ValueError(
                    f"task {task_id!r} expects unknown shape {shape_name!r}; "
                    f"known: {sorted(shapes)}"
                )
        for shape_name in options.selected_shapes:
            shape = shapes[shape_name]
            expectation = resolve_expectation(task, shape)
            if expectation is None:
                continue
            if expectation.file_edit_none and not any(
                shape.tools.get(tool, False) for tool in FILE_EDIT_TOOLS
            ):
                raise ValueError(
                    f"task {task_id!r} uses file_edit_none in shape {shape_name!r}, "
                    f"but none of {list(FILE_EDIT_TOOLS)} are enabled"
                )


def _validate_targets(
    options: RunOptions,
    shapes: dict[str, Shape],
) -> None:
    """Check that the editability gate agrees with each target's ``form``.

    An ``opaque`` target (binary ``.res``/``.scn``) has no text route, so
    ``file_edit_none`` on one is the whole point. A ``text`` target can be
    produced either way, so forbidding a hand edit of one is a config error: the
    artifact checks decide text-serializable files.
    """
    for task_id in options.selected_tasks:
        task = options.tasks[task_id]
        if not task.targets:
            continue
        for shape_name in options.selected_shapes:
            shape = shapes[shape_name]
            expectation = resolve_expectation(task, shape)
            if expectation is None:
                continue
            for target in task.targets:
                aliases = _target_aliases(target.path)
                for pattern in expectation.file_edit_none:
                    if _pattern_hits(pattern, aliases) and target.form == "text":
                        raise ValueError(
                            f"task {task_id!r} shape {shape_name!r} forbids a hand edit "
                            f"(file_edit_none {pattern!r}) of text target {target.path!r}; "
                            "the artifact checks decide text-serializable files"
                        )


def _target_aliases(path: str) -> set[str]:
    aliases = {path}
    if path.startswith("res://"):
        aliases.add(path[len("res://") :])
    else:
        aliases.add(f"res://{path}")
    return aliases


def _pattern_hits(pattern: str, aliases: set[str]) -> bool:
    return any(fnmatch.fnmatch(alias, pattern) for alias in aliases)


def _run_once(
    *,
    options: RunOptions,
    client: OpencodeClient,
    task: Task,
    shape: Shape,
    rep: int,
    run_id: str,
    executor_model: dict[str, str],
    judge_model: dict[str, str],
    judge_enabled: bool,
    disabled: dict[str, bool],
    timeout_s: float,
    console: Console,
) -> dict[str, Any]:
    project = options.work_root / "runs" / run_id
    materialize(options.fixtures_dir / task.fixture, project)
    runtime = short_runtime_dir()
    session_id = ""
    error: str | None = None
    messages: list[dict[str, Any]] = []
    diff: list[dict[str, Any]] = []
    wall_ms = 0

    try:
        client.add_mcp(
            str(project),
            "godot",
            {
                "type": "local",
                "enabled": True,
                "command": [
                    str(options.mcp_bin),
                    "--project",
                    str(project),
                    "--idle-timeout",
                    "2s",
                    "--log-level",
                    "error",
                ],
                "environment": {
                    "PATH": _env("PATH"),
                    "HOME": _env("HOME"),
                    "XDG_RUNTIME_DIR": str(runtime),
                    "XDG_DATA_HOME": str(options.godot_data_home),
                    "GODOT_PATH": str(options.godot),
                },
            },
        )
        client.wait_mcp(str(project), "godot")
        session_id = client.create_session(str(project), run_id)
        started = time.monotonic()
        try:
            client.prompt(
                session_id,
                str(project),
                model=executor_model,
                agent="build",
                tools=shape.tools,
                parts=[{"type": "text", "text": task.prompt}],
                timeout_s=timeout_s,
            )
        except OpencodeError as exc:
            error = str(exc)
        wall_ms = int((time.monotonic() - started) * 1000)

        try:
            messages = client.messages(session_id, str(project))
            diff = client.diff(session_id, str(project))
        except OpencodeError as exc:
            error = error or str(exc)
    except OpencodeError as exc:
        error = str(exc)
    finally:
        try:
            client.dispose(str(project))
        except OpencodeError:
            pass
        time.sleep(2.5)  # let the detached Godot daemon hit its idle timeout

    run_metrics = metrics_mod.extract(messages, wall_ms)
    check_results = verifier.run_checks(
        task,
        project,
        options.godot,
        options.out_dir / "verify" / f"{run_id}.log",
        answer=run_metrics["final_answer"],
        fixture_dir=options.fixtures_dir / task.fixture,
        timeout_s=max(120, timeout_s),
    )
    check_results.extend(
        _trajectory_checks(task, shape, run_metrics["call_log"])
    )
    threshold = task.min_score if task.min_score is not None else 1.0
    score = _weighted_score(check_results)
    required_ok = all(result.passed for result in check_results if result.required)
    passed = bool(check_results) and required_ok and score >= threshold - 1e-9

    judge: dict[str, Any] = {}
    if judge_enabled and run_metrics["turns"] > 0:
        judge = judge_run(
            client,
            model=judge_model,
            judge_dir=options.judge_dir / run_id,
            disabled_tools=disabled,
            task_id=task.id,
            description=task.description,
            prompt=task.prompt,
            rubric=task.rubric,
            check_results=[dataclasses.asdict(r) for r in check_results],
            answer=run_metrics["final_answer"],
            diff=diff,
            call_log=run_metrics["call_log"],
            timeout_s=timeout_s,
        )

    if not passed and error is None:
        error = "one or more checks failed"

    record = {
        "run_id": run_id,
        "task": task.id,
        "shape": shape.name,
        "rep": rep,
        "fixture": task.fixture,
        "passed": passed,
        "score": round(score, 4),
        "min_score": threshold,
        "required_ok": required_ok,
        "error": error,
        "session_id": session_id,
        "project": str(project),
        "checks": [dataclasses.asdict(r) for r in check_results],
        "metrics": run_metrics,
        "judge": judge,
    }

    if not options.keep_work:
        remove_tree(project)
        remove_tree(runtime)

    return record


def _weighted_score(check_results: list[verifier.CheckResult]) -> float:
    """Fraction of total check weight earned by the run (0.0 when no checks)."""
    total = sum(result.weight for result in check_results)
    if total <= 0:
        return 0.0
    earned = sum(result.weight for result in check_results if result.passed)
    return earned / total


def _trajectory_checks(
    task: Task,
    shape: Shape,
    call_log: list[dict[str, Any]] | None = None,
) -> list[verifier.CheckResult]:
    """Grade the one trajectory invariant the suite still enforces.

    No tool call is mandatory: any route that produces a correct artifact is
    accepted, and the deterministic checks are the verdict. The sole gate is
    that a ``file_edit_none`` target with no text form (a binary ``.res``/``.scn``)
    must not be rewritten by a file-editing built-in, which would corrupt it.
    """
    expectation = resolve_expectation(task, shape)
    if expectation is None:
        return []
    checks: list[verifier.CheckResult] = []
    prefix = f"{task.id}:trajectory:{shape.name}"
    edited = _file_edit_targets(call_log or [])
    for pattern in expectation.file_edit_none:
        hits = sorted(path for path in edited if fnmatch.fnmatch(path, pattern))
        checks.append(
            verifier.CheckResult(
                id=f"{prefix}:file_edit_none:{pattern}",
                kind="file_edit",
                passed=not hits,
                detail=(
                    f"{pattern} untouched by {list(FILE_EDIT_TOOLS)}"
                    if not hits
                    else f"{pattern} edited as a file: {hits}"
                ),
                required=True,
            )
        )
    return checks


def _file_edit_targets(call_log: list[dict[str, Any]]) -> set[str]:
    """Paths targeted by a file-editing built-in, matched as a set of aliases.

    A call may name the path absolute (the sandbox project root is passed to the
    tools) or project-relative, with or without the ``res://`` prefix, so each is
    recorded both ways for the glob patterns to match against.
    """
    targets: set[str] = set()
    for call in call_log:
        name = str(call.get("name") or "")
        if name not in FILE_EDIT_TOOLS:
            continue
        path = call.get("path")
        if not isinstance(path, str) or not path.strip():
            continue
        raw = path.strip()
        targets.add(raw)
        if raw.startswith("res://"):
            targets.add(raw[len("res://") :])
        elif not os.path.isabs(raw):
            targets.add(f"res://{raw}")
    return targets


def _print_plan(
    options: RunOptions,
    shapes: dict[str, Shape],
    executor_spec: str,
    judge_spec: str,
    reps_by_shape: dict[str, int],
    console: Console,
) -> None:
    console.print(f"executor: [bold]{executor_spec}[/]  judge: [bold]{judge_spec}[/]")
    for name in options.selected_shapes:
        shape = shapes[name]
        console.print(f"\n[bold]{name}[/]: {shape.description}  reps: {reps_by_shape[name]}")
        console.print(f"  {len(shape.enabled())} tools: {', '.join(shape.enabled())}")
    console.print(f"\ntasks: {', '.join(options.selected_tasks)}")
    console.print(f"would run {len(options.selected_tasks) * sum(reps_by_shape.values())} sessions")


def _env(name: str) -> str:
    import os

    return os.environ.get(name, "")
