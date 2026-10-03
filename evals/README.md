# godot-mcp-evals

A small, self-contained evals suite that compares **toolset shapes** of the
`slopfood-godot-mcp` server. Each run asks a model to complete a Godot editing
task through an MCP server whose exposed tools are filtered to a named shape,
then scores the result both deterministically (the pinned Godot engine inspects
the project) and with an LLM judge.

Inference for both the executor and the judge comes from a single
`opencode serve` process. Nothing here talks to a model provider directly, so
credentials stay in opencode's own configuration.

This directory is a standalone [uv](https://docs.astral.sh/uv/) project. It is
not part of the root Go module.

## Usage

```sh
make evals                                  # full suite, default models
make evals EVALS_REPS=1 EVALS_SHAPES=full   # quick smoke slice
```

Or drive it directly:

```sh
cd evals
uv run evals --list
uv run evals run --reps 3 --model opencode-go/deepseek-v4.1-flash
```

Useful flags:

| Flag | Meaning |
| --- | --- |
| `--tasks` / `--shapes` | Comma-separated allowlists (default: the suite config). |
| `--reps` | Repetitions per task × shape. |
| `--model` / `--judge` | `provider/model` for the executor and judge. |
| `--no-judge` | Deterministic checks only (cheaper, faster). |
| `--dry-run` | Materialize fixtures and print the plan without model calls. |
| `--keep-work` | Keep per-run sandbox directories (under the work root) for inspection. |
| `--out` | Output directory (default `results/<UTC timestamp>`). |

Output is written to the run directory as `runs.jsonl` (one record per run),
`results.json` (everything) and `summary.md` (aggregate tables). A per-shape
summary is also printed to the terminal.

## How a run works

1. One `opencode serve` is started with an isolated `XDG_CONFIG_HOME` so the
   host's global MCP servers do not leak into the eval. The host's global config
   is carried over minus its `mcp` block, so configured providers/models still
   resolve; set `EVALS_OPENCODE_CONFIG` to point at a specific config instead.
   Subagents are disabled (`subagent_depth: 0`) so each trajectory is the one
   model's own.
2. For each (task × shape × repetition):
   - the task fixture is copied to a fresh project directory under the work
     root (a temp directory outside the repository, so the host's `AGENTS.md`
     and git state do not leak into the session);
   - a local MCP server is attached over `POST /mcp`, bound to that project
     (`--project <dir> --idle-timeout 2s`);
   - a session is created and prompted with the shape's tool map in the
     request body, so gating is per-prompt and needs no server restart;
   - all messages, tool calls, tokens and cost are collected;
   - `POST /instance/dispose` closes the MCP process and its Godot daemon.
3. The deterministic verifier runs the pinned Godot binary over a declarative
   check list. The judge (optional) reads the task, the checks, the final
   answer and the session diff, and returns a JSON verdict.

## Layout

```
config/suite.yaml      # default models, reps, shapes, task selection
config/toolsets.yaml   # named toolset shapes
tasks/*.yaml           # task prompt + fixture + checks + rubric
fixtures/<task>/       # minimal Godot project seeding each task
src/godot_mcp_evals/   # harness
tests/                 # unit tests for the pure logic
verify/godot_verify.gd # generic in-engine checker
results/               # run artifacts (gitignored)
```

## Adding a task

Add `tasks/<id>.yaml` and a matching `fixtures/<fixture>/` Godot project:

```yaml
id: set_z_index
description: Raise a sprite above its siblings.
fixture: set_z_index
prompt: |
  In main.tscn, set the z_index of Root/Sprite to 5.
checks:
  - kind: node_property
    scene: res://main.tscn
    path: Sprite
    property: z_index
    value: 5
expect:
  full:
    used_all: [godot_modify_scene_node]
    used_none: [edit, write]
  script_escape:
    used_all: [godot_run_script]
rubric: |
  The Sprite node keeps its type and position; only z_index changes.
```

A fixture normally ships a `project.godot`. Tasks that create the project
themselves (e.g. `create_project`) set `bootstraps_project: true`; their fixture
is then deliberately not a Godot project.

Supported checks: `node_exists`, `node_not_exists`, `node_type`, `node_property`,
`node_script`, `scene_root_type`, `file_exists`, `file_contains`,
`script_has_method`, `project_setting`, `file_unchanged`, and `answer_contains`
(the last two are evaluated in Python; `file_unchanged` compares the graded
project against the fixture). A `node_property` `property` may be dotted to
reach nested objects, e.g. `shape.size` on a `CollisionShape2D`.

### Partial credit for composite tasks

Tasks whose `min_score` is set are graded on a weighted fraction of their
checks rather than all-or-nothing, which suits multi-step "composite" tasks
where several valid trajectories exist:

```yaml
min_score: 0.85
checks:
  - kind: node_exists
    scene: res://level.tscn
    path: Hero
    required: true
  - kind: node_property
    scene: res://level.tscn
    path: Hero/Camera
    property: name
    value: Camera
    weight: 2
```

- `min_score` (0–1) is the weighted fraction a run must earn to pass. When
  omitted, every check must pass (the binary default).
- `weight` (default 1) is how much a check contributes to the score.
- `required: true` gates the verdict: the check must pass no matter how high
  the score is. Use it for load-bearing checks (the scene exists, the root
  type) so a run cannot pass by skipping the point of the task.

Trajectory expectations (`used_all`/`used_none`/`used_any`) are always
*required*: partial credit is for the artifact, never for tool choice, since
tool choice is the signal the suite exists to measure. The report shows the
mean weighted score per shape and task alongside pass@1.

### Trajectory expectations

`expect` maps a shape name to tool-use assertions, graded alongside the artifact
checks and part of pass/fail:

- `used_all` — every listed tool must appear in the trajectory.
- `used_any` — at least one of the listed tools must appear.
- `used_none` — none of the listed tools may appear (e.g. `edit`/`write`, to
  catch a model that hand-edits `.tscn` text instead of using the structured
  tool).

Tool names are the fully-qualified opencode names: MCP tools carry the server
prefix (`godot_add_node`, `godot_run_script`), built-ins do not (`edit`,
`write`). A required tool that the shape does not expose is a config error and
fails the run loudly, so expectations cannot silently become unsatisfiable.

## Adding a toolset shape

Edit `config/toolsets.yaml`. `mcp` accepts `all`, `none`, a list of tool names,
or `all-except:<name,name>`; `builtin` is a list of opencode built-in tools.
The harness always sends a complete boolean map, so unspecified tools are off.
A shape may also set `reps` to override the suite default (e.g. `read_only: 1`).
A shape's `reps` takes precedence over `--reps`; `--reps` only overrides shapes
that do not set their own.

## Interpreting results

The headline number is deterministic pass@1 per shape. Alongside it the report
tracks mean judge score, MCP vs built-in tool calls, turns, input/output/
reasoning/cache tokens, cost and wall time. Pass rates include a Wilson 95%
interval; treat overlapping intervals across shapes as inconclusive at low reps.
