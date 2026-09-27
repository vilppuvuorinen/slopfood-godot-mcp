#!/usr/bin/env -S godot --headless --script
# Adapted from tugcantopaloglu/godot-mcp (src/scripts/godot_operations.gd), a
# fork of Coding-Solo/godot-mcp (both MIT-licensed); most of this file is
# copied verbatim from those projects.
extends SceneTree

const JSON_START = "<<<GODOT_MCP_JSON_START>>>"
const JSON_END = "<<<GODOT_MCP_JSON_END>>>"

# Bridge contract
#
# Every handler has the signature `func <name>(result: Result, params) -> Result`
# and reports exclusively through the Result it is handed:
#
#   - Progress goes to stdout: log_info(result, ...) buffers a line that _finish
#     prints to standard output. A structured payload is JSON between
#     JSON_START and JSON_END, which the Go side extracts with markerJSON; an
#     operation without one reports status text instead.
#   - Diagnostics go to stderr: log_error(result, ...) buffers a line that
#     _finish prints to standard error. The Go side fails the operation on any
#     stderr line, so nothing benign is written there.
#   - A handler returns the same Result on success, or result.fail(...) (which
#     flips ok to false and buffers the reason) on failure. Helpers that report
#     failure return null/false for their caller to propagate.
#
# A handler never calls print()/printerr() and never calls quit(): _init is the
# only place that flushes the buffers and sets the exit code, through _finish.
# This keeps a failing handler from leaving a partial payload on stdout and
# keeps the exit status decided once.

# Result carries a handler's outcome back to the dispatcher. ok is the exit
# status; stdout and stderr are buffers the dispatcher flushes to the matching
# standard streams. Handlers write through out()/err() instead of printing, so
# a failure never leaves half a payload on stdout and the exit code is decided
# in one place.
class Result extends RefCounted:
    var ok := true
    var stdout := ""
    var stderr := ""

    func out(message: String) -> void:
        stdout += message + "\n"

    func err(message: String) -> void:
        stderr += message + "\n"

    # Mark the operation failed and return self, so a handler can write
    # `return result.fail("reason")` from anywhere.
    func fail(message: String = "") -> Result:
        if not message.is_empty():
            err(message)
        ok = false
        return self

# DecodeResult carries the outcome of turning a caller-supplied value into the
# Variant a property expects. ok is false when the value cannot be decoded,
# error is the reason, and value is the decoded Variant (or a neutral fallback
# such as a zero vector, so a composite constructor never receives null). The
# result travels with the value, so a failure can never be mistaken for a stale
# one from an earlier decode.
class DecodeResult extends RefCounted:
    var ok := true
    var error := ""
    var value = null

    static func success(decoded) -> DecodeResult:
        var result := DecodeResult.new()
        result.value = decoded
        return result

    static func failure(message: String, fallback = null) -> DecodeResult:
        var result := DecodeResult.new()
        result.ok = false
        result.error = message
        result.value = fallback
        return result

func _init():
    # The active result is allocated here and threaded through every handler
    # and helper that emits output or reports failure. _finish flushes it last.
    var result := Result.new()
    var args = OS.get_cmdline_args()

    # Find the script argument and determine the positions of operation and params
    var script_index = args.find("--script")
    if script_index == -1:
        log_error(result, "Could not find --script argument")
        _finish(result)
        return

    # The operation should be 2 positions after the script path (script_index + 1 is the script path itself)
    var operation_index = script_index + 2
    # The params should be 3 positions after the script path
    var params_index = script_index + 3

    if args.size() <= params_index:
        log_error(result, "Usage: godot --headless --script godot_operations.gd <operation> <json_params>")
        log_error(result, "Not enough command-line arguments provided.")
        _finish(result)
        return

    var operation = args[operation_index]
    var params_json = args[params_index]

    # Parse JSON using Godot 4.x API
    var json = JSON.new()
    var error = json.parse(params_json)
    var params = null

    if error == OK:
        params = json.get_data()
    else:
        log_error(result, "Failed to parse JSON parameters: " + params_json)
        log_error(result, "JSON Error: " + json.get_error_message() + " at line " + str(json.get_error_line()))
        _finish(result)
        return

    if params == null:
        log_error(result, "Failed to parse JSON parameters: " + params_json)
        _finish(result)
        return

    result = dispatch(operation, params)
    _finish(result)

# Turn an operation name and its already-decoded params into a Result by
# invoking the matching handler. _init calls it once, after argv and JSON
# parsing, so handler invocation no longer depends on either: a daemon driver
# can call it per request without the process exiting between them.
func dispatch(operation, params) -> Result:
    var result := Result.new()
    _temp_nodes.clear()

    log_info(result, "Operation: " + operation)
    log_info(result, "Executing operation: " + operation)

    match operation:
        "create_scene":
            result = create_scene(result, params)
        "add_node":
            result = add_node(result, params)
        "export_mesh_library":
            result = export_mesh_library(result, params)
        "read_scene":
            result = read_scene(result, params)
        "modify_node":
            result = modify_node(result, params)
        "remove_node":
            result = remove_node(result, params)
        "manage_scene_structure":
            result = manage_scene_structure(result, params)
        "manage_scene_signals":
            result = manage_scene_signals(result, params)
        "attach_script":
            result = attach_script(result, params)
        "manage_resource":
            result = manage_resource(result, params)
        "manage_animation":
            result = manage_animation(result, params)
        "manage_tileset":
            result = manage_tileset(result, params)
        "manage_theme":
            result = manage_theme(result, params)
        "manage_tilemap":
            result = manage_tilemap(result, params)
        "validate_project":
            result = validate_project(result, params)
        _:
            log_error(result, "Unknown operation: " + operation)
            result.ok = false

    _free_temp_nodes()
    return result

# Print the buffers a handler produced to their standard streams and end the
# main loop with the status it reported. SceneTree.quit() does not stop the
# current call, so quit() is the last thing _init does.
func _finish(result: Result):
    var stdout := result.stdout
    if not stdout.is_empty():
        print(stdout.substr(0, stdout.length() - 1))
    var stderr := result.stderr
    if not stderr.is_empty():
        printerr(stderr.substr(0, stderr.length() - 1))
    quit(0 if result.ok else 1)

# Temporary nodes instantiated by handlers are tracked in _temp_nodes. dispatch
# owns this list: it clears it before invoking a handler and frees every entry
# after the handler returns, success or failure. The nodes are never added to
# the scene tree, so without this the engine reports ObjectDB/RID leaks at exit.
# Cleanup is deterministic and does not depend on the SceneTree ticking
# _process, which a daemon loop blocked on stdin would never do.
var _temp_nodes = []

func track_temp(node):
    _temp_nodes.append(node)
    return node

# Free every node tracked since the current dispatch began and clear the list.
# is_instance_valid guards against a handler that already freed a node itself.
func _free_temp_nodes():
    for node in _temp_nodes:
        if is_instance_valid(node):
            node.free()
    _temp_nodes.clear()

# Logging functions
func log_info(result: Result, message):
    result.out("[INFO] " + message)

func log_error(result: Result, message):
    result.err("[ERROR] " + message)

# Reduce a Godot virtual path ("res://...") to a project-relative path, so
# path-valued result fields share the project-relative realm of tool inputs
# instead of leaking a form the caller must translate. Paths outside res://
# (for example user://) are returned unchanged.
func project_relative(path):
    if path.begins_with("res://"):
        return path.substr(6)
    return path

# Get a script by registered class name.
# Only looks up names via the project's global class registry. Raw paths
# (e.g. "res://evil.gd") are intentionally not accepted here to prevent
# arbitrary script instantiation from agent-supplied input.
func get_script_by_name(result: Result, name_of_class):
    # Search for it in the global class registry if it's a class name
    var global_classes = ProjectSettings.get_global_class_list()
    for global_class in global_classes:
        var found_name_of_class = global_class["class"]
        var found_path = global_class["path"]
        
        if found_name_of_class == name_of_class:
            var script = load(found_path) as Script
            if script:
                return script
            else:
                result.err("Failed to load script from registry path: " + found_path)
                break
    
    result.err("Could not find script for class: " + name_of_class)
    return null

# Instantiate a class by name
func instantiate_class(result: Result, name_of_class):
    if name_of_class.is_empty():
        result.err("Cannot instantiate class: name is empty")
        return null
    
    var instance = null
    # Check if it's a built-in class
    if ClassDB.class_exists(name_of_class):
        if ClassDB.can_instantiate(name_of_class):
            instance = ClassDB.instantiate(name_of_class)
            if instance == null:
                result.err("ClassDB.instantiate() returned null for class: " + name_of_class)
        else:
            result.err("Class exists but cannot be instantiated: " + name_of_class)
            result.err("This may be an abstract class or interface that cannot be directly instantiated")
    else:
        # Try to get the script
        var script = get_script_by_name(result, name_of_class)
        if script is GDScript:
            instance = script.new()
        else:
            result.err("Failed to get script for class: " + name_of_class)
            return null
    
    if instance == null:
        result.err("Failed to instantiate class: " + name_of_class)
    return instance

# Ensure the directory holding a target scene file exists, creating it
# recursively. Returns false (after buffering the reason) when it cannot be
# created.
func _ensure_scene_directory(result: Result, full_scene_path):
    var scene_dir_res = full_scene_path.get_base_dir()
    var scene_dir_abs = ProjectSettings.globalize_path(scene_dir_res)
    var scene_dir_relative = scene_dir_res.substr(6)  # Remove "res://" prefix
    if scene_dir_relative.is_empty():
        return true
    if DirAccess.dir_exists_absolute(scene_dir_abs):
        return true
    var dir = DirAccess.open("res://")
    if dir == null:
        var open_error = DirAccess.get_open_error()
        result.err("Failed to open res:// directory: " + str(open_error))
        var make_dir_error = DirAccess.make_dir_recursive_absolute(scene_dir_abs)
        if make_dir_error != OK:
            result.err("Failed to create directory using absolute path")
            result.err("Error code: " + str(make_dir_error))
            return false
    else:
        var make_dir_error = dir.make_dir_recursive(scene_dir_relative)
        if make_dir_error != OK:
            result.err("Failed to create directory: " + scene_dir_relative)
            result.err("Error code: " + str(make_dir_error))
            return false
    if not DirAccess.dir_exists_absolute(scene_dir_abs):
        result.err("Directory reported as created but does not exist: " + scene_dir_abs)
        result.err("This may indicate a problem with path resolution or permissions")
        return false
    return true

# Create an inherited scene: a scene whose root node instances another
# PackedScene. The engine cannot author one at runtime (PackedScene.pack
# flattens instances), so the minimal file is written directly. base_scene_path
# may be project-relative or a res:// path.
func _create_inherited_scene(result: Result, full_scene_path, base_scene_path):
    var full_base_path = base_scene_path
    if not full_base_path.begins_with("res://"):
        full_base_path = "res://" + full_base_path
    if "\"" in full_base_path or "\n" in full_base_path:
        result.err("Invalid base scene path: " + base_scene_path)
        result.ok = false
        return
    if not FileAccess.file_exists(full_base_path):
        result.err("Base scene does not exist at: " + full_base_path)
        result.ok = false
        return

    var content = "[gd_scene load_steps=2 format=3]\n\n"
    content += "[ext_resource type=\"PackedScene\" path=\"" + full_base_path + "\" id=\"1_base\"]\n\n"
    content += "[node name=\"root\" instance=ExtResource(\"1_base\")]\n"

    var file = FileAccess.open(full_scene_path, FileAccess.WRITE)
    if file == null:
        result.err("Failed to create scene: " + full_scene_path)
        result.ok = false
        return
    file.store_string(content)
    file.close()
    if not FileAccess.file_exists(full_scene_path):
        result.err("Failed to create scene: " + full_scene_path)
        result.ok = false
        return
    result.out("Scene created successfully at: " + project_relative(full_scene_path))

# Create a new scene with a specified root node type, or an inherited scene when
# baseScenePath names another scene.
func create_scene(result: Result, params) -> Result:
    result.out("Creating scene: " + params.scenePath)
    
    # Normalize the scene path
    var full_scene_path = params.scenePath
    if not full_scene_path.begins_with("res://"):
        full_scene_path = "res://" + full_scene_path
    if not _ensure_scene_directory(result, full_scene_path):
        return result.fail()

    # An inherited scene's root is an instance of the base scene rather than a
    # fresh node, so baseScenePath replaces rootNodeType entirely.
    var base_scene_path = str(params.get("baseScenePath", ""))
    if not base_scene_path.is_empty():
        _create_inherited_scene(result, full_scene_path, base_scene_path)
        return result

    # Use traditional if-else statement for better compatibility
    var root_node_type = "Node2D"  # Default value
    if params.has("rootNodeType"):
        root_node_type = params.rootNodeType
    # Create the root node
    var scene_root = track_temp(instantiate_class(result, root_node_type))
    if not scene_root:
        result.err("Failed to instantiate node of type: " + root_node_type)
        result.err("Make sure the class exists and can be instantiated")
        result.err("Check if the class is registered in ClassDB or available as a script")
        return result.fail()
    
    scene_root.name = "root"
    # Pack the scene
    var packed_scene = PackedScene.new()
    var pack_error = packed_scene.pack(scene_root)
    if pack_error == OK:
        # Save the scene
        var save_error = ResourceSaver.save(packed_scene, full_scene_path)
        if save_error == OK:
            # Check the scene file was actually written
            var file_exists = FileAccess.file_exists(full_scene_path)
            if file_exists:
                result.out("Scene created successfully at: " + params.scenePath)
            else:
                result.err("Failed to create scene: " + params.scenePath)
                return result.fail()
        else:
            # Handle specific error codes
            var error_message = "Failed to save scene. Error code: " + str(save_error)
            
            if save_error == ERR_CANT_CREATE:
                error_message += " (ERR_CANT_CREATE - Cannot create the scene file)"
            elif save_error == ERR_CANT_OPEN:
                error_message += " (ERR_CANT_OPEN - Cannot open the scene file for writing)"
            elif save_error == ERR_FILE_CANT_WRITE:
                error_message += " (ERR_FILE_CANT_WRITE - Cannot write to the scene file)"
            elif save_error == ERR_FILE_NO_PERMISSION:
                error_message += " (ERR_FILE_NO_PERMISSION - No permission to write the scene file)"
            
            result.err(error_message)
            return result.fail()
    else:
        result.err("Failed to pack scene: " + str(pack_error))
        result.err("Error code: " + str(pack_error))
        return result.fail()
    return result

