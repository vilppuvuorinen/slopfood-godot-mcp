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


def test_entity_scoped_expect_parsing():
    task = config.Task.from_yaml(EVALS_DIR / "tasks" / "project_setting.yaml")
    assert task.expect["full"].file_edit_any == ("**/project.godot",)
    assert task.expect["full"].used_none == ("godot_run_script",)
    spawn = config.Task.from_yaml(EVALS_DIR / "tasks" / "spawn_pickup.yaml")
    assert spawn.expect["full"].file_edit_none == ("**/main.tscn",)


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


def test_file_edit_checks_are_entity_scoped():
    task = config.Task.from_yaml(EVALS_DIR / "tasks" / "attach_script.yaml")
    shape = config.Shape(name="full", description="", tools={})
    call_log = [
        {"name": "write", "path": "res://player.gd", "args": {}},
        {"name": "godot_attach_script", "path": "res://main.tscn", "args": {}},
    ]
    checks = runner._trajectory_checks(task, shape, ["write", "godot_attach_script"], call_log)
    assert all(result.passed for result in checks)
    # Editing the scene by hand fails the entity check even though it is not the
    # named edit tool's own target.
    bad = runner._trajectory_checks(
        task,
        shape,
        ["write", "godot_attach_script"],
        [{"name": "write", "path": "res://main.tscn", "args": {}}],
    )
    assert any(
        not result.passed and "main.tscn" in result.detail for result in bad
    )
    assert not any(
        result.id.endswith("file_edit_any:**/player.gd") and result.passed for result in bad
    )


def test_file_edit_targets_normalizes_res_prefix():
    targets = runner._file_edit_targets(
        [
            {"name": "write", "path": "res://a.gd"},
            {"name": "edit", "path": "b.tscn"},
            {"name": "write", "path": "/tmp/run/project.godot"},
            {"name": "godot_run_script", "path": "res://c.tscn"},
            {"name": "bash", "path": None},
        ]
    )
    assert targets == {
        "res://a.gd",
        "a.gd",
        "b.tscn",
        "res://b.tscn",
        "/tmp/run/project.godot",
    }


def test_expect_alias_resolution():
    task = config.Task.from_yaml(EVALS_DIR / "tasks" / "project_setting.yaml")
    aliased = config.Shape(name="full_nobash", description="", tools={}, expect_alias="full")
    assert config.resolve_expectation(task, aliased) is task.expect["full"]
    direct = config.Shape(name="full", description="", tools={})
    assert config.resolve_expectation(task, direct) is task.expect["full"]
    other = config.Shape(name="unknown", description="", tools={})
    assert config.resolve_expectation(task, other) is None


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


def test_judge_prompt_includes_legality_and_call_log():
    prompt = judge._build_prompt(
        task_id="t",
        description="d",
        prompt="p",
        rubric="r",
        check_results=[{"id": "t:0", "passed": True, "detail": "ok"}],
        answer="done",
        diff=[{"file": "res://main.tscn", "patch": "@@"}],
        call_log=[{"name": "write", "path": "res://a.gd", "args": {"filePath": "res://a.gd"}}],
    )
    assert judge.LEGALITY_RULE.split(".")[0] in prompt
    assert "Tool-call log" in prompt
    assert "res://a.gd" in prompt


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


def test_cli_jobs_flag():
    assert cli.build_parser().parse_args(["--jobs", "7"]).jobs == 7
    assert cli.build_parser().parse_args([]).jobs is None


def test_cli_server_max_runs_flag():
    parsed = cli.build_parser().parse_args(["--server-max-runs", "7"])
    assert parsed.server_max_runs == 7
    assert cli.build_parser().parse_args([]).server_max_runs is None


def test_suite_server_max_runs_default_and_override(tmp_path):
    assert config.load_suite(EVALS_DIR / "config" / "suite.yaml").server_max_runs == 40
    custom = tmp_path / "suite.yaml"
    custom.write_text("server_max_runs: 5\n")
    assert config.load_suite(custom).server_max_runs == 5
    custom.write_text("server_max_runs: 0\n")
    assert config.load_suite(custom).server_max_runs == 1


