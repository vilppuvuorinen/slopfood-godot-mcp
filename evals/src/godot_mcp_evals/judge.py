"""LLM judge: score a run's outcome through opencode structured output."""

from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any

from .opencode import OpencodeClient, OpencodeError

JUDGE_SCHEMA: dict[str, Any] = {
    "type": "object",
    "properties": {
        "pass": {"type": "boolean"},
        "score": {"type": "number", "minimum": 0, "maximum": 1},
        "reasons": {"type": "array", "items": {"type": "string"}},
    },
    "required": ["pass", "score", "reasons"],
    "additionalProperties": False,
}

JUDGE_SYSTEM = (
    "You are a strict evaluator of Godot project edits. You are given a task, the "
    "deterministic check results, the agent's tool-call log, its final answer and the "
    "file diff. Judge whether the task was actually accomplished, not whether the agent "
    "claimed success. Also flag any edit that violates the legality rule below. Return "
    "only the requested JSON. Do not use tools."
)

# The rule the suite exists to measure: Godot entities with no text form (binary
# .res/.scn and other non-text-serializable resources) cannot be hand-edited at
# all and must be mutated through a Godot process. Text files, including
# .tscn/.tres scenes, may be edited as text; file-editable entities such as
# scripts and project.godot should go through the file tools.
LEGALITY_RULE = (
    "A Godot file that has no text form (a binary .res or .scn, or any other "
    "non-text-serializable resource) must be mutated through a Godot process: an "
    "MCP godot_* tool or run_script. Rewriting its bytes with edit/write or a shell "
    "command corrupts it and is an illegal edit even though it does not call a "
    "scene tool. Text-serialized files (.tscn/.tres, .gd, project.godot) may be "
    "edited as text; file-editable entities (GDScript, project.godot, importable "
    "source assets) should be edited with the edit/write tools rather than "
    "rewritten through run_script."
)


def judge_run(
    client: OpencodeClient,
    *,
    model: dict[str, str],
    judge_dir: Path,
    disabled_tools: dict[str, bool],
    task_id: str,
    description: str,
    prompt: str,
    rubric: str,
    check_results: list[dict[str, Any]],
    answer: str,
    diff: list[dict[str, Any]],
    timeout_s: float,
    call_log: list[dict[str, Any]] | None = None,
) -> dict[str, Any]:
    judge_dir.mkdir(parents=True, exist_ok=True)
    user = _build_prompt(
        task_id=task_id,
        description=description,
        prompt=prompt,
        rubric=rubric,
        check_results=check_results,
        answer=answer,
        diff=diff,
        call_log=call_log or [],
    )
    try:
        session_id = client.create_session(str(judge_dir), f"judge-{task_id}")
        response = client.prompt(
            session_id,
            str(judge_dir),
            model=model,
            agent="build",
            tools=disabled_tools,
            parts=[{"type": "text", "text": user}],
            timeout_s=timeout_s,
            system=JUDGE_SYSTEM,
            output_format={"type": "json_schema", "schema": JUDGE_SCHEMA},
        )
    except OpencodeError as exc:
        return {"error": str(exc), "pass": None, "score": None, "reasons": []}

    text = "\n".join(
        str(part.get("text", ""))
        for part in response.get("parts", [])
        if part.get("type") == "text"
    )
    parsed = _structured_output(response) or _parse_json(text)
    if parsed is None:
        return {
            "error": "judge returned no parseable JSON",
            "raw": text[:1000],
            "pass": None,
            "score": None,
            "reasons": [],
        }
    return {
        "pass": bool(parsed.get("pass")),
        "score": float(parsed.get("score", 0.0)),
        "reasons": [str(r) for r in parsed.get("reasons", [])],
        "raw": parsed,
    }


def _build_prompt(
    *,
    task_id: str,
    description: str,
    prompt: str,
    rubric: str,
    check_results: list[dict[str, Any]],
    answer: str,
    diff: list[dict[str, Any]],
    call_log: list[dict[str, Any]],
) -> str:
    checks = "\n".join(
        f"- [{ 'PASS' if c['passed'] else 'FAIL' }] {c['id']}: {c['detail']}" for c in check_results
    )
    diff_text = _render_diff(diff)
    return (
        f"# Task: {task_id}\n{description}\n\n"
        f"## Instruction given to the agent\n{prompt}\n\n"
        f"## Rubric\n{rubric or 'The task instruction should be satisfied exactly.'}\n\n"
        f"## Legality rule (flag violations in reasons)\n{LEGALITY_RULE}\n\n"
        f"## Deterministic checks\n{checks or '(none)'}\n\n"
        f"## Tool-call log\n{_render_call_log(call_log)}\n\n"
        f"## Agent final answer\n{answer or '(empty)'}\n\n"
        f"## File diff\n{diff_text}\n"
    )


def _render_call_log(call_log: list[dict[str, Any]], limit: int = 6000) -> str:
    if not call_log:
        return "(no tool calls recorded)"
    lines = []
    for call in call_log:
        name = call.get("name") or "?"
        path = call.get("path")
        args = call.get("args") or {}
        rendered = json.dumps(args, ensure_ascii=False)
        target = f" -> {path}" if path else ""
        lines.append(f"- {name}{target}: {rendered}")
    return "\n".join(lines)[:limit]


def _render_diff(diff: list[dict[str, Any]], limit: int = 6000) -> str:
    if not diff:
        return "(no diff recorded)"
    chunks = []
    for entry in diff:
        path = entry.get("file") or entry.get("path") or "?"
        patch = entry.get("patch") or entry.get("diff") or ""
        chunks.append(f"--- {path}\n{patch}")
    return "\n".join(chunks)[:limit]


def _structured_output(response: dict[str, Any]) -> dict[str, Any] | None:
    """Structured output arrives as a synthetic ``StructuredOutput`` tool call."""
    for part in response.get("parts", []):
        if part.get("type") != "tool":
            continue
        if str(part.get("tool", "")).lower() != "structuredoutput":
            continue
        payload = (part.get("state") or {}).get("input")
        if isinstance(payload, dict):
            return payload
    return None


def _parse_json(text: str) -> dict[str, Any] | None:
    text = text.strip()
    if not text:
        return None
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        pass
    match = re.search(r"\{.*\}", text, re.DOTALL)
    if not match:
        return None
    try:
        return json.loads(match.group(0))
    except json.JSONDecodeError:
        return None