# Add a node to an existing scene
func add_node(result: Result, params) -> Result:
    result.out("Adding node to scene: " + params.scenePath)
    
    var full_scene_path = params.scenePath
    if not full_scene_path.begins_with("res://"):
        full_scene_path = "res://" + full_scene_path
    var absolute_scene_path = ProjectSettings.globalize_path(full_scene_path)
    if not FileAccess.file_exists(absolute_scene_path):
        result.err("Scene file does not exist at: " + absolute_scene_path)
        return result.fail()
    if _reject_inherited_scene(result, full_scene_path):
        return result.fail()
    
    var scene = load(full_scene_path)
    if not scene:
        result.err("Failed to load scene: " + full_scene_path)
        return result.fail()
    
    var scene_root = track_temp(scene.instantiate())
    # Use traditional if-else statement for better compatibility
    var parent_path = "root"  # Default value
    if params.has("parentNodePath"):
        parent_path = params.parentNodePath
    var parent = scene_root
    if parent_path != "root":
        parent = scene_root.get_node(parent_path.replace("root/", ""))
        if not parent:
            result.err("Parent node not found: " + parent_path)
            return result.fail()
    var instance_path = str(params.get("instancePath", ""))
    var new_node = null
    if not instance_path.is_empty():
        # Add an instance of another scene rather than a fresh node. PackedScene
        # .instantiate() stamps the root's scene_file_path, so packing the parent
        # records the node as an ExtResource-backed instance instead of
        # flattening the subtree into the parent scene.
        var full_instance_path = instance_path
        if not full_instance_path.begins_with("res://"):
            full_instance_path = "res://" + full_instance_path
        if not FileAccess.file_exists(full_instance_path):
            result.err("Instance scene does not exist at: " + full_instance_path)
            return result.fail()
        var packed_instance = load(full_instance_path)
        if not (packed_instance is PackedScene):
            result.err("Not a scene file: " + full_instance_path)
            return result.fail()
        new_node = track_temp(packed_instance.instantiate())
        if not new_node:
            result.err("Failed to instantiate scene: " + full_instance_path)
            return result.fail()
    else:
        new_node = track_temp(instantiate_class(result, params.nodeType))
        if not new_node:
            result.err("Failed to instantiate node of type: " + params.nodeType)
            result.err("Make sure the class exists and can be instantiated")
            result.err("Check if the class is registered in ClassDB or available as a script")
            return result.fail()
    new_node.name = params.nodeName
    if params.has("properties"):
        var applied = _apply_properties(new_node, params.properties)
        if not applied.ok:
            result.err("Invalid property value: " + applied.error)
            return result.fail()

    parent.add_child(new_node)
    new_node.owner = scene_root
    var packed_scene = PackedScene.new()
    var pack_error = packed_scene.pack(scene_root)
    if pack_error == OK:
        var save_error = ResourceSaver.save(packed_scene, absolute_scene_path)
        if save_error == OK:
            if instance_path.is_empty():
                result.out("Node '" + params.nodeName + "' of type '" + params.nodeType + "' added successfully")
            else:
                result.out("Scene '" + instance_path + "' instanced as '" + params.nodeName + "' added successfully")
        else:
            result.err("Failed to save scene: " + str(save_error))
            return result.fail()
    else:
        result.err("Failed to pack scene: " + str(pack_error))
        return result.fail()
    return result

# Export a scene as a MeshLibrary resource
func export_mesh_library(result: Result, params) -> Result:
    result.out("Exporting MeshLibrary from scene: " + params.scenePath)
    
    # Ensure the scene path starts with res:// for Godot's resource system
    var full_scene_path = params.scenePath
    if not full_scene_path.begins_with("res://"):
        full_scene_path = "res://" + full_scene_path
    
    # Project-relative output paths resolve under res://. Absolute host paths
    # (and user://) are kept as-is so the caller can target a location outside
    # the project; prefixing those with res:// would produce an invalid path.
    var full_output_path = params.outputPath
    if not full_output_path.begins_with("res://") and not full_output_path.begins_with("user://") and not full_output_path.is_absolute_path():
        full_output_path = "res://" + full_output_path
    
    # Check if the scene file exists
    var file_check = FileAccess.file_exists(full_scene_path)
    if not file_check:
        result.err("Scene file does not exist at: " + full_scene_path)
        # Get the absolute path for reference
        var absolute_path = ProjectSettings.globalize_path(full_scene_path)
        result.err("Absolute file path that doesn't exist: " + absolute_path)
        return result.fail()
    
    # Load the scene
    var scene = load(full_scene_path)
    if not scene:
        result.err("Failed to load scene: " + full_scene_path)
        return result.fail()
    
    # Instance the scene
    var scene_root = track_temp(scene.instantiate())
    # Create a new MeshLibrary
    var mesh_library = MeshLibrary.new()
    # Get mesh item names if provided
    var mesh_item_names = params.meshItemNames if params.has("meshItemNames") else []
    var use_specific_items = mesh_item_names.size() > 0
    
    # Process all child nodes
    var item_id = 0
    for child in scene_root.get_children():
        # Skip if not using all items and this item is not in the list
        if use_specific_items and not (child.name in mesh_item_names):
            continue
            
        # Check if the child has a mesh
        var mesh_instance = null
        if child is MeshInstance3D:
            mesh_instance = child
        else:
            # Try to find a MeshInstance3D in the child's descendants
            for descendant in child.get_children():
                if descendant is MeshInstance3D:
                    mesh_instance = descendant
                    break
        
        if mesh_instance and mesh_instance.mesh:
            # Add the mesh to the library
            mesh_library.create_item(item_id)
            mesh_library.set_item_name(item_id, child.name)
            mesh_library.set_item_mesh(item_id, mesh_instance.mesh)
            # Add collision shape if available
            for collision_child in child.get_children():
                if collision_child is CollisionShape3D and collision_child.shape:
                    mesh_library.set_item_shapes(item_id, [collision_child.shape])
                    break
            
            # MeshLibrary previews must be textures. The mesh's albedo texture is
            # the only texture available at runtime, so use it when present.
            var preview_texture = _mesh_preview_texture(mesh_instance.mesh)
            if preview_texture:
                mesh_library.set_item_preview(item_id, preview_texture)
            item_id += 1
    # Create directory if it doesn't exist
    var output_dir = full_output_path.get_base_dir()
    if full_output_path.begins_with("res://"):
        var dir = DirAccess.open("res://")
        if dir == null:
            result.err("Failed to open res:// directory")
            result.err("DirAccess error: " + str(DirAccess.get_open_error()))
            return result.fail()
        if output_dir != "res://" and not dir.dir_exists(output_dir.substr(6)):  # Remove "res://" prefix
            var make_error = dir.make_dir_recursive(output_dir.substr(6))  # Remove "res://" prefix
            if make_error != OK:
                result.err("Failed to create directory: " + output_dir + ", error: " + str(make_error))
                return result.fail()
    elif not DirAccess.dir_exists_absolute(output_dir):
        var make_error = DirAccess.make_dir_recursive_absolute(output_dir)
        if make_error != OK:
            result.err("Failed to create directory: " + output_dir + ", error: " + str(make_error))
            return result.fail()
    
    # Save the mesh library
    if item_id > 0:
        var error = ResourceSaver.save(mesh_library, full_output_path)
        if error == OK:
            # Verify the file was actually created
            result.out("MeshLibrary exported successfully with " + str(item_id) + " items to: " + full_output_path)
        else:
            result.err("Failed to save MeshLibrary: " + str(error))
            return result.fail()
    else:
        result.err("No valid meshes found in the scene")
        return result.fail()
    return result

# Pick a Texture2D to use as a MeshLibrary item preview. MeshLibrary previews
# require a Texture2D, but meshes are not textures, so fall back to the first
# albedo texture found on the mesh surfaces and return null when there is none.
func _mesh_preview_texture(mesh) -> Texture2D:
    if not mesh:
        return null
    for surface_index in range(mesh.get_surface_count()):
        var material = mesh.surface_get_material(surface_index)
        if material is BaseMaterial3D and material.albedo_texture:
            return material.albedo_texture
    return null

# Read a scene file as a JSON node tree
func read_scene(result: Result, params) -> Result:
    if not params.has("scenePath"):
        result.err("scenePath is required")
        return result.fail()

    var full_scene_path = params.scenePath
    if not full_scene_path.begins_with("res://"):
        full_scene_path = "res://" + full_scene_path

    log_info(result, "Reading scene: " + full_scene_path)

    if not FileAccess.file_exists(full_scene_path):
        result.err("Scene file does not exist at: " + full_scene_path)
        return result.fail()

    # load() succeeds for any resource, so scenePath can point at a non-scene
    # such as a .tres. That has no instantiate() and would otherwise die as a
    # SCRIPT ERROR, so a non-PackedScene is a controlled failure rather than a
    # degraded raw-text success; read_binary_resource inspects those resources. A null
    # load is a parse/IO failure whose engine errors already reach stderr, so no
    # extra stderr line here: it would only duplicate them.
    var scene = load(full_scene_path)
    if scene == null:
        return result.fail()
    if not (scene is PackedScene):
        result.err("Not a scene file: " + full_scene_path + " (use read_binary_resource for binary resources)")
        return result.fail()

    var scene_root = track_temp(scene.instantiate())
    if scene_root == null:
        result.err("Failed to instantiate scene: " + full_scene_path)
        return result.fail()

    # An optional nodePath roots the returned tree at a descendant instead of
    # the scene root, so a caller can page into a large scene without pulling
    # the whole tree into its context.
    var start_node = scene_root
    if params.has("nodePath") and params.nodePath != null and not str(params.nodePath).is_empty():
        var node_path = str(params.nodePath)
        if node_path.begins_with("root/"):
            node_path = node_path.substr(5)
        if node_path != "root" and node_path != "." and not node_path.is_empty():
            start_node = scene_root.get_node_or_null(node_path)
            if start_node == null:
                result.err("Node not found: " + str(params.nodePath))
                return result.fail()

    # An optional depth limits how many child levels are included below the
    # start node; -1 (the default) means no limit and 0 returns the node alone.
    var depth = -1
    if params.has("depth") and params.depth != null:
        depth = int(params.depth)

    var tree_data = _walk_scene_tree(start_node, depth)

    # An inherited scene's root instances another PackedScene. Instantiating it
    # resolves every node's scene_file_path, so the in-memory tree alone cannot
    # tell an inherited scene from a plain one; the raw file is the only signal.
    # Report it so a caller does not mistake the copied base subtree for local
    # nodes.
    var base_scene_path = _inherited_scene_base(full_scene_path)
    if not base_scene_path.is_empty():
        tree_data["baseScenePath"] = base_scene_path

    # Surface the scene's own [connection] lines alongside the tree so a caller
    # does not need a separate read just to list signal connections. Inherited
    # connections live in the base scene and are not repeated here.
    tree_data["connections"] = _scene_connections(FileAccess.get_file_as_string(full_scene_path))

    result.out(JSON_START)
    result.out(JSON.stringify(tree_data))
    result.out(JSON_END)

    scene_root.queue_free()
    return result

# Return the project-relative path of the PackedScene a scene file inherits
# from, or "" when the file is not an inherited scene. The scene root is the
# first [node] line and, unlike a child, carries no parent attribute; when it
# has an instance=ExtResource("id") reference, that id resolves through the
# file's [ext_resource] entries.
func _inherited_scene_base(full_scene_path):
    var content = FileAccess.get_file_as_string(full_scene_path)
    if content.is_empty():
        return ""
    var ext_paths = {}
    var root_instance = ""
    for raw_line in content.split("\n"):
        var line = raw_line.strip_edges()
        if line.begins_with("[ext_resource"):
            var id = _tscn_attr(line, "id")
            var path = _tscn_attr(line, "path")
            if not id.is_empty() and not path.is_empty():
                ext_paths[id] = path
        elif line.begins_with("[node"):
            if not (" parent=" in line):
                var marker = "instance=ExtResource("
                var at = line.find(marker)
                if at != -1:
                    var rest = line.substr(at + marker.length())
                    if rest.begins_with("\""):
                        var end = rest.find("\"", 1)
                        if end != -1:
                            root_instance = rest.substr(1, end - 1)
            break
    if root_instance.is_empty():
        return ""
    return project_relative(str(ext_paths.get(root_instance, "")))

# Read a name="value" attribute from a .tscn block header. The name must start
# its own attribute (preceded by a space), so an id lookup does not match inside
# uid=.
func _tscn_attr(line, name):
    var marker = name + "=\""
    var start = 0
    while true:
        var at = line.find(marker, start)
        if at == -1:
            return ""
        if at > 0 and line[at - 1] == " ":
            var value_start = at + marker.length()
            var end = line.find("\"", value_start)
            if end == -1:
                return ""
            return line.substr(value_start, end - value_start)
        start = at + 1

# Refuse a structural edit that would flatten an inherited scene. The mutators
# below load, instantiate, and repack with PackedScene.pack(), which cannot
# re-serialize an inherited root as an instance: it would replace the base
# reference with a full copy of the base subtree. Returns true when the caller
# must stop.
func _reject_inherited_scene(result: Result, full_scene_path):
    var base_scene_path = _inherited_scene_base(full_scene_path)
    if base_scene_path.is_empty():
        return false
    result.err("Cannot edit inherited scene: " + full_scene_path)
    result.err("Its root instances " + base_scene_path + "; editing it would flatten the base scene into this file")
    result.err("Edit the base scene instead, or recreate this scene with create_scene")
    return true

