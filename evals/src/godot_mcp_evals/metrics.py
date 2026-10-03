"""Metric extraction and aggregation."""

from __future__ import annotations

import math
from collections import defaultdict
from typing import Any


def extract(messages: list[dict[str, Any]], wall_ms: int) -> dict[str, Any]:
    """Summarise tokens, cost, tool usage and errors from session messages."""
    turns = 0
    tool_names: list[str] = []
    tool_errors = 0
    text_parts: list[str] = []
    errors: list[str] = []
    totals = {
        "input_tokens": 0,
        "output_tokens": 0,
        "reasoning_tokens": 0,
        "cache_read_tokens": 0,
        "cache_write_tokens": 0,
        "total_tokens": 0,
        "cost": 0.0,
    }

    for message in messages:
        info = message.get("info", {}) or {}
        parts = message.get("parts", []) or []
        if info.get("role") == "assistant":
            turns += 1
            tokens = info.get("tokens") or {}
            cache = tokens.get("cache") or {}
            totals["input_tokens"] += int(tokens.get("input") or 0)
            totals["output_tokens"] += int(tokens.get("output") or 0)
            totals["reasoning_tokens"] += int(tokens.get("reasoning") or 0)
            totals["cache_read_tokens"] += int(cache.get("read") or 0)
            totals["cache_write_tokens"] += int(cache.get("write") or 0)
            totals["total_tokens"] += int(tokens.get("total") or 0)
            totals["cost"] += float(info.get("cost") or 0.0)
            if info.get("error"):
                errors.append(_error_text(info["error"]))

        for part in parts:
            kind = part.get("type")
            if kind == "tool":
                name = str(part.get("tool") or part.get("name") or "")
                tool_names.append(name)
                state = part.get("state") or {}
                if state.get("status") == "error":
                    tool_errors += 1
            elif kind == "text" and info.get("role") == "assistant":
                text_parts.append(str(part.get("text", "")))

    mcp_calls = [name for name in tool_names if name.startswith("godot_")]
    return {
        "turns": turns,
        "tool_calls": len(tool_names),
        "mcp_tool_calls": len(mcp_calls),
        "builtin_tool_calls": len(tool_names) - len(mcp_calls),
        "tool_names": tool_names,
        "tool_errors": tool_errors,
        "input_tokens": totals["input_tokens"],
        "output_tokens": totals["output_tokens"],
        "reasoning_tokens": totals["reasoning_tokens"],
        "cache_read_tokens": totals["cache_read_tokens"],
        "cache_write_tokens": totals["cache_write_tokens"],
        "total_tokens": totals["total_tokens"],
        "cost": round(totals["cost"], 6),
        "wall_ms": wall_ms,
        "errors": errors,
        "final_answer": "\n".join(text_parts).strip(),
    }


def _error_text(error: Any) -> str:
    if isinstance(error, dict):
        data = error.get("data")
        if isinstance(data, dict) and data.get("message"):
            return str(data["message"])
        if error.get("name"):
            return str(error["name"])
    return str(error)


def wilson(k: int, n: int, z: float = 1.96) -> tuple[float, float]:
    """Wilson score interval for a binomial proportion."""
    if n == 0:
        return (0.0, 0.0)
    phat = k / n
    denom = 1 + z * z / n
    center = (phat + z * z / (2 * n)) / denom
    margin = (z * math.sqrt(phat * (1 - phat) / n + z * z / (4 * n * n))) / denom
    return (max(0.0, center - margin), min(1.0, center + margin))


def _mean(values: list[float]) -> float:
    return sum(values) / len(values) if values else 0.0


def _score(record: dict[str, Any]) -> float:
    """Weighted check score; older records predate it, so fall back to pass."""
    if "score" in record:
        return float(record["score"])
    return 1.0 if record["passed"] else 0.0


def aggregate(records: list[dict[str, Any]], key: str) -> list[dict[str, Any]]:
    """Aggregate run records by ``key`` (e.g. ``shape`` or ``task``)."""
    groups: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for record in records:
        groups[str(record[key])].append(record)

    rows = []
    for name, group in sorted(groups.items()):
        passes = sum(1 for r in group if r["passed"])
        n = len(group)
        low, high = wilson(passes, n)
        judge_scores = [
            r["judge"]["score"]
            for r in group
            if r.get("judge", {}).get("score") is not None
        ]
        rows.append(
            {
                key: name,
                "runs": n,
                "passes": passes,
                "pass_rate": passes / n if n else 0.0,
                "pass_ci_low": low,
                "pass_ci_high": high,
                "score_mean": _mean([_score(r) for r in group]),
                "judge_mean": _mean([float(s) for s in judge_scores]),
                "judge_scored": len(judge_scores),
                "mcp_calls_mean": _mean([float(r["metrics"]["mcp_tool_calls"]) for r in group]),
                "builtin_calls_mean": _mean(
                    [float(r["metrics"]["builtin_tool_calls"]) for r in group]
                ),
                "turns_mean": _mean([float(r["metrics"]["turns"]) for r in group]),
                "total_tokens_mean": _mean(
                    [float(r["metrics"]["total_tokens"]) for r in group]
                ),
                "cost_mean": _mean([float(r["metrics"]["cost"]) for r in group]),
                "wall_s_mean": _mean([float(r["metrics"]["wall_ms"]) / 1000 for r in group]),
                "error_runs": sum(1 for r in group if r["metrics"]["errors"]),
            }
        )
    return rows