def test_plan_batches_caps_batch_size():
    plan = list(range(10))
    assert runner._plan_batches(plan, 4) == [[0, 1, 2, 3], [4, 5, 6, 7], [8, 9]]
    assert runner._plan_batches(plan, 100) == [list(range(10))]
    assert runner._plan_batches([], 4) == []
    assert runner._plan_batches(plan, 0) == [[i] for i in range(10)]


def test_suite_jobs_default_and_override(tmp_path):
    assert config.load_suite(EVALS_DIR / "config" / "suite.yaml").jobs == 4
    custom = tmp_path / "suite.yaml"
    custom.write_text("jobs: 6\n")
    assert config.load_suite(custom).jobs == 6
    custom.write_text("jobs: 0\n")
    assert config.load_suite(custom).jobs == 1


def test_plan_runs_flattens_in_execution_order():
    import types

    def task(task_id):
        return config.Task(
            id=task_id,
            description="",
            fixture=task_id,
            prompt="",
            checks=[],
            rubric="",
            timeout_s=None,
            expect={},
        )

    shapes = {"full": config.Shape(name="full", description="", tools={})}
    options = types.SimpleNamespace(
        selected_tasks=["a", "b"], tasks={"a": task("a"), "b": task("b")}, selected_shapes=["full"]
    )
    plan = runner._plan_runs(options, shapes, {"full": 2})
    assert [p.run_id for p in plan] == [
        "a__full__r1",
        "a__full__r2",
        "b__full__r1",
        "b__full__r2",
    ]
    assert [p.index for p in plan] == [1, 2, 3, 4]


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
    assert set(shapes) == {
        "full",
        "full_nobash",
        "script_escape",
        "script_escape_nobash",
        "read_only",
    }
    assert shapes["full"].tools["bash"] is True
    # script_escape isolates the structured MCP tools only; the model must keep
    # the same built-in escape hatches so its choice of tool stays observable.
    for name in ("bash", "edit", "write", "read", "glob", "grep"):
        assert shapes["script_escape"].tools[name] is True
        assert shapes["full"].tools[name] is True
    for name in ("bash", "edit", "write"):
        assert shapes["read_only"].tools[name] is False
    # The *_nobash variants differ from their parent only by the shell tool, and
    # grade with the parent's expectations.
    assert shapes["full_nobash"].tools["bash"] is False
    assert shapes["full_nobash"].tools["godot_add_node"] is True
    assert shapes["full_nobash"].expect_alias == "full"
    assert shapes["script_escape_nobash"].tools["bash"] is False
    assert shapes["script_escape_nobash"].tools["godot_run_script"] is True
    assert shapes["script_escape_nobash"].expect_alias == "script_escape"


def test_shapes_validate_expectations(tmp_path):
    import types

    shapes = config.load_shapes(
        EVALS_DIR / "config" / "toolsets.yaml",
        REGISTERED_MCP,
        runner.DEFAULT_BUILTIN_TOOLS,
    )
    tasks = config.load_tasks(EVALS_DIR / "tasks")
    options = types.SimpleNamespace(
        tasks=tasks,
        selected_tasks=list(tasks),
        selected_shapes=list(shapes),
    )
    runner._validate_expectations(options, shapes)


def test_validate_expectations_rejects_alias_cycle():
    import types

    shapes = {
        "a": config.Shape(name="a", description="", tools={"edit": True}, expect_alias="missing"),
    }
    options = types.SimpleNamespace(tasks={}, selected_tasks=[], selected_shapes=["a"])
    with pytest.raises(ValueError):
        runner._validate_expectations(options, shapes)


def test_extract_records_call_log():
    messages = [
        {
            "info": {"role": "assistant"},
            "parts": [
                {
                    "type": "tool",
                    "tool": "write",
                    "state": {
                        "status": "completed",
                        "input": {"filePath": "res://a.gd", "content": "x"},
                    },
                },
                {
                    "type": "tool",
                    "tool": "godot_run_script",
                    "state": {"status": "completed", "input": {"scenePath": "res://main.tscn"}},
                },
            ],
        }
    ]
    metrics_out = metrics.extract(messages, wall_ms=1)
    assert metrics_out["tool_names"] == ["write", "godot_run_script"]
    assert metrics_out["call_log"][0] == {
        "name": "write",
        "path": "res://a.gd",
        "args": {"filePath": "res://a.gd", "content": "x"},
    }
    assert metrics_out["call_log"][1]["path"] == "res://main.tscn"