# Walk a scene tree and build a dictionary describing each node. depth is the
# number of child levels still to include below node: -1 means no limit, 0
# returns the node without its children.
func _walk_scene_tree(node, depth := -1) -> Dictionary:
    var info = {
        "name": node.name,
        "type": node.get_class(),
    }

    var node_script = node.get_script()
    if node_script != null and node_script is Script:
        info["script"] = project_relative(node_script.resource_path)

    var props = {}
    for prop in node.get_property_list():
        var prop_name = prop["name"]
        var usage = prop.get("usage", 0)
        if usage & PROPERTY_USAGE_EDITOR and usage & PROPERTY_USAGE_STORAGE:
            var value = node.get(prop_name)
            if value != null:
                props[prop_name] = _variant_to_json(value)

    if props.size() > 0:
        info["properties"] = props

    var groups = node.get_groups()
    if groups.size() > 0:
        var group_names = []
        for g in groups:
            group_names.append(str(g))
        info["groups"] = group_names

    if depth != 0:
        var child_depth = -1 if depth < 0 else depth - 1
        var children_arr = []
        for child in node.get_children():
            children_arr.append(_walk_scene_tree(child, child_depth))

        if children_arr.size() > 0:
            info["children"] = children_arr

    return info

# The tool value contract. A Variant is encoded as JSON-native data where one
# exists, and as a tagged object {"$type": "...", ...} for engine-only types.
# _decode_value inverts this exactly, so a value read from a scene or resource
# can be written back without reinterpretation. Types with no structured
# mapping are wrapped in the single GodotText escape hatch rather than being
# handed back as a bare engine string.
func _variant_to_json(value):
    match typeof(value):
        TYPE_NIL:
            return null
        TYPE_BOOL, TYPE_INT, TYPE_STRING:
            return value
        TYPE_FLOAT:
            # JSON has no NaN/Infinity, so those use the escape hatch.
            if not is_finite(value):
                return _godot_text(value)
            return value
        TYPE_STRING_NAME:
            return {"$type": "StringName", "value": String(value)}
        TYPE_VECTOR2:
            return {"$type": "Vector2", "x": value.x, "y": value.y}
        TYPE_VECTOR2I:
            return {"$type": "Vector2i", "x": value.x, "y": value.y}
        TYPE_VECTOR3:
            return {"$type": "Vector3", "x": value.x, "y": value.y, "z": value.z}
        TYPE_VECTOR3I:
            return {"$type": "Vector3i", "x": value.x, "y": value.y, "z": value.z}
        TYPE_VECTOR4:
            return {"$type": "Vector4", "x": value.x, "y": value.y, "z": value.z, "w": value.w}
        TYPE_VECTOR4I:
            return {"$type": "Vector4i", "x": value.x, "y": value.y, "z": value.z, "w": value.w}
        TYPE_COLOR:
            return {"$type": "Color", "r": value.r, "g": value.g, "b": value.b, "a": value.a}
        TYPE_RECT2:
            return {"$type": "Rect2", "position": _variant_to_json(value.position), "size": _variant_to_json(value.size)}
        TYPE_RECT2I:
            return {"$type": "Rect2i", "position": _variant_to_json(value.position), "size": _variant_to_json(value.size)}
        TYPE_TRANSFORM2D:
            return {
                "$type": "Transform2D",
                "x": _variant_to_json(value.x),
                "y": _variant_to_json(value.y),
                "origin": _variant_to_json(value.origin),
            }
        TYPE_QUATERNION:
            return {"$type": "Quaternion", "x": value.x, "y": value.y, "z": value.z, "w": value.w}
        TYPE_BASIS:
            return {
                "$type": "Basis",
                "x": _variant_to_json(value.x),
                "y": _variant_to_json(value.y),
                "z": _variant_to_json(value.z),
            }
        TYPE_TRANSFORM3D:
            return {"$type": "Transform3D", "basis": _variant_to_json(value.basis), "origin": _variant_to_json(value.origin)}
        TYPE_AABB:
            return {"$type": "AABB", "position": _variant_to_json(value.position), "size": _variant_to_json(value.size)}
        TYPE_PLANE:
            return {"$type": "Plane", "normal": _variant_to_json(value.normal), "d": value.d}
        TYPE_NODE_PATH:
            return {"$type": "NodePath", "path": String(value)}
        TYPE_PACKED_STRING_ARRAY:
            return {"$type": "PackedStringArray", "values": Array(value)}
        TYPE_ARRAY:
            var items = []
            for item in value:
                items.append(_variant_to_json(item))
            return items
        TYPE_DICTIONARY:
            var result = {}
            for key in value:
                result[str(key)] = _variant_to_json(value[key])
            return result
        TYPE_OBJECT:
            if value == null:
                return null
            if value is Resource and value.resource_path != "":
                return {"$type": "Resource", "path": value.resource_path}
            return _godot_text(value)
        _:
            return _godot_text(value)

# _godot_text is the single, named escape hatch for an engine value with no
# structured mapping (an InputEvent, a callable, a non-finite float, ...). It
# carries var_to_str output, the engine's own Variant serialization, so it is
# both complete and reparsable rather than an opaque <Object#id> address.
func _godot_text(value):
    return {"$type": "GodotText", "text": var_to_str(value)}

# Set every property, decoding each one first. Returns a DecodeResult carrying
# the failure reason when a value cannot be decoded, so the caller aborts before
# writing anything.
func _apply_properties(target, properties) -> DecodeResult:
    for prop_name in properties:
        var decoded = _convert_property_value(target, prop_name, properties[prop_name])
        if not decoded.ok:
            return decoded
        target.set(prop_name, decoded.value)
    return DecodeResult.success(null)

# Convert a caller-supplied value into the type a target property expects. The
# value must already be in the tool value contract, with a $type tag for any
# engine-only type; a bare JSON object is a plain Dictionary, never a Vector2.
func _convert_property_value(target, prop_name, value) -> DecodeResult:
    for prop in target.get_property_list():
        if prop["name"] == prop_name:
            return _convert_value_for_type(prop.get("type", 0), value)
    return _decode_value(value)

# Convert a value to the Godot type identified by a Variant type id. Native
# JSON scalars are coerced to the property's declared type; engine-only types
# must arrive as a matching $type object.
func _convert_value_for_type(type_id, value) -> DecodeResult:
    if value is Dictionary and value.has("$type"):
        return _decode_typed(value)
    match type_id:
        TYPE_VECTOR2, TYPE_VECTOR2I, TYPE_VECTOR3, TYPE_VECTOR3I, TYPE_VECTOR4, TYPE_VECTOR4I, \
        TYPE_COLOR, TYPE_RECT2, TYPE_RECT2I, TYPE_TRANSFORM2D, TYPE_QUATERNION, \
        TYPE_BASIS, TYPE_TRANSFORM3D, TYPE_AABB, TYPE_PLANE:
            return DecodeResult.failure("expected a tagged {\"$type\": ...} value for a %s property" % type_string(type_id))
        TYPE_BOOL:
            if value is bool:
                return DecodeResult.success(value)
            return DecodeResult.failure("expected a boolean")
        TYPE_INT:
            if value is int or value is float:
                return DecodeResult.success(int(value))
            return DecodeResult.failure("expected an integer")
        TYPE_FLOAT:
            if value is int or value is float:
                return DecodeResult.success(float(value))
            return DecodeResult.failure("expected a number")
        TYPE_STRING:
            if value is String:
                return DecodeResult.success(value)
            return DecodeResult.failure("expected a string")
        TYPE_STRING_NAME:
            if value is String:
                return DecodeResult.success(StringName(value))
            return DecodeResult.failure("expected a string")
        TYPE_NODE_PATH:
            if value is String:
                return DecodeResult.success(NodePath(value))
            return DecodeResult.failure("expected a string path")
        TYPE_PACKED_STRING_ARRAY:
            if value is Array:
                var out = PackedStringArray()
                for item in value:
                    if not (item is String):
                        return DecodeResult.failure("expected an array of strings")
                    out.append(item)
                return DecodeResult.success(out)
            return DecodeResult.failure("expected an array of strings")
        TYPE_OBJECT:
            return _decode_object(value)
    return _decode_value(value)

# Decode a value with no destination type. Tagged objects dispatch on their
# $type; plain dictionaries and arrays are decoded recursively so tagged values
# nested inside them still work. An unrecognized $type is an error, never a
# silent pass-through.
func _decode_value(value) -> DecodeResult:
    if value is Dictionary:
        if value.has("$type"):
            return _decode_typed(value)
        var result = {}
        for key in value:
            var decoded = _decode_value(value[key])
            if not decoded.ok:
                return decoded
            result[key] = decoded.value
        return DecodeResult.success(result)
    if value is Array:
        var items = []
        for item in value:
            var decoded = _decode_value(item)
            if not decoded.ok:
                return decoded
            items.append(decoded.value)
        return DecodeResult.success(items)
    return DecodeResult.success(value)

# Decode a tagged value by its $type. Every branch validates its fields and
# returns a DecodeResult; an unlisted tag is rejected.
func _decode_typed(value) -> DecodeResult:
    var tag = value.get("$type", "")
    match tag:
        "Vector2":
            var x = _number_field(value, "x")
            var y = _number_field(value, "y")
            return _decoded(Vector2(x.value, y.value), [x, y])
        "Vector2i":
            var x = _number_field(value, "x")
            var y = _number_field(value, "y")
            return _decoded(Vector2i(int(x.value), int(y.value)), [x, y])
        "Vector3":
            var x = _number_field(value, "x")
            var y = _number_field(value, "y")
            var z = _number_field(value, "z")
            return _decoded(Vector3(x.value, y.value, z.value), [x, y, z])
        "Vector3i":
            var x = _number_field(value, "x")
            var y = _number_field(value, "y")
            var z = _number_field(value, "z")
            return _decoded(Vector3i(int(x.value), int(y.value), int(z.value)), [x, y, z])
        "Vector4":
            var x = _number_field(value, "x")
            var y = _number_field(value, "y")
            var z = _number_field(value, "z")
            var w = _number_field(value, "w")
            return _decoded(Vector4(x.value, y.value, z.value, w.value), [x, y, z, w])
        "Vector4i":
            var x = _number_field(value, "x")
            var y = _number_field(value, "y")
            var z = _number_field(value, "z")
            var w = _number_field(value, "w")
            return _decoded(Vector4i(int(x.value), int(y.value), int(z.value), int(w.value)), [x, y, z, w])
        "Color":
            var r = _number_field(value, "r")
            var g = _number_field(value, "g")
            var b = _number_field(value, "b")
            var a = _number_field_or(value, "a", 1.0)
            return _decoded(Color(r.value, g.value, b.value, a.value), [r, g, b, a])
        "Rect2":
            var position = _vector_field(value, "position", TYPE_VECTOR2)
            var size = _vector_field(value, "size", TYPE_VECTOR2)
            return _decoded(Rect2(position.value, size.value), [position, size])
        "Rect2i":
            var position = _vector_field(value, "position", TYPE_VECTOR2I)
            var size = _vector_field(value, "size", TYPE_VECTOR2I)
            return _decoded(Rect2i(position.value, size.value), [position, size])
        "Transform2D":
            var x = _vector_field(value, "x", TYPE_VECTOR2)
            var y = _vector_field(value, "y", TYPE_VECTOR2)
            var origin = _vector_field(value, "origin", TYPE_VECTOR2)
            return _decoded(Transform2D(x.value, y.value, origin.value), [x, y, origin])
        "Quaternion":
            var x = _number_field(value, "x")
            var y = _number_field(value, "y")
            var z = _number_field(value, "z")
            var w = _number_field_or(value, "w", 1.0)
            return _decoded(Quaternion(x.value, y.value, z.value, w.value), [x, y, z, w])
        "Basis":
            var x = _vector_field(value, "x", TYPE_VECTOR3)
            var y = _vector_field(value, "y", TYPE_VECTOR3)
            var z = _vector_field(value, "z", TYPE_VECTOR3)
            return _decoded(Basis(x.value, y.value, z.value), [x, y, z])
        "Transform3D":
            var basis = _typed_field(value, "basis", "Basis")
            var origin = _vector_field(value, "origin", TYPE_VECTOR3)
            var failure = _last_failure([basis, origin])
            if failure != null:
                return failure
            return DecodeResult.success(Transform3D(basis.value, origin.value))
        "AABB":
            var position = _vector_field(value, "position", TYPE_VECTOR3)
            var size = _vector_field(value, "size", TYPE_VECTOR3)
            return _decoded(AABB(position.value, size.value), [position, size])
        "Plane":
            var normal = _vector_field(value, "normal", TYPE_VECTOR3)
            var d = _number_field(value, "d")
            return _decoded(Plane(normal.value, d.value), [normal, d])
        "NodePath":
            var path = _string_field(value, "path")
            return _decoded(NodePath(path.value), [path])
        "StringName":
            var name = _string_field(value, "value")
            return _decoded(StringName(name.value), [name])
        "PackedStringArray":
            var raw = value.get("values", null)
            if not (raw is Array):
                return DecodeResult.failure("$type PackedStringArray needs a \"values\" array")
            var out = PackedStringArray()
            for item in raw:
                if not (item is String):
                    return DecodeResult.failure("PackedStringArray values must be strings")
                out.append(item)
            return DecodeResult.success(out)
        "Resource":
            return _decode_resource(value)
        "GodotText":
            # Best-effort reparsing lets a scene or resource slot accept an
            # escape-hatch value; a type var_to_str cannot rebuild (an Object)
            # falls back to the literal text.
            var text = _string_field(value, "text")
            if not text.ok:
                return text
            var parsed = str_to_var(text.value)
            if parsed != null:
                return DecodeResult.success(parsed)
            return DecodeResult.success(text.value)
        "Dictionary":
            var inner = value.get("value", null)
            if inner is Dictionary:
                return _decode_value(inner)
            return DecodeResult.failure("$type Dictionary needs a \"value\" object")
    return DecodeResult.failure("unknown $type: " + str(tag))

