#!/usr/bin/env -S godot --headless --script
# Copied verbatim from tugcantopaloglu/godot-mcp (src/scripts/validate_script.gd),
# a fork of Coding-Solo/godot-mcp (both MIT-licensed); only indentation was
# changed (tabs to spaces).
extends SceneTree
## _initialize() runs after autoloads load, so autoload references resolve; --check-only would misreport them.

func _initialize():
    var args = OS.get_cmdline_args()
    var script_index = args.find("--script")
    var target_index = script_index + 2

    if script_index == -1 or args.size() <= target_index:
        printerr("SCRIPT ERROR: usage: godot --headless --path <project> --script validate_script.gd <res://path/to/script.gd>")
        quit(1)
        return

    var target = args[target_index]

    # CACHE_MODE_IGNORE forces a fresh parse instead of reusing a cached entry for this path.
    ResourceLoader.load(target, "GDScript", ResourceLoader.CACHE_MODE_IGNORE)

    quit()
