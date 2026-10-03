"""Report writing and terminal summaries."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from rich.console import Console
from rich.table import Table

from .metrics import _score as _record_score
from .metrics import aggregate


def write_reports(out_dir: Path, records: list[dict[str, Any]], meta: dict[str, Any]) -> None:
    out_dir.mkdir(parents=True, exist_ok=True)
    with (out_dir / "runs.jsonl").open("w") as handle:
        for record in records:
            handle.write(json.dumps(record) + "\n")
    (out_dir / "results.json").write_text(
        json.dumps({"meta": meta, "runs": records}, indent=2) + "\n"
    )
    (out_dir / "summary.md").write_text(_markdown(records, meta))


def print_summary(records: list[dict[str, Any]], console: Console | None = None) -> None:
    console = console or Console()
    console.print(_shape_table(records))
    console.print(_task_table(records))


def _shape_table(records: list[dict[str, Any]]) -> Table:
    table = Table(title="Pass rate by toolset shape", header_style="bold")
    columns = (
        "shape",
        "runs",
        "pass@1",
        "95% CI",
        "score",
        "judge",
        "mcp/task",
        "builtin/task",
        "turns",
        "tokens",
        "cost $",
        "wall s",
    )
    for column in columns:
        table.add_column(column, justify="right" if column not in ("shape",) else "left")
    for row in aggregate(records, "shape"):
        table.add_row(
            row["shape"],
            str(row["runs"]),
            f"{row['pass_rate']:.0%}",
            f"{row['pass_ci_low']:.2f}-{row['pass_ci_high']:.2f}",
            f"{row['score_mean']:.2f}",
            f"{row['judge_mean']:.2f}" if row["judge_scored"] else "-",
            f"{row['mcp_calls_mean']:.1f}",
            f"{row['builtin_calls_mean']:.1f}",
            f"{row['turns_mean']:.1f}",
            f"{row['total_tokens_mean']:,.0f}",
            f"{row['cost_mean']:.4f}",
            f"{row['wall_s_mean']:.1f}",
        )
    return table


def _task_table(records: list[dict[str, Any]]) -> Table:
    table = Table(title="Pass rate by task", header_style="bold")
    table.add_column("task")
    table.add_column("runs", justify="right")
    table.add_column("pass@1", justify="right")
    table.add_column("score", justify="right")
    table.add_column("shapes passing")
    for row in aggregate(records, "task"):
        group = [r for r in records if r["task"] == row["task"]]
        shape_names = sorted({r["shape"] for r in group})
        passing = sorted({r["shape"] for r in group if r["passed"]})
        table.add_row(
            row["task"],
            str(row["runs"]),
            f"{row['pass_rate']:.0%}",
            f"{row['score_mean']:.2f}",
            f"{len(passing)}/{len(shape_names)}",
        )
    return table


def _markdown(records: list[dict[str, Any]], meta: dict[str, Any]) -> str:
    lines = ["# Toolset-shape eval summary", ""]
    lines.append(f"- Executor: `{meta.get('executor')}`")
    lines.append(f"- Judge: `{meta.get('judge')}`")
    reps = meta.get("reps")
    if isinstance(reps, dict):
        reps = ", ".join(f"{name}={count}" for name, count in sorted(reps.items()))
    lines.append(f"- Repetitions per cell: {reps}")
    lines.append(f"- Runs: {len(records)}")
    lines.append("")

    lines.append("## By shape")
    lines.append("")
    lines.append(
        "| shape | runs | pass@1 | 95% CI | score | judge | mcp calls | builtin calls | "
        "turns | tokens | cost $ | wall s |"
    )
    lines.append(
        "| --- | ---: | ---: | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |"
    )
    for row in aggregate(records, "shape"):
        judge = f"{row['judge_mean']:.2f}" if row["judge_scored"] else "-"
        lines.append(
            f"| {row['shape']} | {row['runs']} | {row['pass_rate']:.0%} | "
            f"{row['pass_ci_low']:.2f}-{row['pass_ci_high']:.2f} | "
            f"{row['score_mean']:.2f} | {judge} | "
            f"{row['mcp_calls_mean']:.1f} | {row['builtin_calls_mean']:.1f} | "
            f"{row['turns_mean']:.1f} | {row['total_tokens_mean']:,.0f} | "
            f"{row['cost_mean']:.4f} | {row['wall_s_mean']:.1f} |"
        )
    lines.append("")

    lines.append("## By task and shape")
    lines.append("")
    shapes = sorted({r["shape"] for r in records})
    tasks = sorted({r["task"] for r in records})
    lines.append("| task | " + " | ".join(shapes) + " |")
    lines.append("| --- | " + " | ".join("---:" for _ in shapes) + " |")
    for task in tasks:
        cells = []
        for shape in shapes:
            group = [r for r in records if r["task"] == task and r["shape"] == shape]
            if not group:
                cells.append("-")
            else:
                passed = sum(1 for r in group if r["passed"])
                score = sum(_record_score(r) for r in group) / len(group)
                cells.append(f"{passed}/{len(group)} ({score:.2f})")
        lines.append(f"| {task} | " + " | ".join(cells) + " |")
    lines.append("")
    return "\n".join(lines)