# Carry a constructed composite value once every field decoded, or report the
# last field failure. Evaluating every field before returning mirrors the old
# shared-error behavior, where a later failure overwrote an earlier one, and the
# neutral fallback each field carries keeps null out of the constructor.
func _decoded(decoded, results) -> DecodeResult:
    var failure = _last_failure(results)
    if failure != null:
        return failure
    return DecodeResult.success(decoded)

# The last failing DecodeResult in results, or null when every result succeeded.
func _last_failure(results):
    var failure = null
    for result in results:
        if not result.ok:
            failure = result
    return failure

# Decode a nested tagged value, requiring the given tag.
func _typed_field(value, field, tag) -> DecodeResult:
    var inner = value.get(field, null)
    if inner is Dictionary and inner.get("$type", "") == tag:
        return _decode_typed(inner)
    return DecodeResult.failure("field \"" + field + "\" must be a {\"$type\": \"" + tag + "\"} value")

# Decode a nested Vector field, requiring and converting the given type. On
# failure it reports and returns that type's zero vector, so a composite
# constructor is never handed null (which the engine logs as a separate
# SCRIPT ERROR before the caller's decode error).
func _vector_field(value, field, type_id) -> DecodeResult:
    var inner = value.get(field, null)
    if inner is Dictionary and inner.has("$type"):
        var decoded = _decode_typed(inner)
        if not decoded.ok:
            return DecodeResult.failure(decoded.error, _zero_vector(type_id))
        if typeof(decoded.value) == type_id:
            return DecodeResult.success(decoded.value)
        return DecodeResult.failure("field \"" + field + "\" has the wrong vector type", _zero_vector(type_id))
    return DecodeResult.failure("field \"" + field + "\" must be a tagged vector value", _zero_vector(type_id))

# The zero value for a Vector Variant type id, returned by _vector_field when
# a field cannot be decoded.
func _zero_vector(type_id):
    match type_id:
        TYPE_VECTOR2:
            return Vector2.ZERO
        TYPE_VECTOR2I:
            return Vector2i.ZERO
        TYPE_VECTOR3:
            return Vector3.ZERO
        TYPE_VECTOR3I:
            return Vector3i.ZERO
        TYPE_VECTOR4:
            return Vector4.ZERO
        TYPE_VECTOR4I:
            return Vector4i.ZERO
    return null

# Decode an object-typed value. A bare res:// or user:// string is sugar for a
# Resource reference; a tagged Resource or GodotText also decodes.
func _decode_object(value) -> DecodeResult:
    if value is String:
        if value.begins_with("res://") or value.begins_with("user://"):
            return _decode_resource_path(value)
        return DecodeResult.failure("expected a res:// path or a tagged value for an object property")
    return _decode_value(value)

func _decode_resource(value) -> DecodeResult:
    var path = value.get("path", null)
    if not (path is String):
        return DecodeResult.failure("$type Resource needs a \"path\" string")
    return _decode_resource_path(path)

func _decode_resource_path(path) -> DecodeResult:
    if ResourceLoader.exists(path):
        var res = load(path)
        if res != null:
            return DecodeResult.success(res)
    return DecodeResult.failure("failed to load resource: " + path)

# Field readers. Each returns a DecodeResult whose value is a neutral fallback
# (0.0 or "") on failure, so a composite constructor never receives null; the
# caller propagates the failure before using the value.
func _number_field(value, field) -> DecodeResult:
    var raw = value.get(field, null)
    if raw is int or raw is float:
        return DecodeResult.success(float(raw))
    return DecodeResult.failure("expected a number for \"" + field + "\" in " + _tag_label(value), 0.0)

func _number_field_or(value, field, fallback) -> DecodeResult:
    if not value.has(field):
        return DecodeResult.success(fallback)
    return _number_field(value, field)

func _string_field(value, field) -> DecodeResult:
    var raw = value.get(field, null)
    if raw is String:
        return DecodeResult.success(raw)
    return DecodeResult.failure("expected a string for \"" + field + "\" in " + _tag_label(value), "")

func _tag_label(value) -> String:
    return "$type " + str(value.get("$type", "value"))

# Modify a node's properties in a scene file
func modify_node(result: Result, params) -> Result:
    if not params.has("scenePath") or not params.has("nodePath") or not params.has("properties"):
        result.err("scenePath, nodePath, and properties are required")
        return result.fail()

    var full_scene_path = params.scenePath
    if not full_scene_path.begins_with("res://"):
        full_scene_path = "res://" + full_scene_path

    log_info(result, "Modifying node in scene: " + full_scene_path)

    if not FileAccess.file_exists(full_scene_path):
        result.err("Scene file does not exist at: " + full_scene_path)
        return result.fail()
    if _reject_inherited_scene(result, full_scene_path):
        return result.fail()

    var scene = load(full_scene_path)
    if not scene:
        result.err("Failed to load scene: " + full_scene_path)
        return result.fail()

    var scene_root = track_temp(scene.instantiate())

    # Find the target node
    var node_path = params.nodePath
    var target = scene_root
    if node_path != "root" and node_path != ".":
        if node_path.begins_with("root/"):
            node_path = node_path.substr(5)
        target = scene_root.get_node_or_null(node_path)

    if target == null:
        result.err("Node not found: " + params.nodePath)
        return result.fail()

    # Set properties with type conversion
    var applied = _apply_properties(target, params.properties)
    if not applied.ok:
        result.err("Invalid property value: " + applied.error)
        return result.fail()

    # Repack and save
    var packed_scene = PackedScene.new()
    var pack_error = packed_scene.pack(scene_root)
    if pack_error != OK:
        result.err("Failed to pack scene after modification: " + str(pack_error))
        return result.fail()

    var save_error = ResourceSaver.save(packed_scene, full_scene_path)
    if save_error != OK:
        result.err("Failed to save modified scene: " + str(save_error))
        return result.fail()

    result.out("Node modified successfully in: " + full_scene_path)
    return result

# Remove a node from a scene file
func remove_node(result: Result, params) -> Result:
    if not params.has("scenePath") or not params.has("nodePath"):
        result.err("scenePath and nodePath are required")
        return result.fail()

    var full_scene_path = params.scenePath
    if not full_scene_path.begins_with("res://"):
        full_scene_path = "res://" + full_scene_path

    log_info(result, "Removing node from scene: " + full_scene_path)

    if not FileAccess.file_exists(full_scene_path):
        result.err("Scene file does not exist at: " + full_scene_path)
        return result.fail()
    if _reject_inherited_scene(result, full_scene_path):
        return result.fail()

    var scene = load(full_scene_path)
    if not scene:
        result.err("Failed to load scene: " + full_scene_path)
        return result.fail()

    var scene_root = track_temp(scene.instantiate())

    # Find the target node
    var node_path = params.nodePath
    var target = scene_root
    if node_path != "root" and node_path != ".":
        if node_path.begins_with("root/"):
            node_path = node_path.substr(5)
        target = scene_root.get_node_or_null(node_path)

    if target == null:
        result.err("Node not found: " + params.nodePath)
        return result.fail()

    if target == scene_root:
        result.err("Failed to remove the root node of a scene")
        return result.fail()

    var removed_name = target.name
    target.get_parent().remove_child(target)
    target.queue_free()

    # Repack and save
    var packed_scene = PackedScene.new()
    var pack_error = packed_scene.pack(scene_root)
    if pack_error != OK:
        result.err("Failed to pack scene after removal: " + str(pack_error))
        return result.fail()

    var save_error = ResourceSaver.save(packed_scene, full_scene_path)
    if save_error != OK:
        result.err("Failed to save scene after removal: " + str(save_error))
        return result.fail()

    result.out("Node '" + removed_name + "' removed successfully from: " + full_scene_path)
    return result

# List/add/remove signal connections in a scene file
func manage_scene_signals(result: Result, params) -> Result:
    var scene_path = params.get("scenePath", "")
    var action = params.get("action", "")
    var full_path = scene_path
    if not full_path.begins_with("res://"):
        full_path = "res://" + full_path

    if not FileAccess.file_exists(full_path):
        result.err("Scene not found: " + full_path)
        return result.fail()

    var content = FileAccess.get_file_as_string(full_path)

    if action == "add":
        var signal_name = params.get("signalName", "")
        var source_path = params.get("sourcePath", "")
        var target_path = params.get("targetPath", "")
        var method = params.get("method", "")
        for field in [signal_name, source_path, target_path, method]:
            if field.is_empty():
                result.err("signal_name, source_path, target_path, and method are required for add")
                return result.fail()
            if not _connection_field_is_safe(field):
                result.err("Signal connection fields cannot contain quotes, backslashes, or newlines")
                return result.fail()

        var scene = load(full_path) as PackedScene
        if scene == null:
            result.err("Failed to load scene: " + full_path)
            return result.fail()
        var scene_root = track_temp(scene.instantiate())
        if scene_root == null:
            result.err("Failed to instantiate scene: " + full_path)
            return result.fail()
        if _resolve_scene_node(scene_root, source_path) == null:
            result.err("Source node not found: " + source_path)
            return result.fail()
        if _resolve_scene_node(scene_root, target_path) == null:
            result.err("Target node not found: " + target_path)
            return result.fail()

        var from_path = _connection_node_path(source_path)
        var to_path = _connection_node_path(target_path)
        var conn_line = '[connection signal="%s" from="%s" to="%s" method="%s"]' % [signal_name, from_path, to_path, method]
        for line in content.split("\n"):
            if line.strip_edges() == conn_line:
                result.out("Signal connection already exists: " + conn_line)
                return result
        content += "\n" + conn_line + "\n"
        var file = FileAccess.open(full_path, FileAccess.WRITE)
        file.store_string(content)
        file.close()
        result.out("Signal connection added: " + conn_line)
    elif action == "remove":
        # A connection is identified by its signal, source, target, and (when
        # given) method. Matching on the signal alone would delete every
        # connection that shares a signal name, so source and target are
        # required to keep removal precise.
        var signal_name = params.get("signalName", "")
        var source_path = params.get("sourcePath", "")
        var target_path = params.get("targetPath", "")
        var method = params.get("method", "")
        for field in [signal_name, source_path, target_path]:
            if field.is_empty():
                result.err("signal_name, source_path, and target_path are required for remove")
                return result.fail()

        var from_path = _connection_node_path(source_path)
        var to_path = _connection_node_path(target_path)
        var lines = content.split("\n")
        var new_lines = []
        var removed = 0
        for line in lines:
            if line.begins_with("[connection") \
                    and _connection_field(line, "signal") == signal_name \
                    and _connection_field(line, "from") == from_path \
                    and _connection_field(line, "to") == to_path \
                    and (method.is_empty() or _connection_field(line, "method") == method):
                removed += 1
                continue
            new_lines.append(line)
        if removed == 0:
            result.out("No matching signal connection removed")
            return result
        var file = FileAccess.open(full_path, FileAccess.WRITE)
        if file == null:
            result.err("Failed to open scene for writing: " + full_path)
            return result.fail()
        file.store_string("\n".join(new_lines))
        file.close()
        result.out("Signal connection removed: %d" % [removed])
    else:
        result.err("Unknown manage_scene_signals action: " + action)
        return result.fail()
    return result

# Signal-connection helpers: keep the hand-written [connection] lines safe and
# resolve the tool's "root/Child" node paths to the scene-relative paths Godot
# actually stores ("." for the root, "Child" for a child).
func _connection_field_is_safe(value):
    return not ("\"" in value or "\\" in value or "\n" in value or "\r" in value)

func _connection_node_path(tool_path):
    if tool_path == "" or tool_path == "root" or tool_path == ".":
        return "."
    if tool_path.begins_with("root/"):
        return tool_path.substr(5)
    if tool_path.begins_with("./"):
        return tool_path.substr(2)
    return tool_path

func _connection_field(line, field_name):
    var marker = field_name + '="'
    var start = line.find(marker)
    if start == -1:
        return ""
    start += marker.length()
    var end = line.find('"', start)
    if end == -1:
        return ""
    return line.substr(start, end - start)

# Parse the scene's own [connection] lines out of its text. Shared by read_scene
# so a reader gets connections with the tree they belong to.
func _scene_connections(content):
    var connections = []
    for line in content.split("\n"):
        if line.begins_with("[connection"):
            connections.append({
                "signal": _connection_field(line, "signal"),
                "from": _connection_field(line, "from"),
                "to": _connection_field(line, "to"),
                "method": _connection_field(line, "method"),
            })
    return connections

# Rename/duplicate/move nodes within a scene file
func manage_scene_structure(result: Result, params) -> Result:
    var scene_path = params.get("scenePath", "")
    var action = params.get("action", "rename")
    var node_path_str = params.get("nodePath", "")
    var full_path = scene_path
    if not full_path.begins_with("res://"):
        full_path = "res://" + full_path

    if not FileAccess.file_exists(full_path):
        result.err("Scene not found: " + full_path)
        return result.fail()
    if _reject_inherited_scene(result, full_path):
        return result.fail()

    var scene = load(full_path) as PackedScene
    if scene == null:
        result.err("Failed to load scene: " + full_path)
        return result.fail()

    var root = track_temp(scene.instantiate())
    var target = _resolve_scene_node(root, node_path_str)
    if target == null:
        result.err("Node not found: " + node_path_str)
        return result.fail()

    if action == "rename":
        var new_name = params.get("newName", "")
        if new_name.is_empty():
            result.err("newName is required for rename")
            return result.fail()
        target.name = new_name
        result.out("Node renamed to '%s'" % target.name)
    elif action == "duplicate":
        if target == root:
            result.err("Cannot duplicate the root node")
            return result.fail()
        var dup = target.duplicate()
        target.get_parent().add_child(dup, true)
        _set_owner_recursive(dup, root)
        result.out("Node duplicated: %s (as '%s')" % [node_path_str, dup.name])
    elif action == "move":
        var new_parent_path = params.get("newParentPath", "")
        if new_parent_path.is_empty():
            result.err("newParentPath is required for move")
            return result.fail()
        if target == root:
            result.err("Cannot move the root node")
            return result.fail()
        var new_parent = _resolve_scene_node(root, new_parent_path)
        if new_parent == null:
            result.err("New parent not found: " + new_parent_path)
            return result.fail()
        if new_parent == target or _is_ancestor(target, new_parent):
            result.err("Cannot move a node into itself or one of its descendants")
            return result.fail()
        target.get_parent().remove_child(target)
        # Clear the owner before reparenting; keeping the old owner makes the
        # engine warn that the owner is inconsistent. It is reset below.
        target.owner = null
        new_parent.add_child(target, true)
        _set_owner_recursive(target, root)
        result.out("Node moved: %s -> parent %s (as '%s')" % [node_path_str, new_parent_path, target.name])
    else:
        result.err("Unknown manage_scene_structure action: " + action)
        return result.fail()

    var packed = PackedScene.new()
    var pack_result = packed.pack(root)
    if pack_result != OK:
        result.err("Failed to pack scene: " + str(pack_result))
        return result.fail()
    var save_error = ResourceSaver.save(packed, full_path)
    if save_error != OK:
        result.err("Failed to save scene: " + str(save_error))
        return result.fail()
    result.out("Scene structure saved: " + full_path)
    return result

