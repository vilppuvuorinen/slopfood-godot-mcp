"""Deterministic verification: run the pinned Godot engine over task checks."""

from __future__ import annotations

import dataclasses
import json
import shutil
import subprocess
from pathlib import Path

from .config import Check, Task

RESULT_PREFIX = "EVALS_RESULT "


@dataclasses.dataclass
class CheckResult:
    id: str
    kind: str
    passed: bool
    detail: str
    weight: float = 1.0
    required: bool = False


def verifier_script() -> Path:
    path = Path(__file__).resolve().parents[2] / "verify" / "godot_verify.gd"
    if not path.is_file():
        raise FileNotFoundError(f"verifier script missing: {path}")
    return path


def run_checks(
    task: Task,
    project_dir: Path,
    godot: Path,
    result_path: Path,
    *,
    answer: str,
    fixture_dir: Path | None = None,
    timeout_s: float,
) -> list[CheckResult]:
    """Evaluate all checks; answer/file checks run in Python, the rest in Godot."""
    results: list[CheckResult] = []
    engine_checks: list[dict[str, object]] = []
    indexed = list(enumerate(task.checks))
    for index, check in indexed:
        if check.kind == "answer_contains":
            results.append(_answer_check(task.id, index, check, answer))
        elif check.kind == "file_unchanged":
            results.append(
                _file_unchanged_check(task.id, index, check, project_dir, fixture_dir)
            )
        else:
            entry = {
                "id": _check_id(task.id, index, check),
                "kind": check.kind,
                "weight": check.weight,
                "required": check.required,
            }
            entry.update(check.params)
            engine_checks.append(entry)

    if engine_checks:
        results.extend(
            _engine_checks(project_dir, godot, result_path, engine_checks, timeout_s)
        )

    order = {_check_id(task.id, i, c): i for i, c in indexed}
    return sorted(results, key=lambda r: order.get(r.id, 1_000_000))


def _engine_checks(
    project_dir: Path,
    godot: Path,
    result_path: Path,
    checks: list[dict[str, object]],
    timeout_s: float,
) -> list[CheckResult]:
    checks_path = project_dir / "evals_checks.json"
    script_path = project_dir / "evals_verify.gd"
    checks_path.write_text(json.dumps(checks, indent=2))
    shutil.copyfile(verifier_script(), script_path)
    try:
        completed = subprocess.run(
            [
                str(godot),
                "--headless",
                "--path",
                str(project_dir),
                "--script",
                "res://evals_verify.gd",
            ],
            capture_output=True,
            text=True,
            timeout=timeout_s,
            cwd=project_dir,
            check=False,
        )
    except subprocess.TimeoutExpired:
        return [
            _engine_result(
                check, passed=False, detail=f"verifier timed out after {timeout_s:.0f}s"
            )
            for check in checks
        ]
    finally:
        # Do not leave the harness's own files behind in the graded project.
        for leftover in (checks_path, script_path):
            try:
                leftover.unlink()
            except FileNotFoundError:
                pass

    result_path.write_text(
        f"# exit={completed.returncode}\n"
        f"# stdout\n{completed.stdout}\n"
        f"# stderr\n{completed.stderr}\n"
    )

    payload: dict | None = None
    for line in completed.stdout.splitlines():
        if line.startswith(RESULT_PREFIX):
            payload = json.loads(line[len(RESULT_PREFIX) :])
    if payload is None or "results" not in payload:
        reason = payload.get("error") if payload else "no EVALS_RESULT line from engine"
        return [
            _engine_result(check, passed=False, detail=f"verifier error: {reason}")
            for check in checks
        ]

    by_id = {str(item.get("id")): item for item in payload["results"]}
    out = []
    for check in checks:
        item = by_id.get(str(check["id"]), {})
        out.append(
            _engine_result(
                check,
                passed=bool(item.get("passed", False)),
                detail=str(item.get("detail", item.get("error", "missing result"))),
            )
        )
    return out


def _engine_result(check: dict[str, object], *, passed: bool, detail: str) -> CheckResult:
    return CheckResult(
        id=str(check["id"]),
        kind=str(check["kind"]),
        passed=passed,
        detail=detail,
        weight=float(check.get("weight", 1.0)),
        required=bool(check.get("required", False)),
    )


def _file_unchanged_check(
    task_id: str,
    index: int,
    check: Check,
    project_dir: Path,
    fixture_dir: Path | None,
) -> CheckResult:
    rel = _relative_path(str(check.params.get("path", "")))
    result_id = _check_id(task_id, index, check)
    if fixture_dir is None or not rel:
        return _scored(
            result_id, check, False, "file_unchanged requires a fixture and a path"
        )
    original = fixture_dir / rel
    current = project_dir / rel
    if not original.is_file():
        return _scored(result_id, check, False, f"fixture has no {rel}")
    if not current.is_file():
        return _scored(result_id, check, False, f"{rel} was deleted")
    changed = original.read_bytes() != current.read_bytes()
    return _scored(result_id, check, not changed, f"{rel} {'changed' if changed else 'unchanged'}")


def _relative_path(path: str) -> str:
    return path[len("res://") :] if path.startswith("res://") else path


def _answer_check(task_id: str, index: int, check: Check, answer: str) -> CheckResult:
    wanted = str(check.params.get("value", ""))
    found = wanted.casefold() in answer.casefold()
    return _scored(
        _check_id(task_id, index, check),
        check,
        found,
        f"answer {'contains' if found else 'missing'} {wanted!r}",
    )


def _scored(result_id: str, check: Check, passed: bool, detail: str) -> CheckResult:
    return CheckResult(
        id=result_id,
        kind=check.kind,
        passed=passed,
        detail=detail,
        weight=check.weight,
        required=check.required,
    )


def _check_id(task_id: str, index: int, check: Check) -> str:
    explicit = check.params.get("id")
    if explicit:
        return f"{task_id}:{explicit}"
    return f"{task_id}:{index}:{check.kind}"
