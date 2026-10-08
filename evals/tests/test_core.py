import shutil
from pathlib import Path

import httpx
import pytest

from godot_mcp_evals import cli, config, isolation, judge, metrics, opencode, runner, verifier

EVALS_DIR = Path(__file__).resolve().parents[1]
REGISTERED_MCP = [
    "attach_script",
    "create_project",
    "export_mesh_library",
    "export_project",
    "read",
    "read_node",
    "run_script",
    "validate",
]
KNOWN_CHECK_KINDS = {
    "node_exists",
    "node_not_exists",
    "node_type",
    "node_property",
    "resource_property",
    "node_script",
    "scene_root_type",
    "file_exists",
    "file_contains",
    "script_has_method",
    "project_setting",
    "answer_contains",
    "file_unchanged",
    "resource_distinct",
    "resource_same",
    "resource_count",
    "node_instance",
    "node_unchanged",
    "animation_exists",
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
    # No tool call is mandatory, so scene-mutation and read tasks assert no
    # trajectory expectation.
    task = config.Task.from_yaml(EVALS_DIR / "tasks" / "add_node.yaml")
    assert task.expect == {}
    read_answer = config.Task.from_yaml(EVALS_DIR / "tasks" / "read_answer.yaml")
    assert read_answer.expect == {}
    # The editability gate is the only expectation the suite keeps.
    binary = config.Task.from_yaml(EVALS_DIR / "tasks" / "binary_resource.yaml")
    assert binary.expect["full"].file_edit_none == ("**/score.res",)
    # Dropped tool-name gates are rejected loudly rather than silently ignored.
    with pytest.raises(ValueError):
        config.Expectation.from_mapping({"used_all": ["godot_run_script"]})


def test_entity_scoped_expect_parsing():
    project = config.Task.from_yaml(EVALS_DIR / "tasks" / "project_setting.yaml")
    assert project.expect == {}
    binary = config.Task.from_yaml(EVALS_DIR / "tasks" / "binary_resource.yaml")
    assert binary.expect["full"].file_edit_none == ("**/score.res",)
    scene = config.Task.from_yaml(EVALS_DIR / "tasks" / "binary_scene.yaml")
    assert scene.expect["full"].file_edit_none == ("**/level.scn",)


def test_trajectory_checks_flag_binary_hand_edits():
    task = config.Task.from_yaml(EVALS_DIR / "tasks" / "binary_scene.yaml")
    shape = config.Shape(name="full", description="", tools={})
    # A Godot-process rewrite does not touch the file as text and passes.
    passing = runner._trajectory_checks(
        task,
        shape,
        [{"name": "godot_run_script", "path": "res://level.scn", "args": {}}],
    )
    assert all(result.passed for result in passing)
    # A file-editing built-in targeting the binary scene corrupts it and fails.
    failing = runner._trajectory_checks(
        task,
        shape,
        [{"name": "write", "path": "res://level.scn", "args": {}}],
    )
    assert not all(result.passed for result in failing)
    assert any(
        result.id.endswith("file_edit_none:**/level.scn") and not result.passed
        for result in failing
    )


def test_file_edit_checks_are_entity_scoped():
    binary = config.Task.from_yaml(EVALS_DIR / "tasks" / "binary_scene.yaml")
    shape = config.Shape(name="full", description="", tools={})
    # Editing an unrelated file leaves the binary target untouched.
    clean = runner._trajectory_checks(
        binary, shape, [{"name": "write", "path": "res://player.gd", "args": {}}]
    )
    assert all(result.passed for result in clean)
    # Hand-rewriting the binary scene fails the editability gate.
    dirty = runner._trajectory_checks(
        binary, shape, [{"name": "write", "path": "res://level.scn", "args": {}}]
    )
    assert any(not result.passed and "level.scn" in result.detail for result in dirty)


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
    task = config.Task.from_yaml(EVALS_DIR / "tasks" / "binary_scene.yaml")
    aliased = config.Shape(name="full-variant", description="", tools={}, expect_alias="full")
    assert config.resolve_expectation(task, aliased) is task.expect["full"]
    direct = config.Shape(name="full", description="", tools={})
    assert config.resolve_expectation(task, direct) is task.expect["full"]
    other = config.Shape(name="unknown", description="", tools={})
    assert config.resolve_expectation(task, other) is None


def test_targets_parsing_and_defaults():
    binary = config.Task.from_yaml(EVALS_DIR / "tasks" / "binary_resource.yaml")
    assert [(t.path, t.form) for t in binary.targets] == [
        ("res://data/score.res", "opaque")
    ]
    unique = config.Task.from_yaml(EVALS_DIR / "tasks" / "unique_shape.yaml")
    assert unique.targets[0].form == "text"
    no_targets = config.Task.from_yaml(EVALS_DIR / "tasks" / "add_node.yaml")
    assert no_targets.targets == ()


def test_target_parsing_rejects_bad_form(tmp_path):
    task = tmp_path / "t.yaml"
    task.write_text(
        "id: t\nfixture: t\nprompt: p\nchecks: []\n"
        "targets:\n  - path: res://x.tscn\n    form: weird\n"
    )
    with pytest.raises(ValueError):
        config.Task.from_yaml(task)


def test_validate_targets_rejects_text_hand_edit_ban():
    import types

    shape = config.Shape(name="full", description="", tools={"edit": True})
    # A text target must not be forbidden a hand edit.
    text_bad = config.Task(
        id="text",
        description="",
        fixture="text",
        prompt="",
        checks=[],
        rubric="",
        timeout_s=None,
        expect={"full": config.Expectation(file_edit_none=("**/main.tscn",))},
        targets=(config.Target(path="res://main.tscn", form="text"),),
    )
    options = types.SimpleNamespace(
        tasks={"text": text_bad}, selected_tasks=["text"], selected_shapes=["full"]
    )
    with pytest.raises(ValueError, match="text"):
        runner._validate_targets(options, {"full": shape})


def test_new_engine_check_params_round_trip():
    check = config.Check.from_mapping(
        {
            "kind": "resource_distinct",
            "scene": "res://main.tscn",
            "path_a": "A",
            "property_a": "shape",
            "path_b": "B",
            "property_b": "shape",
            "weight": 2,
            "required": True,
        }
    )
    assert check.kind == "resource_distinct"
    assert check.params["path_a"] == "A"
    assert check.params["path_b"] == "B"
    assert check.weight == 2
    assert check.required is True
    assert "weight" not in check.params and "required" not in check.params
    anim = config.Check.from_mapping(
        {
            "kind": "animation_exists",
            "scene": "s",
            "node": "Anim",
            "name": "bob",
            "paths": ["Sprite:position"],
        }
    )
    assert anim.params["paths"] == ["Sprite:position"]


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
    assert shapes["full"].reps is None
    assert runner.effective_reps(shapes["full"], None, 3) == 3
    assert runner.effective_reps(shapes["full"], 5, 3) == 5
    # A shape-level reps overrides both the suite default and the CLI flag.
    custom = config.Shape(name="custom", description="", tools={}, reps=1)
    assert custom.reps == 1
    assert runner.effective_reps(custom, None, 3) == 1
    assert runner.effective_reps(custom, 5, 3) == 1


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
    assert shapes["full"].tools["godot_run_script"] is True
    assert all(
        shapes["full"].tools[name] is True
        for name in (
            "godot_run_script",
            "godot_read",
            "godot_read_node",
            "godot_create_project",
        )
    )
    # no_mcp is the baseline control: every MCP tool stays off.
    assert all(shapes["no_mcp"].tools[f"godot_{name}"] is False for name in REGISTERED_MCP)
    assert set(shapes) == {
        "full",
        "no_mcp",
    }
    assert shapes["full"].tools["bash"] is True
    # no_mcp keeps the same built-in escape hatches as full so the only
    # variable is the MCP server itself.
    for name in ("bash", "edit", "write", "read", "glob", "grep"):
        assert shapes["no_mcp"].tools[name] is True
        assert shapes["full"].tools[name] is True


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


def test_run_parent_is_private(tmp_path):
    parent = isolation.run_parent()
    try:
        assert list(parent.iterdir()) == []
        project = parent / "project"
        project.mkdir()
        assert [entry.name for entry in parent.iterdir()] == ["project"]
        # No harness artifacts share the project's parent; ``..`` sees only the
        # project, not judge sessions, server config or sibling runs.
        assert not (parent / "judge").exists()
        assert not (parent / "oc-config").exists()
        assert not (parent / "opencode-serve.log").exists()
    finally:
        shutil.rmtree(parent, ignore_errors=True)


def test_isolation_instruction_names_the_project(tmp_path):
    text = isolation.instruction(tmp_path)
    assert str(tmp_path) in text
    assert "outside" in text


def test_isolation_flags_calls_outside_the_project(tmp_path):
    project = tmp_path / "run" / "project"
    project.mkdir(parents=True)
    (project / "main.tscn").write_text("x")
    clean = [
        {"name": "read", "args": {"filePath": str(project / "main.tscn")}},
        {"name": "read", "args": {"filePath": "sub/../main.tscn"}},
        {"name": "write", "args": {"filePath": "res://a.gd"}},
        {"name": "bash", "args": {"command": "mkdir -p exported && cat main.tscn"}},
        {"name": "bash", "args": {"command": "ls -la /dev/null"}},
        {"name": "godot_run_script", "args": {"scenePath": "/elsewhere/x.tscn"}},
    ]
    assert isolation.escapes(project, clean) == []

    escape_calls = [
        {"name": "read", "args": {"filePath": str(tmp_path / "sibling" / "main.tscn")}},
        {"name": "bash", "args": {"command": "ls -la .."}},
        {"name": "bash", "args": {"command": "cat /etc/passwd"}},
        {"name": "bash", "args": {"command": "grep -rn x /home/developer/sandboxes/go-dot-mcp"}},
        {"name": "bash", "args": {"command": "find / -iname '*.res'"}},
    ]
    found = isolation.escapes(project, escape_calls)
    assert len(found) == len(escape_calls)
    assert all(entry.startswith(("read:", "bash:")) for entry in found)


def test_sandbox_check_is_required_but_weightless():
    clean = runner._sandbox_checks([])
    assert clean[0].passed and clean[0].required and clean[0].weight == 0.0
    dirty = runner._sandbox_checks(["bash: .."])
    assert not dirty[0].passed and dirty[0].required and dirty[0].weight == 0.0
    assert runner._weighted_score(
        dirty + [verifier.CheckResult("a", "node_exists", True, "", weight=1.0)]
    ) == pytest.approx(1.0)


def test_request_timeout_raises_opencode_timeout(monkeypatch):
    client = opencode.OpencodeClient("http://127.0.0.1:1")

    def boom(*_args, **_kwargs):
        raise httpx.ReadTimeout("timed out")

    monkeypatch.setattr(client._client, "request", boom)
    try:
        with pytest.raises(opencode.OpencodeTimeout):
            client.health()
    finally:
        client.close()


def _metric_record(shape: str, *, timed_out: bool = False, escaped: bool = False) -> dict:
    return {
        "shape": shape,
        "task": "t",
        "passed": not (timed_out or escaped),
        "score": 0.0 if (timed_out or escaped) else 1.0,
        "judge": {},
        "metrics": {
            "mcp_tool_calls": 0,
            "builtin_tool_calls": 0,
            "turns": 0,
            "total_tokens": 0,
            "cost": 0.0,
            "wall_ms": 0,
            "errors": [],
        },
        "timed_out": timed_out,
        "escaped": escaped,
    }


def test_aggregate_reports_timeouts_and_escapes_separately():
    rows = metrics.aggregate(
        [
            _metric_record("full", timed_out=True),
            _metric_record("full", escaped=True),
            _metric_record("full"),
        ],
        "shape",
    )
    assert rows[0]["timeouts"] == 1
    assert rows[0]["escapes"] == 1
    assert rows[0]["runs"] == 3


def test_every_task_defines_a_positive_timeout():
    tasks = config.load_tasks(EVALS_DIR / "tasks")
    for task in tasks.values():
        assert task.timeout_s and task.timeout_s > 0, f"{task.id}: missing timeout_s"