func _resolve_scene_node(root, tool_path):
    if tool_path == "" or tool_path == "root" or tool_path == ".":
        return root
    var p = tool_path
    if p.begins_with("root/"):
        p = p.substr(5)
    return root.get_node_or_null(p)

func _set_owner_recursive(node, owner_root):
    node.owner = owner_root
    if node.scene_file_path != "":
        return
    for c in node.get_children():
        _set_owner_recursive(c, owner_root)

func _is_ancestor(node, maybe_descendant):
    var n = maybe_descendant.get_parent()
    while n != null:
        if n == node:
            return true
        n = n.get_parent()
    return false

# Attach a script to a node in a scene file
func attach_script(result: Result, params) -> Result:
    if not params.has("scenePath") or not params.has("nodePath") or not params.has("scriptPath"):
        result.err("scenePath, nodePath, and scriptPath are required")
        return result.fail()

    var full_scene_path = params.scenePath
    if not full_scene_path.begins_with("res://"):
        full_scene_path = "res://" + full_scene_path

    var full_script_path = params.scriptPath
    if not full_script_path.begins_with("res://"):
        full_script_path = "res://" + full_script_path

    log_info(result, "Attaching script " + full_script_path + " to node in scene: " + full_scene_path)

    if not FileAccess.file_exists(full_scene_path):
        result.err("Scene file does not exist at: " + full_scene_path)
        return result.fail()
    if _reject_inherited_scene(result, full_scene_path):
        return result.fail()

    if not FileAccess.file_exists(full_script_path):
        result.err("Script file does not exist at: " + full_script_path)
        return result.fail()

    var scene = load(full_scene_path)
    if not scene:
        result.err("Failed to load scene: " + full_scene_path)
        return result.fail()

    var scene_root = track_temp(scene.instantiate())

    # Find the target node
    var node_path = params.nodePath
    var target = scene_root
    if node_path != "root" and node_path != ".":
        if node_path.begins_with("root/"):
            node_path = node_path.substr(5)
        target = scene_root.get_node_or_null(node_path)

    if target == null:
        result.err("Node not found: " + params.nodePath)
        return result.fail()

    # Load and attach the script
    var script = load(full_script_path)
    if not script:
        result.err("Failed to load script: " + full_script_path)
        return result.fail()

    target.set_script(script)

    # Repack and save
    var packed_scene = PackedScene.new()
    var pack_error = packed_scene.pack(scene_root)
    if pack_error != OK:
        result.err("Failed to pack scene after attaching script: " + str(pack_error))
        return result.fail()

    var save_error = ResourceSaver.save(packed_scene, full_scene_path)
    if save_error != OK:
        result.err("Failed to save scene after attaching script: " + str(save_error))
        return result.fail()

    result.out("Script '" + full_script_path + "' attached successfully to node in: " + full_scene_path)
    return result

# Create a resource file (.tres)
func manage_resource(result: Result, params) -> Result:
    var resource_path = params.get("resourcePath", "")
    var action = params.get("action", "read")
    var full_path = resource_path
    if not full_path.begins_with("res://"):
        full_path = "res://" + full_path

    if action == "create":
        var resource_type = params.get("resourceType", "")
        if resource_type.is_empty():
            result.err("resourceType is required to create a resource")
            return result.fail()

        log_info(result, "Creating resource of type " + resource_type + " at: " + full_path)

        if not ClassDB.class_exists(resource_type):
            result.err("Unknown resource type: " + resource_type)
            result.err("Must be a valid Godot class name (e.g., StandardMaterial3D, AudioStreamPlayer, Theme)")
            return result.fail()

        if not ClassDB.can_instantiate(resource_type):
            result.err("Cannot instantiate resource type: " + resource_type)
            return result.fail()

        var resource = ClassDB.instantiate(resource_type)
        if resource == null:
            result.err("Failed to instantiate resource of type: " + resource_type)
            return result.fail()

        if not resource is Resource:
            result.err("Type " + resource_type + " is not a Resource subclass")
            return result.fail()

        var applied_create = _apply_resource_properties(resource, params.get("properties", {}))
        if not applied_create.ok:
            result.err("Invalid property value: " + applied_create.error)
            return result.fail()

        if not _save_resource(result, resource, full_path):
            return result

        result.out("Resource created: " + full_path)
    elif action == "read":
        var resource = _load_resource(result, full_path)
        if resource == null:
            return result
        var filter = params.get("propertyFilter", [])
        if filter is String:
            filter = [filter]
        var has_filter = filter is Array and filter.size() > 0
        var props = {}
        for prop in resource.get_property_list():
            if prop["usage"] & PROPERTY_USAGE_STORAGE:
                var prop_name = prop["name"]
                if has_filter and not _matches_property_filter(prop_name, filter):
                    continue
                props[prop_name] = _variant_to_json(resource.get(prop_name))
        result.out(JSON_START)
        result.out(JSON.stringify({"type": resource.get_class(), "path": project_relative(full_path), "properties": props}))
        result.out(JSON_END)
    elif action == "modify":
        var resource = _load_resource(result, full_path)
        if resource == null:
            return result
        var applied_modify = _apply_resource_properties(resource, params.get("properties", {}))
        if not applied_modify.ok:
            result.err("Invalid property value: " + applied_modify.error)
            return result.fail()
        if not _save_resource(result, resource, full_path):
            return result
        result.out("Resource modified: " + full_path)
    else:
        result.err("Unknown manage_resource action: " + action)
        return result.fail()
    return result


# Report whether a property name passes a caller's read filter. An entry
# matches by exact name or as a wildcard pattern using "*" and "?", so a family
# such as "level/*" can be selected without listing every key.
func _matches_property_filter(prop_name, filter) -> bool:
    for entry in filter:
        var pattern = str(entry)
        if pattern == prop_name or prop_name.match(pattern):
            return true
    return false


# Load a resource with the shared not-found and load-failure messages. Returns
# null after flagging the active result failed, so the caller returns it.
func _load_resource(result: Result, full_path):
    if not ResourceLoader.exists(full_path):
        result.err("Resource not found: " + full_path)
        result.ok = false
        return null
    var resource = ResourceLoader.load(full_path)
    if resource == null:
        result.err("Failed to load resource: " + full_path)
        result.ok = false
        return null
    return resource


# Save a resource, creating its directory first. Returns false after flagging
# failure so the caller can bail out instead of reporting a false success.
func _save_resource(result: Result, resource, full_path) -> bool:
    var dir_path = full_path.get_base_dir()
    var dir_relative = dir_path.substr(6)  # Remove "res://"
    if not dir_relative.is_empty():
        var dir = DirAccess.open("res://")
        if dir and not dir.dir_exists(dir_relative):
            dir.make_dir_recursive(dir_relative)
    var save_error = ResourceSaver.save(resource, full_path)
    if save_error != OK:
        result.err("Failed to save resource: " + str(save_error))
        result.ok = false
        return false
    return true


# Apply a caller's properties to a resource. Theme items need their value type
# inferred from the item path, so Themes go through the theme codec; every other
# resource uses the plain property decoder.
func _apply_resource_properties(resource, properties) -> DecodeResult:
    if resource is Theme:
        return _apply_theme_properties(resource, properties)
    return _apply_properties(resource, properties)


# Apply a caller's properties map to a Theme with type conversion. Theme items
# are keyed by composite names such as "Label/colors/font_color", so the value
# type is derived from the matching property entry when the item already exists
# and from the key's category when this call creates it. Returns a DecodeResult
# carrying the failure reason when a value cannot be decoded.
func _apply_theme_properties(theme, properties) -> DecodeResult:
    for key in properties:
        var decoded = _convert_theme_property(theme, key, properties[key])
        if not decoded.ok:
            return decoded
        theme.set(key, decoded.value)
    return DecodeResult.success(null)

func _convert_theme_property(theme, key, value) -> DecodeResult:
    var type_id = -1
    for prop in theme.get_property_list():
        if prop["name"] == key:
            type_id = prop.get("type", 0)
            break
    if type_id == -1:
        var parts = key.split("/")
        if parts.size() == 3:
            match parts[1]:
                "colors":
                    type_id = TYPE_COLOR
                "constants", "font_sizes":
                    type_id = TYPE_INT
                "fonts", "icons", "styles":
                    type_id = TYPE_OBJECT
    if type_id == -1:
        return DecodeResult.success(value)
    return _convert_value_for_type(type_id, value)

# Track type, loop mode, and interpolation names as they appear in the tool
# contract, mapped to the engine's enum values.
const ANIMATION_TRACK_TYPES = {
    "value": Animation.TYPE_VALUE,
    "position_3d": Animation.TYPE_POSITION_3D,
    "rotation_3d": Animation.TYPE_ROTATION_3D,
    "scale_3d": Animation.TYPE_SCALE_3D,
    "blend_shape": Animation.TYPE_BLEND_SHAPE,
    "method": Animation.TYPE_METHOD,
    "bezier": Animation.TYPE_BEZIER,
    "audio": Animation.TYPE_AUDIO,
    "animation": Animation.TYPE_ANIMATION,
}

const ANIMATION_LOOP_MODES = {
    "none": Animation.LOOP_NONE,
    "linear": Animation.LOOP_LINEAR,
    "pingpong": Animation.LOOP_PINGPONG,
}

const ANIMATION_INTERPOLATIONS = {
    "nearest": Animation.INTERPOLATION_NEAREST,
    "linear": Animation.INTERPOLATION_LINEAR,
    "cubic": Animation.INTERPOLATION_CUBIC,
}

# Read/create/modify an Animation resource and its tracks. Tracks are not
# properties, so the generic manage_resource codec cannot touch them; this
# operation is the dedicated path for animation authoring.
func manage_animation(result: Result, params) -> Result:
    var resource_path = params.get("resourcePath", "")
    var action = params.get("action", "read")
    var full_path = resource_path
    if not full_path.begins_with("res://"):
        full_path = "res://" + full_path

    if action == "read":
        _read_animation(result, full_path)
        return result

    if action == "create":
        var created = Animation.new()
        var created_properties = _apply_animation_properties(created, params)
        if not created_properties.ok:
            result.err("Invalid animation property: " + created_properties.error)
            return result.fail()
        if not _save_resource(result, created, full_path):
            return result
        result.out("Animation created: " + full_path)
        return result

    var animation = _load_animation(result, full_path)
    if animation == null:
        return result

    if action == "modify":
        var modified = _apply_animation_properties(animation, params)
        if not modified.ok:
            result.err("Invalid animation property: " + modified.error)
            return result.fail()
    elif action == "add":
        var added = _add_animation_track(animation, params)
        if not added.ok:
            result.err("Invalid animation track: " + added.error)
            return result.fail()
    elif action == "set":
        var set_track = _set_animation_track(animation, params)
        if not set_track.ok:
            result.err("Invalid animation track: " + set_track.error)
            return result.fail()
    elif action == "remove":
        var removed = _remove_animation_track(animation, params)
        if not removed.ok:
            result.err("Invalid animation track: " + removed.error)
            return result.fail()
    else:
        result.err("Unknown manage_animation action: " + action)
        return result.fail()

    if not _save_resource(result, animation, full_path):
        return result
    result.out("Animation " + action + " succeeded: " + full_path)
    return result

# Load an Animation resource with the shared not-found and type checks. Returns
# null after flagging the active result failed, so the caller returns it.
func _load_animation(result: Result, full_path):
    if not ResourceLoader.exists(full_path):
        result.err("Animation resource not found: " + full_path)
        result.ok = false
        return null
    var resource = ResourceLoader.load(full_path)
    if resource == null:
        result.err("Failed to load animation: " + full_path)
        result.ok = false
        return null
    if not (resource is Animation):
        result.err("Not an Animation resource: " + full_path)
        result.ok = false
        return null
    return resource

# Apply the animation-level properties length, loopMode, and step when present.
func _apply_animation_properties(animation, params) -> DecodeResult:
    if params.has("length"):
        var length = params["length"]
        if not (length is int or length is float):
            return DecodeResult.failure("length must be a number")
        animation.length = float(length)
    if params.has("loopMode"):
        var mode = str(params["loopMode"])
        if not ANIMATION_LOOP_MODES.has(mode):
            return DecodeResult.failure("unknown loopMode: " + mode)
        animation.loop_mode = ANIMATION_LOOP_MODES[mode]
    if params.has("step"):
        var step = params["step"]
        if not (step is int or step is float):
            return DecodeResult.failure("step must be a number")
        animation.step = float(step)
    return DecodeResult.success(null)

# Append a track built from the caller's trackType, trackPath, interpolation,
# and keys.
func _add_animation_track(animation, params) -> DecodeResult:
    var type_id = _track_type_id(params.get("trackType", ""))
    if type_id == -1:
        return DecodeResult.failure("unknown trackType: " + str(params.get("trackType", "")))
    var track_index = animation.add_track(type_id)
    return _configure_animation_track(animation, track_index, params)

# Update an existing track selected by trackIndex.
func _set_animation_track(animation, params) -> DecodeResult:
    var track_index = _track_index(animation, params)
    if not track_index.ok:
        return track_index
    return _configure_animation_track(animation, track_index.value, params)

