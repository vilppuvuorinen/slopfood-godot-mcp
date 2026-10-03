from pathlib import Path

import pytest

from godot_mcp_evals import cli, config, judge, metrics, runner, verifier

EVALS_DIR = Path(__file__).resolve().parents[1]
REGISTERED_MCP = [
    "add_node",
    "attach_script",
    "create_project",
    "create_scene",
    "duplicate_scene_node",
    "export_mesh_library",
    "export_project",
    "get_godot_version",
    "modify_scene_node",
    "move_scene_node",
    "read",
    "read_node",
    "remove_scene_node",
    "rename_scene_node",
    "rescan_project",
    "run_script",
    "validate_project",
    "validate_scripts",
]
KNOWN_CHECK_KINDS = {
    "node_exists",
    "node_not_exists",
    "node_type",
    "node_property",
    "node_script",
    "scene_root_type",
    "file_exists",
    "file_contains",
    "script_has_method",
    "project_setting",
    "answer_contains",
    "file_unchanged",
}


def test_resolve_shape_all_and_builtin():
    tools = config.resolve_shape_tools("all", ["read"], ["a", "b"], ["read", "write"])
    assert tools == {"godot_a": True, "godot_b": True, "read": True, "write": False}


def test_resolve_shape_except():
    tools = config.resolve_shape_tools(
        "all-except:b", ["read"], ["a", "b", "c"], ["read"]
    )
    assert tools["godot_a"] is True
    assert tools["godot_b"] is False
    assert tools["godot_c"] is True


def test_resolve_shape_rejects_unknown():
    with pytest.raises(ValueError):
        config.resolve_shape_tools(["nope"], [], ["a"], [])
    with pytest.raises(ValueError):
        config.resolve_shape_tools("all", ["nope"], ["a"], ["read"])
    with pytest.raises(ValueError):
        config.resolve_shape_tools("all-except:nope", [], ["a", "b"], [])


def test_expect_parsing_and_defaults():
    task = config.Task.from_yaml(EVALS_DIR / "tasks" / "add_node.yaml")
    assert task.expect["full"].used_all == ("godot_add_node",)
    assert task.expect["full"].used_none == ("edit", "write")
    assert task.expect["script_escape"].used_all == ("godot_run_script",)
    read_answer = config.Task.from_yaml(EVALS_DIR / "tasks" / "read_answer.yaml")
    assert read_answer.expect["full"].used_any == ("godot_read", "godot_read_node")
    assert read_answer.expect["full"].used_none == ("edit", "write")


def test_trajectory_checks_pass_and_fail():
    task = config.Task.from_yaml(EVALS_DIR / "tasks" / "rename_move.yaml")
    shape = config.Shape(name="full", description="", tools={})
    passing = runner._trajectory_checks(
        task,
        shape,
        ["godot_rename_scene_node", "godot_move_scene_node"],
    )
    assert all(result.passed for result in passing)
    failing = runner._trajectory_checks(task, shape, ["godot_run_script", "edit"])
    assert not all(result.passed for result in failing)
    assert any(result.id.endswith("used_none:edit") for result in failing if not result.passed)


def test_check_weight_and_required_parsing():
    check = config.Check.from_mapping(
        {"kind": "node_exists", "path": "X", "weight": 2.5, "required": True}
    )
    assert check.weight == 2.5
    assert check.required is True
    # weight/required are harness metadata, not engine params.
    assert check.params == {"path": "X"}
    default = config.Check.from_mapping({"kind": "node_exists", "path": "X"})
    assert default.weight == 1.0
    assert default.required is False
    with pytest.raises(ValueError):
        config.Check.from_mapping({"kind": "node_exists", "weight": -1})


def test_min_score_parsing_and_default():
    task = config.Task.from_yaml(EVALS_DIR / "tasks" / "build_arena_scene.yaml")
    assert task.min_score == pytest.approx(0.85)
    assert any(check.required for check in task.checks)
    assert any(check.weight > 1 for check in task.checks)
    binary = config.Task.from_yaml(EVALS_DIR / "tasks" / "add_node.yaml")
    assert binary.min_score is None


def test_weighted_score_and_threshold():
    checks = [
        verifier.CheckResult("a", "node_exists", True, "", weight=3.0),
        verifier.CheckResult("b", "node_exists", False, "", weight=1.0),
    ]
    assert runner._weighted_score(checks) == pytest.approx(0.75)
    assert runner._weighted_score([]) == 0.0


