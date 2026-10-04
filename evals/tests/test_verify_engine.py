"""Engine-backed regression tests for the structural verifier checks.

Each new check kind is exercised twice: once against a correct artifact
(expected to pass) and once against a deliberately wrong one (expected to
fail), so the check cannot silently degrade into a tautology. These tests need
the pinned Godot binary; they skip when it is unavailable.
"""

from __future__ import annotations

import shutil
from pathlib import Path

import pytest

from godot_mcp_evals import paths, verifier

FIXTURE = Path(__file__).resolve().parent / "verify" / "fixture"


def _godot() -> Path:
    try:
        return paths.godot_binary()
    except paths.DiscoveryError:
        pytest.skip("no pinned Godot binary available")


@pytest.fixture()
def project(tmp_path: Path) -> Path:
    dest = tmp_path / "project"
    shutil.copytree(FIXTURE, dest)
    return dest


def _run(project: Path, tmp_path: Path, checks: list[dict]) -> dict[str, bool]:
    results = verifier._engine_checks(
        project,
        _godot(),
        tmp_path / "verify.log",
        checks,
        timeout_s=120,
        fixture_dir=FIXTURE,
    )
    return {result.id: result.passed for result in results}


def _scene_check(check_id: str, kind: str, scene: str, **params) -> dict:
    return {"id": check_id, "kind": kind, "scene": f"res://{scene}", **params}


def test_resource_distinct_and_same(project, tmp_path):
    passing = _run(
        project,
        tmp_path,
        [
            _scene_check(
                "distinct",
                "resource_distinct",
                "distinct.good.tscn",
                path_a="WallA/CollisionShape2D",
                property_a="shape",
                path_b="WallB/CollisionShape2D",
                property_b="shape",
            )
        ],
    )
    assert passing["distinct"] is True

    failing = _run(
        project,
        tmp_path,
        [
            _scene_check(
                "distinct",
                "resource_distinct",
                "shared.bad.tscn",
                path_a="WallA/CollisionShape2D",
                property_a="shape",
                path_b="WallB/CollisionShape2D",
                property_b="shape",
            ),
            _scene_check(
                "same",
                "resource_same",
                "shared.bad.tscn",
                path_a="WallA/CollisionShape2D",
                property_a="shape",
                path_b="WallB/CollisionShape2D",
                property_b="shape",
            ),
        ],
    )
    assert failing["distinct"] is False
    assert failing["same"] is True


def test_resource_count_catches_orphans_and_duplicates(project, tmp_path):
    results = _run(
        project,
        tmp_path,
        [
            _scene_check(
                "two", "resource_count", "distinct.good.tscn", type="RectangleShape2D", min=2, max=2
            ),
            _scene_check(
                "one", "resource_count", "shared.bad.tscn", type="RectangleShape2D", min=2, max=2
            ),
            _scene_check(
                "orphan", "resource_count", "orphan.bad.tscn", type="RectangleShape2D", max=1
            ),
        ],
    )
    assert results["two"] is True
    assert results["one"] is False
    assert results["orphan"] is False


def test_node_instance_rejects_inlined_copy(project, tmp_path):
    results = _run(
        project,
        tmp_path,
        [
            _scene_check("real", "node_instance", "instance.good.tscn", path="Enemy", scene_file="res://enemy.tscn"),
            _scene_check("inlined", "node_instance", "inlined.bad.tscn", path="Enemy", scene_file="res://enemy.tscn"),
        ],
    )
    assert results["real"] is True
    assert results["inlined"] is False


def test_animation_exists(project, tmp_path):
    results = _run(
        project,
        tmp_path,
        [
            _scene_check(
                "anim",
                "animation_exists",
                "animation.good.tscn",
                node="Anim",
                name="bob",
                length=1.5,
                tracks=1,
                paths=["Sprite:position"],
            ),
            _scene_check(
                "missing", "animation_exists", "animation.good.tscn", node="Anim", name="pop"
            ),
        ],
    )
    assert results["anim"] is True
    assert results["missing"] is False


def test_node_unchanged_compares_against_fixture(project, tmp_path):
    # The project starts as a copy of the fixture, so the node matches; then
    # mutate the graded copy and the check must fail against the fixture.
    check = [
        _scene_check("unchanged", "node_unchanged", "unchanged.tscn", path="Player")
    ]
    assert _run(project, tmp_path, check)["unchanged"] is True

    graded = project / "unchanged.tscn"
    graded.write_text(graded.read_text().replace("\n", "\nposition = Vector2(42, 42)\n", 1))
    assert _run(project, tmp_path, check)["unchanged"] is False


def test_fixture_copy_is_not_persisted(project, tmp_path):
    # node_unchanged makes the harness copy the fixture into the project for the
    # engine; the copy (and the engine's own files) must be cleaned up after.
    results = _run(
        project,
        tmp_path,
        [_scene_check("x", "node_unchanged", "unchanged.tscn", path="Player")],
    )
    assert results["x"] is True
    assert not (project / "evals_fixture").exists()
    assert not (project / "evals_checks.json").exists()
    assert not (project / "evals_verify.gd").exists()