# Configure a track's path, interpolation, loop wrap, enabled flag, and keys.
# A keys list replaces the track's existing keys wholesale, so set is
# deterministic rather than appending to whatever was there.
func _configure_animation_track(animation, track_index, params) -> DecodeResult:
    if params.has("trackPath"):
        animation.track_set_path(track_index, NodePath(str(params["trackPath"])))
    if params.has("interpolation"):
        var name = str(params["interpolation"])
        if not ANIMATION_INTERPOLATIONS.has(name):
            return DecodeResult.failure("unknown interpolation: " + name)
        animation.track_set_interpolation_type(track_index, ANIMATION_INTERPOLATIONS[name])
    if params.has("loopWrap"):
        animation.track_set_interpolation_loop_wrap(track_index, bool(params["loopWrap"]))
    if params.has("enabled"):
        animation.track_set_enabled(track_index, bool(params["enabled"]))
    if params.has("keys"):
        return _insert_animation_keys(animation, track_index, params["keys"])
    return DecodeResult.success(null)

# Remove the track selected by trackIndex.
func _remove_animation_track(animation, params) -> DecodeResult:
    var track_index = _track_index(animation, params)
    if not track_index.ok:
        return track_index
    animation.remove_track(track_index.value)
    return DecodeResult.success(null)

# Resolve and bounds-check the caller's trackIndex.
func _track_index(animation, params) -> DecodeResult:
    if not params.has("trackIndex"):
        return DecodeResult.failure("trackIndex is required", -1)
    var track_index = int(params["trackIndex"])
    if track_index < 0 or track_index >= animation.get_track_count():
        return DecodeResult.failure("trackIndex out of range: " + str(track_index), -1)
    return DecodeResult.success(track_index)

# Replace a track's keys with the caller's list, then insert each one.
func _insert_animation_keys(animation, track_index, keys) -> DecodeResult:
    if not (keys is Array):
        return DecodeResult.failure("keys must be an array")
    var type_id = animation.track_get_type(track_index)
    while animation.track_get_key_count(track_index) > 0:
        animation.track_remove_key(track_index, 0)
    for key in keys:
        if not (key is Dictionary) or not key.has("time"):
            return DecodeResult.failure("each key needs a \"time\" number")
        var time = key["time"]
        if not (time is int or time is float):
            return DecodeResult.failure("each key needs a \"time\" number")
        var inserted = _insert_animation_key(animation, track_index, type_id, float(time), key)
        if not inserted.ok:
            return inserted
    return DecodeResult.success(null)

# Insert one key, decoding its value for the track's type. Value tracks take a
# value in the tool value contract, 3D tracks take the matching tagged vector,
# method tracks take {method, args}, and bezier tracks take a number plus
# optional inHandle/outHandle offsets.
func _insert_animation_key(animation, track_index, type_id, time, key) -> DecodeResult:
    var value = key.get("value", null)
    if type_id == Animation.TYPE_VALUE:
        var decoded = _decode_value(value)
        if not decoded.ok:
            return decoded
        var transition = _animation_number_default(key.get("transition", 1.0), "transition")
        animation.track_insert_key(track_index, time, decoded.value, transition.value)
        if not transition.ok:
            return transition
    elif type_id == Animation.TYPE_POSITION_3D:
        var position = _require_tagged_type(value, TYPE_VECTOR3, "a position key value")
        if not position.ok:
            return position
        animation.position_track_insert_key(track_index, time, position.value)
    elif type_id == Animation.TYPE_ROTATION_3D:
        var rotation = _require_tagged_type(value, TYPE_QUATERNION, "a rotation key value")
        if not rotation.ok:
            return rotation
        animation.rotation_track_insert_key(track_index, time, rotation.value)
    elif type_id == Animation.TYPE_SCALE_3D:
        var scale = _require_tagged_type(value, TYPE_VECTOR3, "a scale key value")
        if not scale.ok:
            return scale
        animation.scale_track_insert_key(track_index, time, scale.value)
    elif type_id == Animation.TYPE_BLEND_SHAPE:
        var blend_shape = _animation_number(value, "a blend shape key value")
        animation.blend_shape_track_insert_key(track_index, time, blend_shape.value)
        if not blend_shape.ok:
            return blend_shape
    elif type_id == Animation.TYPE_METHOD:
        var method_key = _decode_method_key(value)
        animation.track_insert_key(track_index, time, method_key.value)
        if not method_key.ok:
            return method_key
    elif type_id == Animation.TYPE_BEZIER:
        var bezier_value = _animation_number(value, "a bezier key value")
        if not bezier_value.ok:
            return bezier_value
        var in_handle = _animation_vector2_or_zero(key.get("inHandle", null))
        if not in_handle.ok:
            return in_handle
        var out_handle = _animation_vector2_or_zero(key.get("outHandle", null))
        if not out_handle.ok:
            return out_handle
        animation.bezier_track_insert_key(track_index, time, bezier_value.value, in_handle.value, out_handle.value)
    elif type_id == Animation.TYPE_AUDIO:
        var stream = _animation_resource_value(value)
        if not stream.ok:
            return stream
        var start_offset = _animation_number_default(key.get("startOffset", 0.0), "startOffset")
        var end_offset = _animation_number_default(key.get("endOffset", 0.0), "endOffset")
        animation.audio_track_insert_key(track_index, time, stream.value, start_offset.value, end_offset.value)
        if not end_offset.ok:
            return end_offset
        if not start_offset.ok:
            return start_offset
    elif type_id == Animation.TYPE_ANIMATION:
        var nested = _animation_string_name(value)
        if not nested.ok:
            return nested
        animation.animation_track_insert_key(track_index, time, nested.value)
    else:
        return DecodeResult.failure("unsupported track type")
    return DecodeResult.success(null)

# Decode a method-track key value, an object with a "method" string and an
# optional "args" array.
func _decode_method_key(value) -> DecodeResult:
    if not (value is Dictionary) or not (value.get("method", null) is String):
        return DecodeResult.failure("a method key value must be an object with a \"method\" string")
    var args = []
    if value.has("args"):
        var decoded = _decode_value(value["args"])
        if not decoded.ok:
            return decoded
        if not (decoded.value is Array):
            return DecodeResult.failure("a method key's \"args\" must be an array")
        args = decoded.value
    return DecodeResult.success({"method": StringName(value["method"]), "args": args})

# Decode a value that must be a tagged object of the given Variant type.
func _require_tagged_type(value, type_id, label) -> DecodeResult:
    if not (value is Dictionary) or not value.has("$type"):
        return DecodeResult.failure("expected a tagged {\"$type\": ...} value for " + label)
    var decoded = _decode_typed(value)
    if not decoded.ok:
        return decoded
    if typeof(decoded.value) != type_id:
        return DecodeResult.failure(label + " must be a " + type_string(type_id))
    return decoded

# Decode a Vector2 handle, defaulting to zero when the caller omits it.
func _animation_vector2_or_zero(value) -> DecodeResult:
    if value == null:
        return DecodeResult.success(Vector2.ZERO)
    return _require_tagged_type(value, TYPE_VECTOR2, "a bezier handle")

# Read a number, reporting a decode failure when it is not numeric.
func _animation_number(value, label) -> DecodeResult:
    if value is int or value is float:
        return DecodeResult.success(float(value))
    return DecodeResult.failure(label + " must be a number", 0.0)

# Read a number that has a default when the caller omits the field.
func _animation_number_default(value, label) -> DecodeResult:
    return _animation_number(value, label)

# Decode a resource reference for an audio track value: a res:// path or a
# tagged Resource.
func _animation_resource_value(value) -> DecodeResult:
    if value is String:
        return _decode_resource_path(value)
    if value is Dictionary and value.has("$type"):
        return _decode_typed(value)
    return DecodeResult.failure("expected a res:// path or tagged Resource for this track's value")

# Decode an animation-track value: the name of the animation to play, as a bare
# string or a tagged StringName. Unlike an audio track, the engine stores a
# StringName here, not a Resource.
func _animation_string_name(value) -> DecodeResult:
    if value is String:
        return DecodeResult.success(StringName(value))
    if value is Dictionary and value.get("$type", "") == "StringName":
        return _decode_typed(value)
    return DecodeResult.failure("expected an animation name string or tagged StringName for this track's value")

func _track_type_id(name) -> int:
    if ANIMATION_TRACK_TYPES.has(name):
        return ANIMATION_TRACK_TYPES[name]
    return -1

func _track_type_name(type_id) -> String:
    for name in ANIMATION_TRACK_TYPES:
        if ANIMATION_TRACK_TYPES[name] == type_id:
            return name
    return "unknown"

func _loop_mode_name(mode) -> String:
    for name in ANIMATION_LOOP_MODES:
        if ANIMATION_LOOP_MODES[name] == mode:
            return name
    return "none"

func _interpolation_name(type_id) -> String:
    for name in ANIMATION_INTERPOLATIONS:
        if ANIMATION_INTERPOLATIONS[name] == type_id:
            return name
    return "linear"

# Emit an Animation resource as a JSON payload: length, loopMode, step, and one
# object per track with its keys.
func _read_animation(result: Result, full_path):
    var animation = _load_animation(result, full_path)
    if animation == null:
        return
    var tracks = []
    for index in range(animation.get_track_count()):
        tracks.append(_animation_track_to_json(animation, index))
    var payload = {
        "path": project_relative(full_path),
        "length": animation.length,
        "loopMode": _loop_mode_name(animation.loop_mode),
        "step": animation.step,
        "tracks": tracks,
    }
    result.out(JSON_START)
    result.out(JSON.stringify(payload))
    result.out(JSON_END)

func _animation_track_to_json(animation, index) -> Dictionary:
    var type_id = animation.track_get_type(index)
    var keys = []
    for key_index in range(animation.track_get_key_count(index)):
        keys.append(_animation_key_to_json(animation, index, type_id, key_index))
    return {
        "index": index,
        "type": _track_type_name(type_id),
        "path": String(animation.track_get_path(index)),
        "interpolation": _interpolation_name(animation.track_get_interpolation_type(index)),
        "loopWrap": animation.track_get_interpolation_loop_wrap(index),
        "enabled": animation.track_is_enabled(index),
        "keys": keys,
    }

func _animation_key_to_json(animation, index, type_id, key_index) -> Dictionary:
    var key = {
        "time": animation.track_get_key_time(index, key_index),
        "transition": animation.track_get_key_transition(index, key_index),
        "value": null,
    }
    if type_id == Animation.TYPE_METHOD:
        key["value"] = {
            "method": String(animation.method_track_get_name(index, key_index)),
            "args": _variant_to_json(animation.method_track_get_params(index, key_index)),
        }
    elif type_id == Animation.TYPE_BEZIER:
        key["value"] = animation.bezier_track_get_key_value(index, key_index)
        key["inHandle"] = _variant_to_json(animation.bezier_track_get_key_in_handle(index, key_index))
        key["outHandle"] = _variant_to_json(animation.bezier_track_get_key_out_handle(index, key_index))
    else:
        key["value"] = _variant_to_json(animation.track_get_key_value(index, key_index))
    return key

# A TileSet's sources, tiles, and per-tile data are not storage properties:
# they live behind add_source/create_tile/get_tile_data. A TileMapLayer's cell
# grid is likewise an opaque tile_map_data blob. Neither the generic
# manage_resource codec nor manage_scene_node's property codec can reach them,
# so these operations are the dedicated authoring paths, mirroring
# manage_animation for tracks.

const TILESET_SHAPES = {
    "square": TileSet.TILE_SHAPE_SQUARE,
    "isometric": TileSet.TILE_SHAPE_ISOMETRIC,
    "half_offset_square": TileSet.TILE_SHAPE_HALF_OFFSET_SQUARE,
    "hexagon": TileSet.TILE_SHAPE_HEXAGON,
}

const TILESET_LAYOUTS = {
    "stacked": TileSet.TILE_LAYOUT_STACKED,
    "stacked_offset": TileSet.TILE_LAYOUT_STACKED_OFFSET,
    "stairs_right": TileSet.TILE_LAYOUT_STAIRS_RIGHT,
    "stairs_down": TileSet.TILE_LAYOUT_STAIRS_DOWN,
    "diamond_right": TileSet.TILE_LAYOUT_DIAMOND_RIGHT,
    "diamond_down": TileSet.TILE_LAYOUT_DIAMOND_DOWN,
}

const TILESET_OFFSET_AXES = {
    "horizontal": TileSet.TILE_OFFSET_AXIS_HORIZONTAL,
    "vertical": TileSet.TILE_OFFSET_AXIS_VERTICAL,
}

# Read/create/modify a TileSet resource, its sources, and its tiles.
func manage_tileset(result: Result, params) -> Result:
    var resource_path = params.get("resourcePath", "")
    var action = params.get("action", "read")
    var full_path = resource_path
    if not full_path.begins_with("res://"):
        full_path = "res://" + full_path

    if action == "read":
        _read_tileset(result, full_path)
        return result

    if action == "create":
        var created = TileSet.new()
        var created_properties = _apply_tileset_properties(created, params)
        if not created_properties.ok:
            result.err("Invalid tileset property: " + created_properties.error)
            return result.fail()
        if not _save_resource(result, created, full_path):
            return result
        result.out("TileSet created: " + full_path)
        return result

    var tile_set = _load_tileset(result, full_path)
    if tile_set == null:
        return result

    match action:
        "modify":
            var modified = _apply_tileset_properties(tile_set, params)
            if not modified.ok:
                result.err("Invalid tileset property: " + modified.error)
                return result.fail()
        "addSource":
            var added_source = _add_tileset_source(result, tile_set, params)
            if not added_source.ok:
                result.err("Invalid tileset source: " + added_source.error)
                return result.fail()
        "removeSource":
            var removed_source = _remove_tileset_source(tile_set, params)
            if not removed_source.ok:
                result.err("Invalid tileset source: " + removed_source.error)
                return result.fail()
        "addTile":
            var added_tile = _add_tileset_tile(tile_set, params)
            if not added_tile.ok:
                result.err("Invalid tileset tile: " + added_tile.error)
                return result.fail()
        "removeTile":
            var removed_tile = _remove_tileset_tile(tile_set, params)
            if not removed_tile.ok:
                result.err("Invalid tileset tile: " + removed_tile.error)
                return result.fail()
        "addAlternative":
            var added_alternative = _add_tile_alternative(result, tile_set, params)
            if not added_alternative.ok:
                result.err("Invalid tile alternative: " + added_alternative.error)
                return result.fail()
        "removeAlternative":
            var removed_alternative = _remove_tile_alternative(tile_set, params)
            if not removed_alternative.ok:
                result.err("Invalid tile alternative: " + removed_alternative.error)
                return result.fail()
        "setTileData":
            var set_data = _set_tile_data(tile_set, params)
            if not set_data.ok:
                result.err("Invalid tile data: " + set_data.error)
                return result.fail()
        _:
            result.err("Unknown manage_tileset action: " + action)
            return result.fail()

    if not _save_resource(result, tile_set, full_path):
        return result
    result.out("TileSet " + action + " succeeded: " + full_path)
    return result

