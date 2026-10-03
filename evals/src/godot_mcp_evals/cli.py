"""Command-line entry point for the evals suite."""

from __future__ import annotations

import argparse
import datetime as dt
import sys
import tempfile
from pathlib import Path

from rich.console import Console
from rich.table import Table

from . import paths, runner, sandbox
from .config import load_shapes, load_suite, load_tasks
from .mcp_probe import discover_mcp_tools
from .opencode import OpencodeError


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="evals",
        description="Compare slopfood-godot-mcp toolset shapes through opencode serve.",
    )
    parser.add_argument(
        "command",
        nargs="?",
        choices=["run", "list"],
        default="run",
        help="run the suite (default) or list tasks and shapes",
    )
    parser.add_argument("--suite", type=Path, help="suite config (default: config/suite.yaml)")
    parser.add_argument(
        "--toolsets", type=Path, help="toolset shapes (default: config/toolsets.yaml)"
    )
    parser.add_argument("--tasks", help="comma-separated task ids (default: suite config)")
    parser.add_argument("--shapes", help="comma-separated shape names (default: suite config)")
    parser.add_argument("--reps", type=int, help="repetitions per task x shape")
    parser.add_argument(
        "--jobs", type=int, help="concurrent sessions (default: suite config, usually 4)"
    )
    parser.add_argument("--model", help="executor model as provider/model")
    parser.add_argument("--judge", help="judge model as provider/model")
    parser.add_argument("--no-judge", action="store_true", help="skip the LLM judge")
    parser.add_argument("--out", type=Path, help="output directory (default: results/<UTC>)")
    parser.add_argument("--work-root", type=Path, help="directory for project sandboxes")
    parser.add_argument("--dry-run", action="store_true", help="print the plan without model calls")
    parser.add_argument("--keep-work", action="store_true", help="keep sandboxes after the run")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    console = Console()
    try:
        evals_dir = paths.repo_root() / "evals"
    except paths.DiscoveryError as exc:
        console.print(f"[red]error:[/] {exc}")
        return 2

    suite_path = args.suite or evals_dir / "config" / "suite.yaml"
    toolsets_path = args.toolsets or evals_dir / "config" / "toolsets.yaml"
    tasks_dir = evals_dir / "tasks"
    fixtures_dir = evals_dir / "fixtures"
    tasks = load_tasks(tasks_dir)
    suite = load_suite(suite_path)

    if args.command == "list":
        return _list(console, tasks, toolsets_path, args)

    try:
        mcp_bin = paths.mcp_binary()
        godot = paths.godot_binary()
        opencode_bin = paths.opencode_binary()
    except paths.DiscoveryError as exc:
        console.print(f"[red]error:[/] {exc}")
        return 2

    selected_tasks = _select(args.tasks, suite.tasks, sorted(tasks))
    ephemeral_work_root = args.work_root is None
    work_root = args.work_root or _default_work_root()

    stamp = dt.datetime.now(dt.UTC).strftime("%Y%m%dT%H%M%SZ")
    out_dir = args.out or evals_dir / "results" / stamp

    options = runner.RunOptions(
        tasks=tasks,
        suite=suite,
        selected_tasks=selected_tasks,
        selected_shapes=_select(args.shapes, suite.shapes, []),
        out_dir=out_dir,
        work_root=work_root,
        fixtures_dir=fixtures_dir,
        toolsets_path=toolsets_path,
        mcp_bin=mcp_bin,
        godot=godot,
        opencode_bin=opencode_bin,
        godot_data_home=evals_dir.parent / ".godot",
        judge_enabled=suite.judge_enabled and not args.no_judge,
        judge_dir=work_root / "judge",
        keep_work=args.keep_work,
        dry_run=args.dry_run,
        executor_spec=args.model,
        judge_spec=args.judge,
        reps=args.reps,
        jobs=args.jobs,
    )
    try:
        runner.run_suite(options, console)
    except (OpencodeError, ValueError) as exc:
        console.print(f"[red]error:[/] {exc}")
        return 1
    if ephemeral_work_root and not args.keep_work:
        # On success only: keep the work root on failure so the server log and
        # judge sessions survive for debugging.
        try:
            sandbox.remove_tree(work_root)
        except OSError as exc:
            console.print(f"[yellow]warning:[/] could not remove {work_root}: {exc}")
    return 0


def _list(console: Console, tasks: dict, toolsets_path: Path, args: argparse.Namespace) -> int:
    table = Table(title="Tasks", header_style="bold")
    table.add_column("id")
    table.add_column("fixture")
    table.add_column("checks", justify="right")
    table.add_column("min_score", justify="right")
    table.add_column("description")
    for task in tasks.values():
        min_score = "-" if task.min_score is None else f"{task.min_score:.2f}"
        table.add_row(
            task.id, task.fixture, str(len(task.checks)), min_score, task.description
        )
    console.print(table)

    try:
        mcp_bin = paths.mcp_binary()
        all_mcp = discover_mcp_tools(mcp_bin)
    except (paths.DiscoveryError, Exception) as exc:  # noqa: BLE001 - listing is best effort
        console.print(f"[yellow]warning:[/] could not probe MCP tools: {exc}")
        all_mcp = []
    try:
        shapes = load_shapes(toolsets_path, all_mcp, runner.DEFAULT_BUILTIN_TOOLS)
    except ValueError as exc:
        console.print(f"[yellow]warning:[/] could not resolve toolset shapes: {exc}")
        return 0
    shape_table = Table(title="Toolset shapes", header_style="bold")
    shape_table.add_column("name")
    shape_table.add_column("tools", justify="right")
    shape_table.add_column("description")
    for shape in shapes.values():
        shape_table.add_row(shape.name, str(len(shape.enabled())), shape.description)
    console.print(shape_table)
    return 0


def _select(explicit: str | None, configured, all_values: list[str]) -> list[str]:
    if explicit:
        return [part.strip() for part in explicit.split(",") if part.strip()]
    if isinstance(configured, str) and configured == "all":
        return all_values
    if isinstance(configured, list):
        return [str(item) for item in configured]
    return all_values


def _default_work_root() -> Path:
    base = "/tmp" if Path("/tmp").is_dir() else None
    return Path(tempfile.mkdtemp(prefix="godot-mcp-evals-", dir=base))


if __name__ == "__main__":
    sys.exit(main())
