#!/usr/bin/env -S godot --headless --script
# Derived from tugcantopaloglu/godot-mcp (src/scripts/validate_script.gd), a
# fork of Coding-Solo/godot-mcp (both MIT-licensed); reworked to check every
# target in one engine process, bracketing each with a marker the caller uses to
# attribute diagnostics to the script being loaded.
extends SceneTree
## _initialize() runs after autoloads load, so autoload references resolve; --check-only would misreport them.

const MARKER := "<<<GODOT_MCP_VALIDATE:"

func _initialize():
    var args = OS.get_cmdline_args()
    var script_index = args.find("--script")
    var first_target = script_index + 2

    if script_index == -1 or args.size() <= first_target:
        printerr("SCRIPT ERROR: usage: godot --headless --path <project> --script validate_script.gd <res://path/to/script.gd>...")
        quit(1)
        return

    for index in range(first_target, args.size()):
        var target = args[index]
        printerr(MARKER + target + ">>>")
        # CACHE_MODE_IGNORE forces a fresh parse instead of reusing a cached entry for this path.
        ResourceLoader.load(target, "GDScript", ResourceLoader.CACHE_MODE_IGNORE)

    quit()