# Load a TileSet with the shared not-found and type checks. Returns null after
# flagging the active result failed, so the caller returns it.
func _load_tileset(result: Result, full_path):
    if not ResourceLoader.exists(full_path):
        result.err("TileSet resource not found: " + full_path)
        result.ok = false
        return null
    var resource = ResourceLoader.load(full_path)
    if resource == null:
        result.err("Failed to load tileset: " + full_path)
        result.ok = false
        return null
    if not (resource is TileSet):
        result.err("Not a TileSet resource: " + full_path)
        result.ok = false
        return null
    return resource

# Apply the tileset-level properties tileSize, tileShape, tileLayout,
# tileOffsetAxis, and uvClipping when present.
func _apply_tileset_properties(tile_set, params) -> DecodeResult:
    if params.has("tileSize"):
        var size = _require_tagged_type(params["tileSize"], TYPE_VECTOR2I, "tileSize")
        if not size.ok:
            return size
        tile_set.tile_size = size.value
    if params.has("tileShape"):
        var shape = str(params["tileShape"])
        if not TILESET_SHAPES.has(shape):
            return DecodeResult.failure("unknown tileShape: " + shape)
        tile_set.tile_shape = TILESET_SHAPES[shape]
    if params.has("tileLayout"):
        var layout = str(params["tileLayout"])
        if not TILESET_LAYOUTS.has(layout):
            return DecodeResult.failure("unknown tileLayout: " + layout)
        tile_set.tile_layout = TILESET_LAYOUTS[layout]
    if params.has("tileOffsetAxis"):
        var axis = str(params["tileOffsetAxis"])
        if not TILESET_OFFSET_AXES.has(axis):
            return DecodeResult.failure("unknown tileOffsetAxis: " + axis)
        tile_set.tile_offset_axis = TILESET_OFFSET_AXES[axis]
    if params.has("uvClipping"):
        tile_set.uv_clipping = bool(params["uvClipping"])
    return DecodeResult.success(null)

# Add an atlas or scenes-collection source to the tileset. An optional sourceId
# overrides the engine-assigned id; the chosen id is printed for the caller.
func _add_tileset_source(result: Result, tile_set, params) -> DecodeResult:
    var source_type = str(params.get("sourceType", ""))
    var source = null
    if source_type == "atlas":
        source = TileSetAtlasSource.new()
        if params.has("texture"):
            var texture_value = params["texture"]
            if texture_value is String and not texture_value.begins_with("res://") and not texture_value.begins_with("user://"):
                texture_value = "res://" + texture_value
            var texture = _decode_object(texture_value)
            if not texture.ok:
                return texture
            if not (texture.value is Texture2D):
                return DecodeResult.failure("texture must be a Texture2D resource")
            source.texture = texture.value
        if params.has("margins"):
            var margins = _require_tagged_type(params["margins"], TYPE_VECTOR2I, "margins")
            if not margins.ok:
                return margins
            source.margins = margins.value
        if params.has("separation"):
            var separation = _require_tagged_type(params["separation"], TYPE_VECTOR2I, "separation")
            if not separation.ok:
                return separation
            source.separation = separation.value
        if params.has("textureRegionSize"):
            var region = _require_tagged_type(params["textureRegionSize"], TYPE_VECTOR2I, "textureRegionSize")
            if not region.ok:
                return region
            source.texture_region_size = region.value
        if params.has("useTexturePadding"):
            source.use_texture_padding = bool(params["useTexturePadding"])
    elif source_type == "scenes":
        source = TileSetScenesCollectionSource.new()
    else:
        return DecodeResult.failure("sourceType must be atlas or scenes")
    var override_id = -1
    if params.has("sourceId"):
        override_id = int(params["sourceId"])
    var source_id = tile_set.add_source(source, override_id)
    result.out("TileSet source added: " + str(source_id))
    return DecodeResult.success(null)

# Remove the source selected by sourceId.
func _remove_tileset_source(tile_set, params) -> DecodeResult:
    var source_id = _require_source_id(tile_set, params)
    if not source_id.ok:
        return source_id
    tile_set.remove_source(source_id.value)
    return DecodeResult.success(null)

# Resolve and check the caller's sourceId against the tileset.
func _require_source_id(tile_set, params) -> DecodeResult:
    if not params.has("sourceId"):
        return DecodeResult.failure("sourceId is required", -1)
    var source_id = int(params["sourceId"])
    if not tile_set.has_source(source_id):
        return DecodeResult.failure("unknown sourceId: " + str(source_id), -1)
    return DecodeResult.success(source_id)

# Resolve a sourceId and require it to name an atlas source.
func _require_atlas_source(tile_set, params) -> DecodeResult:
    var source_id = _require_source_id(tile_set, params)
    if not source_id.ok:
        return source_id
    var source = tile_set.get_source(source_id.value)
    if not (source is TileSetAtlasSource):
        return DecodeResult.failure("source " + str(source_id.value) + " is not an atlas source")
    return DecodeResult.success(source)

# Add a tile. An atlas source takes atlasCoords (and optional sizeInAtlas); a
# scenes-collection source takes the scenePath of the PackedScene to place.
func _add_tileset_tile(tile_set, params) -> DecodeResult:
    var source_id = _require_source_id(tile_set, params)
    if not source_id.ok:
        return source_id
    var source = tile_set.get_source(source_id.value)
    if source is TileSetAtlasSource:
        var coords = _require_tagged_type(params.get("atlasCoords"), TYPE_VECTOR2I, "atlasCoords")
        if not coords.ok:
            return coords
        if source.has_tile(coords.value):
            return DecodeResult.failure("a tile already exists at " + str(coords.value))
        var size = Vector2i.ONE
        if params.has("sizeInAtlas"):
            var size_result = _require_tagged_type(params["sizeInAtlas"], TYPE_VECTOR2I, "sizeInAtlas")
            if not size_result.ok:
                return size_result
            size = size_result.value
        source.create_tile(coords.value, size)
        return DecodeResult.success(null)
    if source is TileSetScenesCollectionSource:
        var scene_path = str(params.get("scenePath", ""))
        if scene_path.is_empty():
            return DecodeResult.failure("scenePath is required to add a scene tile")
        if not scene_path.begins_with("res://"):
            scene_path = "res://" + scene_path
        var packed = load(scene_path)
        if not (packed is PackedScene):
            return DecodeResult.failure("scenePath must name a PackedScene: " + scene_path)
        var override_id = -1
        if params.has("sceneTileId"):
            override_id = int(params["sceneTileId"])
        var tile_id = source.create_scene_tile(packed, override_id)
        if params.has("displayPlaceholder"):
            source.set_scene_tile_display_placeholder(tile_id, bool(params["displayPlaceholder"]))
        return DecodeResult.success(null)
    return DecodeResult.failure("source " + str(source_id.value) + " has no tiles")

# Remove a tile. An atlas source is addressed by atlasCoords, a scenes-collection
# source by sceneTileId.
func _remove_tileset_tile(tile_set, params) -> DecodeResult:
    var source_id = _require_source_id(tile_set, params)
    if not source_id.ok:
        return source_id
    var source = tile_set.get_source(source_id.value)
    if source is TileSetAtlasSource:
        var coords = _require_tagged_type(params.get("atlasCoords"), TYPE_VECTOR2I, "atlasCoords")
        if not coords.ok:
            return coords
        if not source.has_tile(coords.value):
            return DecodeResult.failure("no tile at " + str(coords.value))
        source.remove_tile(coords.value)
        return DecodeResult.success(null)
    if source is TileSetScenesCollectionSource:
        if not params.has("sceneTileId"):
            return DecodeResult.failure("sceneTileId is required to remove a scene tile")
        var tile_id = int(params["sceneTileId"])
        if not source.has_scene_tile_id(tile_id):
            return DecodeResult.failure("unknown sceneTileId: " + str(tile_id))
        source.remove_scene_tile(tile_id)
        return DecodeResult.success(null)
    return DecodeResult.failure("source " + str(source_id.value) + " has no tiles")

# Add an alternative tile to the atlas tile at atlasCoords.
func _add_tile_alternative(result: Result, tile_set, params) -> DecodeResult:
    var atlas = _require_atlas_source(tile_set, params)
    if not atlas.ok:
        return atlas
    var coords = _require_tagged_type(params.get("atlasCoords"), TYPE_VECTOR2I, "atlasCoords")
    if not coords.ok:
        return coords
    if not atlas.value.has_tile(coords.value):
        return DecodeResult.failure("no tile at " + str(coords.value))
    var override_id = -1
    if params.has("alternativeId"):
        override_id = int(params["alternativeId"])
    var alternative_id = atlas.value.create_alternative_tile(coords.value, override_id)
    result.out("Tile alternative added: " + str(alternative_id))
    return DecodeResult.success(null)

# Remove the alternative tile selected by alternativeId.
func _remove_tile_alternative(tile_set, params) -> DecodeResult:
    var atlas = _require_atlas_source(tile_set, params)
    if not atlas.ok:
        return atlas
    var coords = _require_tagged_type(params.get("atlasCoords"), TYPE_VECTOR2I, "atlasCoords")
    if not coords.ok:
        return coords
    if not params.has("alternativeId"):
        return DecodeResult.failure("alternativeId is required")
    atlas.value.remove_alternative_tile(coords.value, int(params["alternativeId"]))
    return DecodeResult.success(null)

# Apply caller-supplied properties to the TileData of the tile at atlasCoords.
# Alt 0 is the tile's base data; pass alternativeId to target another one.
func _set_tile_data(tile_set, params) -> DecodeResult:
    var atlas = _require_atlas_source(tile_set, params)
    if not atlas.ok:
        return atlas
    var coords = _require_tagged_type(params.get("atlasCoords"), TYPE_VECTOR2I, "atlasCoords")
    if not coords.ok:
        return coords
    if not atlas.value.has_tile(coords.value):
        return DecodeResult.failure("no tile at " + str(coords.value))
    var alternative_id = 0
    if params.has("alternativeId"):
        alternative_id = int(params["alternativeId"])
    var tile_data = atlas.value.get_tile_data(coords.value, alternative_id)
    if tile_data == null:
        return DecodeResult.failure("no tile data for alternative " + str(alternative_id) + " at " + str(coords.value))
    if params.has("properties"):
        var applied = _apply_properties(tile_data, params["properties"])
        if not applied.ok:
            return applied
    return DecodeResult.success(null)

# Emit a TileSet as a JSON payload: the tileset-level settings and one object
# per source. An atlas source lists its tiles and each tile's alternatives.
func _read_tileset(result: Result, full_path):
    var tile_set = _load_tileset(result, full_path)
    if tile_set == null:
        return
    var sources = []
    for index in range(tile_set.get_source_count()):
        sources.append(_tileset_source_to_json(tile_set, tile_set.get_source_id(index)))
    var payload = {
        "path": project_relative(full_path),
        "tileSize": _variant_to_json(tile_set.tile_size),
        "tileShape": _enum_name(TILESET_SHAPES, tile_set.tile_shape),
        "tileLayout": _enum_name(TILESET_LAYOUTS, tile_set.tile_layout),
        "tileOffsetAxis": _enum_name(TILESET_OFFSET_AXES, tile_set.tile_offset_axis),
        "uvClipping": tile_set.uv_clipping,
        "sources": sources,
    }
    result.out(JSON_START)
    result.out(JSON.stringify(payload))
    result.out(JSON_END)

func _tileset_source_to_json(tile_set, source_id) -> Dictionary:
    var source = tile_set.get_source(source_id)
    if source is TileSetAtlasSource:
        return _atlas_source_to_json(source_id, source)
    if source is TileSetScenesCollectionSource:
        return _scenes_source_to_json(source_id, source)
    return {"id": source_id, "type": "unknown", "tiles": []}

func _atlas_source_to_json(source_id, source) -> Dictionary:
    var tiles = []
    for tile_index in range(source.get_tiles_count()):
        var coords = source.get_tile_id(tile_index)
        var alternatives = []
        for alt_index in range(source.get_alternative_tiles_count(coords)):
            var alternative_id = source.get_alternative_tile_id(coords, alt_index)
            alternatives.append({
                "id": alternative_id,
                "data": _variant_to_json(_tile_data_properties(source.get_tile_data(coords, alternative_id))),
            })
        tiles.append({
            "atlasCoords": _variant_to_json(coords),
            "sizeInAtlas": _variant_to_json(source.get_tile_size_in_atlas(coords)),
            "alternatives": alternatives,
        })
    return {
        "id": source_id,
        "type": "atlas",
        "texture": _source_resource_path(source.texture),
        "margins": _variant_to_json(source.margins),
        "separation": _variant_to_json(source.separation),
        "textureRegionSize": _variant_to_json(source.texture_region_size),
        "useTexturePadding": source.use_texture_padding,
        "tiles": tiles,
    }

