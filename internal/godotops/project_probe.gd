#!/usr/bin/env -S godot --headless --script
# Boots the engine against a project and exits without running a main scene.
# ProjectSettings parses project.godot during engine startup, before _init runs,
# so this script's only job is to let that parse happen and then quit. The
# caller reads the engine's stderr and exit code; the script produces no output.
extends SceneTree


func _init():
    quit(0)