def test_file_unchanged_check(tmp_path):
    fixture = tmp_path / "fixture"
    project = tmp_path / "project"
    fixture.mkdir()
    project.mkdir()
    (fixture / "main.tscn").write_text("original")
    (project / "main.tscn").write_text("original")
    check = config.Check(kind="file_unchanged", params={"path": "res://main.tscn"})
    same = verifier._file_unchanged_check("t", 0, check, project, fixture)
    assert same.passed
    (project / "main.tscn").write_text("changed")
    assert not verifier._file_unchanged_check("t", 0, check, project, fixture).passed


def test_shape_reps_override():
    shapes = config.load_shapes(
        EVALS_DIR / "config" / "toolsets.yaml",
        REGISTERED_MCP,
        runner.DEFAULT_BUILTIN_TOOLS,
    )
    assert shapes["read_only"].reps == 1
    assert shapes["full"].reps is None
    assert runner.effective_reps(shapes["read_only"], None, 3) == 1
    assert runner.effective_reps(shapes["full"], None, 3) == 3
    assert runner.effective_reps(shapes["read_only"], 5, 3) == 1
    assert runner.effective_reps(shapes["full"], 5, 3) == 5


def test_wilson_is_bounded():
    low, high = metrics.wilson(3, 3)
    assert 0.0 <= low <= high <= 1.0
    assert high == pytest.approx(1.0)
    assert metrics.wilson(0, 0) == (0.0, 0.0)


def test_parse_json_tolerates_prose_and_fences():
    assert judge._parse_json('```json\n{"a": 1}\n```') == {"a": 1}
    assert judge._parse_json('here you go: {"b": true} thanks') == {"b": True}
    assert judge._parse_json("nothing") is None


def test_structured_output_extraction():
    response = {
        "parts": [
            {"type": "text", "text": ""},
            {"type": "tool", "tool": "StructuredOutput", "state": {"input": {"pass": True}}},
        ]
    }
    assert judge._structured_output(response) == {"pass": True}


def test_select_helper():
    assert cli._select("a,b", "all", ["x"]) == ["a", "b"]
    assert cli._select(None, "all", ["x", "y"]) == ["x", "y"]
    assert cli._select(None, ["x"], ["y"]) == ["x"]


def test_every_task_has_a_fixture_and_known_checks():
    tasks = config.load_tasks(EVALS_DIR / "tasks")
    assert tasks
    for task in tasks.values():
        fixture = EVALS_DIR / "fixtures" / task.fixture
        assert fixture.is_dir(), f"{task.id}: missing fixture {fixture}"
        if not task.bootstraps_project:
            assert (fixture / "project.godot").is_file(), (
                f"{task.id}: fixture has no project.godot"
            )
        assert task.checks, f"{task.id}: no checks"
        for check in task.checks:
            assert check.kind in KNOWN_CHECK_KINDS, f"{task.id}: unknown check {check.kind}"


def test_default_shapes_resolve_against_registered_tools():
    shapes = config.load_shapes(
        EVALS_DIR / "config" / "toolsets.yaml",
        REGISTERED_MCP,
        ["read", "glob", "grep", "bash", "edit", "write"],
    )
    assert shapes["read_only"].tools["godot_run_script"] is False
    assert shapes["script_escape"].tools["godot_run_script"] is True
    assert shapes["script_escape"].tools["godot_rescan_project"] is True
    assert all(
        shapes["script_escape"].tools[f"godot_{name}"] is False
        for name in (
            "add_node",
            "attach_script",
            "create_scene",
            "duplicate_scene_node",
            "modify_scene_node",
            "move_scene_node",
            "remove_scene_node",
            "rename_scene_node",
        )
    )
    assert set(shapes) == {"full", "script_escape", "read_only"}
    assert shapes["full"].tools["bash"] is True
    # script_escape isolates the structured MCP tools only; the model must keep
    # the same built-in escape hatches so its choice of tool stays observable.
    for name in ("bash", "edit", "write", "read", "glob", "grep"):
        assert shapes["script_escape"].tools[name] is True
        assert shapes["full"].tools[name] is True
    for name in ("bash", "edit", "write"):
        assert shapes["read_only"].tools[name] is False