func _scenes_source_to_json(source_id, source) -> Dictionary:
    var tiles = []
    for index in range(source.get_scene_tiles_count()):
        var tile_id = source.get_scene_tile_id(index)
        tiles.append({
            "sceneTileId": tile_id,
            "scene": _source_resource_path(source.get_scene_tile_scene(tile_id)),
            "displayPlaceholder": source.get_scene_tile_display_placeholder(tile_id),
        })
    return {"id": source_id, "type": "scenes", "tiles": tiles}

# The storage properties of a TileData, in the tool value contract.
func _tile_data_properties(tile_data) -> Dictionary:
    var props = {}
    if tile_data == null:
        return props
    for prop in tile_data.get_property_list():
        if prop["usage"] & PROPERTY_USAGE_STORAGE:
            props[prop["name"]] = tile_data.get(prop["name"])
    return props

# The project-relative path of a resource, or "" when it has none.
func _source_resource_path(resource) -> String:
    if resource == null or resource.resource_path == "":
        return ""
    return project_relative(resource.resource_path)

# The tool value contract name for an engine enum value.
func _enum_name(mapping, value) -> String:
    for name in mapping:
        if mapping[name] == value:
            return name
    return "unknown"

# Theme item categories as they appear in the tool contract, mapped to the
# engine's Theme.DataType values.
const THEME_ITEM_TYPES = {
    "color": Theme.DATA_TYPE_COLOR,
    "constant": Theme.DATA_TYPE_CONSTANT,
    "font": Theme.DATA_TYPE_FONT,
    "fontSize": Theme.DATA_TYPE_FONT_SIZE,
    "icon": Theme.DATA_TYPE_ICON,
    "stylebox": Theme.DATA_TYPE_STYLEBOX,
}

# Read/create/modify a Theme resource and its types, variations, and items.
# Theme items are keyed by composite names and variations are not storage
# properties at all, so the generic manage_resource codec cannot express the
# full contract; this operation is the dedicated authoring path, mirroring
# manage_animation for tracks and manage_tileset for sources and tiles.
func manage_theme(result: Result, params) -> Result:
    var resource_path = params.get("resourcePath", "")
    var action = params.get("action", "read")
    var full_path = resource_path
    if not full_path.begins_with("res://"):
        full_path = "res://" + full_path

    if action == "read":
        _read_theme(result, full_path)
        return result

    if action == "create":
        var created = Theme.new()
        var created_defaults = _apply_theme_defaults(created, params)
        if not created_defaults.ok:
            result.err("Invalid theme property: " + created_defaults.error)
            return result.fail()
        if not _save_resource(result, created, full_path):
            return result
        result.out("Theme created: " + full_path)
        return result

    var theme = _load_theme(result, full_path)
    if theme == null:
        return result

    match action:
        "modify":
            var modified = _apply_theme_defaults(theme, params)
            if not modified.ok:
                result.err("Invalid theme property: " + modified.error)
                return result.fail()
        "removeType":
            theme.remove_type(StringName(str(params.get("themeType", ""))))
        "setTypeVariation":
            theme.set_type_variation(StringName(str(params.get("themeType", ""))), StringName(str(params.get("baseType", ""))))
        "clearTypeVariation":
            theme.clear_type_variation(StringName(str(params.get("themeType", ""))))
        "setItem":
            var set_item = _set_theme_item(theme, params)
            if not set_item.ok:
                result.err("Invalid theme item: " + set_item.error)
                return result.fail()
        "clearItem":
            var clear_item = _clear_theme_item(theme, params)
            if not clear_item.ok:
                result.err("Invalid theme item: " + clear_item.error)
                return result.fail()
        _:
            result.err("Unknown manage_theme action: " + action)
            return result.fail()

    if not _save_resource(result, theme, full_path):
        return result
    result.out("Theme " + action + " succeeded: " + full_path)
    return result

# Load a Theme with the shared not-found and type checks. Returns null after
# flagging the active result failed, so the caller returns it.
func _load_theme(result: Result, full_path):
    if not ResourceLoader.exists(full_path):
        result.err("Theme resource not found: " + full_path)
        result.ok = false
        return null
    var resource = ResourceLoader.load(full_path)
    if resource == null:
        result.err("Failed to load theme: " + full_path)
        result.ok = false
        return null
    if not (resource is Theme):
        result.err("Not a Theme resource: " + full_path)
        result.ok = false
        return null
    return resource

# Apply the theme-level defaults defaultFont, defaultFontSize, and
# defaultBaseScale when present.
func _apply_theme_defaults(theme, params) -> DecodeResult:
    if params.has("defaultFont"):
        var value = params["defaultFont"]
        if value is String and not value.begins_with("res://") and not value.begins_with("user://"):
            value = "res://" + value
        var font = _decode_object(value)
        if not font.ok:
            return font
        if not (font.value is Font):
            return DecodeResult.failure("defaultFont must be a Font resource")
        theme.default_font = font.value
    if params.has("defaultFontSize"):
        var size = params["defaultFontSize"]
        if not (size is int or size is float):
            return DecodeResult.failure("defaultFontSize must be a number")
        theme.default_font_size = int(size)
    if params.has("defaultBaseScale"):
        var scale = params["defaultBaseScale"]
        if not (scale is int or scale is float):
            return DecodeResult.failure("defaultBaseScale must be a number")
        theme.default_base_scale = float(scale)
    return DecodeResult.success(null)

# Set one theme item, converting the value to the item category's engine type.
func _set_theme_item(theme, params) -> DecodeResult:
    var type_id = _theme_item_type_id(params.get("itemType", ""))
    if type_id == -1:
        return DecodeResult.failure("unknown itemType: " + str(params.get("itemType", "")))
    var value = _convert_theme_item_value(type_id, params.get("value", null))
    if not value.ok:
        return value
    theme.set_theme_item(type_id, StringName(str(params.get("itemName", ""))), StringName(str(params.get("themeType", ""))), value.value)
    return DecodeResult.success(null)

# Remove one theme item from a type.
func _clear_theme_item(theme, params) -> DecodeResult:
    var type_id = _theme_item_type_id(params.get("itemType", ""))
    if type_id == -1:
        return DecodeResult.failure("unknown itemType: " + str(params.get("itemType", "")))
    theme.clear_theme_item(type_id, StringName(str(params.get("itemName", ""))), StringName(str(params.get("themeType", ""))))
    return DecodeResult.success(null)

# Convert a caller's value to the engine type its item category requires: a
# tagged Color, an integer for constant/fontSize, or a resource for
# font/icon/stylebox.
func _convert_theme_item_value(type_id, value) -> DecodeResult:
    match type_id:
        Theme.DATA_TYPE_COLOR:
            return _require_tagged_type(value, TYPE_COLOR, "a color item value")
        Theme.DATA_TYPE_CONSTANT, Theme.DATA_TYPE_FONT_SIZE:
            return _convert_value_for_type(TYPE_INT, value)
        Theme.DATA_TYPE_FONT, Theme.DATA_TYPE_ICON, Theme.DATA_TYPE_STYLEBOX:
            if value is String and not value.begins_with("res://") and not value.begins_with("user://"):
                value = "res://" + value
            return _convert_value_for_type(TYPE_OBJECT, value)
    return DecodeResult.failure("unsupported item type")

func _theme_item_type_id(name) -> int:
    if THEME_ITEM_TYPES.has(name):
        return THEME_ITEM_TYPES[name]
    return -1

# Emit a Theme as a JSON payload: the theme-level defaults and one object per
# type with its items grouped by category.
func _read_theme(result: Result, full_path):
    var theme = _load_theme(result, full_path)
    if theme == null:
        return
    var types = []
    for type_name in theme.get_type_list():
        types.append(_theme_type_to_json(theme, type_name))
    var payload = {
        "path": project_relative(full_path),
        "defaultFont": _variant_to_json(theme.default_font),
        "defaultFontSize": theme.default_font_size if theme.has_default_font_size() else null,
        "defaultBaseScale": theme.default_base_scale if theme.has_default_base_scale() else null,
        "types": types,
    }
    result.out(JSON_START)
    result.out(JSON.stringify(payload))
    result.out(JSON_END)

func _theme_type_to_json(theme, type_name) -> Dictionary:
    return {
        "name": String(type_name),
        "variationBase": String(theme.get_type_variation_base(type_name)),
        "colors": _theme_items_to_json(theme, Theme.DATA_TYPE_COLOR, type_name),
        "constants": _theme_items_to_json(theme, Theme.DATA_TYPE_CONSTANT, type_name),
        "fonts": _theme_items_to_json(theme, Theme.DATA_TYPE_FONT, type_name),
        "fontSizes": _theme_items_to_json(theme, Theme.DATA_TYPE_FONT_SIZE, type_name),
        "icons": _theme_items_to_json(theme, Theme.DATA_TYPE_ICON, type_name),
        "styleboxes": _theme_items_to_json(theme, Theme.DATA_TYPE_STYLEBOX, type_name),
    }

func _theme_items_to_json(theme, type_id, theme_type) -> Dictionary:
    var items = {}
    for name in theme.get_theme_item_list(type_id, theme_type):
        items[String(name)] = _variant_to_json(theme.get_theme_item(type_id, name, theme_type))
    return items

# Read or edit the cells of a TileMapLayer node in a scene. Cells are not
# properties (the layer stores them in an opaque tile_map_data blob), so a
# scene or property codec cannot reach them.
func manage_tilemap(result: Result, params) -> Result:
    var scene_path = params.get("scenePath", "")
    var action = params.get("action", "read")
    var node_path = str(params.get("nodePath", ""))
    var full_path = scene_path
    if not full_path.begins_with("res://"):
        full_path = "res://" + full_path

    if not FileAccess.file_exists(full_path):
        result.err("Scene file does not exist at: " + full_path)
        return result.fail()
    var scene = load(full_path)
    if not (scene is PackedScene):
        result.err("Failed to load scene: " + full_path)
        return result.fail()
    var scene_root = track_temp(scene.instantiate())
    var target = _resolve_scene_node(scene_root, node_path)
    if target == null:
        result.err("Node not found: " + node_path)
        return result.fail()
    if not (target is TileMapLayer):
        result.err("Node is not a TileMapLayer: " + node_path)
        return result.fail()

    if action == "read":
        _read_tilemap_node(result, target, full_path, node_path)
        return result

    match action:
        "setCell":
            var set_cell = _set_tilemap_cell(target, params)
            if not set_cell.ok:
                result.err("Invalid tilemap cell: " + set_cell.error)
                return result.fail()
        "eraseCell":
            var coords = _require_tagged_type(params.get("coords"), TYPE_VECTOR2I, "coords")
            if not coords.ok:
                result.err("Invalid tilemap cell: " + coords.error)
                return result.fail()
            target.erase_cell(coords.value)
        "clear":
            target.clear()
        _:
            result.err("Unknown manage_tilemap action: " + action)
            return result.fail()

    var packed_scene = PackedScene.new()
    var pack_error = packed_scene.pack(scene_root)
    if pack_error != OK:
        result.err("Failed to pack scene after tilemap edit: " + str(pack_error))
        return result.fail()
    var save_error = ResourceSaver.save(packed_scene, full_path)
    if save_error != OK:
        result.err("Failed to save tilemap scene: " + str(save_error))
        return result.fail()
    result.out("TileMap " + action + " succeeded: " + full_path)
    return result

# Place one cell, checking that the node has a tileset that owns sourceId.
func _set_tilemap_cell(target, params) -> DecodeResult:
    var coords = _require_tagged_type(params.get("coords"), TYPE_VECTOR2I, "coords")
    if not coords.ok:
        return coords
    var atlas_coords = _require_tagged_type(params.get("atlasCoords"), TYPE_VECTOR2I, "atlasCoords")
    if not atlas_coords.ok:
        return atlas_coords
    if not params.has("sourceId"):
        return DecodeResult.failure("sourceId is required")
    var source_id = int(params["sourceId"])
    if target.tile_set == null:
        return DecodeResult.failure("the node has no tileSet")
    if not target.tile_set.has_source(source_id):
        return DecodeResult.failure("unknown sourceId: " + str(source_id))
    var alternative_id = 0
    if params.has("alternativeTile"):
        alternative_id = int(params["alternativeTile"])
    target.set_cell(coords.value, source_id, atlas_coords.value, alternative_id)
    return DecodeResult.success(null)

# Emit a TileMapLayer's tileset path and used cells, sorted for stable output.
func _read_tilemap_node(result: Result, target, full_path, node_path):
    var cells = []
    for coords in target.get_used_cells():
        cells.append({
            "coords": _variant_to_json(coords),
            "sourceId": target.get_cell_source_id(coords),
            "atlasCoords": _variant_to_json(target.get_cell_atlas_coords(coords)),
            "alternativeTile": target.get_cell_alternative_tile(coords),
            "flippedH": target.is_cell_flipped_h(coords),
            "flippedV": target.is_cell_flipped_v(coords),
            "transposed": target.is_cell_transposed(coords),
        })
    cells.sort_custom(func(a, b):
        if a["coords"]["x"] != b["coords"]["x"]:
            return a["coords"]["x"] < b["coords"]["x"]
        return a["coords"]["y"] < b["coords"]["y"])
    var tile_set_path = ""
    if target.tile_set != null:
        tile_set_path = _source_resource_path(target.tile_set)
    var payload = {
        "scenePath": project_relative(full_path),
        "nodePath": node_path,
        "tileSet": tile_set_path,
        "usedCells": cells,
    }
    result.out(JSON_START)
    result.out(JSON.stringify(payload))
    result.out(JSON_END)

# Validate that Godot itself loaded project.godot cleanly. ProjectSettings
# parses project.godot during engine startup, before _init runs; when the file
# is malformed the engine logs "Error parsing '<path>/project.godot'" to stderr
# and leaves the affected settings at their defaults. Starting the engine
# against the project is the whole check, and the caller combines that stderr
# diagnostic with the exit code, so this operation deliberately does no work of
# its own and emits no payload for anyone to read.
func validate_project(result: Result, _params) -> Result:
    # Starting the engine against the project is the whole check; the caller
    # combines the engine's own stderr diagnostics with the exit code. No work
    # or payload is produced here.
    return result
