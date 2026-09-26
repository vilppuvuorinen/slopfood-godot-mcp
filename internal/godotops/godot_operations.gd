#!/usr/bin/env -S godot --headless --script
# Adapted from tugcantopaloglu/godot-mcp (src/scripts/godot_operations.gd), a
# fork of Coding-Solo/godot-mcp (both MIT-licensed); most of this file is
# copied verbatim from those projects.
extends SceneTree

const JSON_START = "<<<GO_DOT_MCP_JSON_START>>>"
const JSON_END = "<<<GO_DOT_MCP_JSON_END>>>"

func _init():
    var args = OS.get_cmdline_args()
    
    # Find the script argument and determine the positions of operation and params
    var script_index = args.find("--script")
    if script_index == -1:
        log_error("Could not find --script argument")
        quit(1)
    
    # The operation should be 2 positions after the script path (script_index + 1 is the script path itself)
    var operation_index = script_index + 2
    # The params should be 3 positions after the script path
    var params_index = script_index + 3
    
    if args.size() <= params_index:
        log_error("Usage: godot --headless --script godot_operations.gd <operation> <json_params>")
        log_error("Not enough command-line arguments provided.")
        quit(1)
    
    var operation = args[operation_index]
    var params_json = args[params_index]
    
    log_info("Operation: " + operation)
    
    # Parse JSON using Godot 4.x API
    var json = JSON.new()
    var error = json.parse(params_json)
    var params = null
    
    if error == OK:
        params = json.get_data()
    else:
        log_error("Failed to parse JSON parameters: " + params_json)
        log_error("JSON Error: " + json.get_error_message() + " at line " + str(json.get_error_line()))
        quit(1)
    
    if params == null:
        log_error("Failed to parse JSON parameters: " + params_json)
        quit(1)
    
    log_info("Executing operation: " + operation)

    # Decoding a caller-supplied value is fallible; reset the shared error so a
    # stale message from a prior decode can never be mistaken for a current one.
    _decode_error = ""

    match operation:
        "create_scene":
            create_scene(params)
        "add_node":
            add_node(params)
        "export_mesh_library":
            export_mesh_library(params)
        "read_scene":
            read_scene(params)
        "modify_node":
            modify_node(params)
        "remove_node":
            remove_node(params)
        "manage_scene_structure":
            manage_scene_structure(params)
        "manage_scene_signals":
            manage_scene_signals(params)
        "attach_script":
            attach_script(params)
        "get_uid":
            get_uid(params)
        "manage_resource":
            manage_resource(params)
        "manage_animation":
            manage_animation(params)
        "manage_tileset":
            manage_tileset(params)
        "manage_theme":
            manage_theme(params)
        "manage_tilemap":
            manage_tilemap(params)
        "validate_project":
            validate_project(params)
        _:
            log_error("Unknown operation: " + operation)
            quit(1)

# Handlers signal failure with quit(1). SceneTree.quit() does not stop the
# current call, so a trailing quit() here would reset the exit code to 0 and
# mask every error. Ending the main loop from _process instead preserves the
# code set by the handler while still exiting cleanly after success.
#
# Temporary nodes instantiated by handlers are tracked in _temp_nodes and freed
# here before the main loop exits; they are never added to the scene tree, so
# without this the engine reports ObjectDB/RID leaks at exit.
var _temp_nodes = []

# _decode_error is set by the value decoder when a caller-supplied value cannot
# be turned into the Variant a property expects. Handlers check it after
# applying properties and abort before saving, so a rejected value never leaves
# a partially-written file behind.
var _decode_error = ""

func track_temp(node):
    _temp_nodes.append(node)
    return node

func _process(_delta):
    for node in _temp_nodes:
        if is_instance_valid(node):
            node.free()
    _temp_nodes.clear()
    return true

# Logging functions
func log_info(message):
    print("[INFO] " + message)

func log_error(message):
    printerr("[ERROR] " + message)

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
func get_script_by_name(name_of_class):
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
                printerr("Failed to load script from registry path: " + found_path)
                break
    
    printerr("Could not find script for class: " + name_of_class)
    return null

# Instantiate a class by name
func instantiate_class(name_of_class):
    if name_of_class.is_empty():
        printerr("Cannot instantiate class: name is empty")
        return null
    
    var result = null
    # Check if it's a built-in class
    if ClassDB.class_exists(name_of_class):
        if ClassDB.can_instantiate(name_of_class):
            result = ClassDB.instantiate(name_of_class)
            if result == null:
                printerr("ClassDB.instantiate() returned null for class: " + name_of_class)
        else:
            printerr("Class exists but cannot be instantiated: " + name_of_class)
            printerr("This may be an abstract class or interface that cannot be directly instantiated")
    else:
        # Try to get the script
        var script = get_script_by_name(name_of_class)
        if script is GDScript:
            result = script.new()
        else:
            printerr("Failed to get script for class: " + name_of_class)
            return null
    
    if result == null:
        printerr("Failed to instantiate class: " + name_of_class)
    return result

# Ensure the directory holding a target scene file exists, creating it
# recursively. Returns false (after printing the reason) when it cannot be
# created.
func _ensure_scene_directory(full_scene_path):
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
        printerr("Failed to open res:// directory: " + str(open_error))
        var make_dir_error = DirAccess.make_dir_recursive_absolute(scene_dir_abs)
        if make_dir_error != OK:
            printerr("Failed to create directory using absolute path")
            printerr("Error code: " + str(make_dir_error))
            return false
    else:
        var make_dir_error = dir.make_dir_recursive(scene_dir_relative)
        if make_dir_error != OK:
            printerr("Failed to create directory: " + scene_dir_relative)
            printerr("Error code: " + str(make_dir_error))
            return false
    if not DirAccess.dir_exists_absolute(scene_dir_abs):
        printerr("Directory reported as created but does not exist: " + scene_dir_abs)
        printerr("This may indicate a problem with path resolution or permissions")
        return false
    return true

# Create an inherited scene: a scene whose root node instances another
# PackedScene. The engine cannot author one at runtime (PackedScene.pack
# flattens instances), so the minimal file is written directly. base_scene_path
# may be project-relative or a res:// path.
func _create_inherited_scene(full_scene_path, base_scene_path):
    var full_base_path = base_scene_path
    if not full_base_path.begins_with("res://"):
        full_base_path = "res://" + full_base_path
    if "\"" in full_base_path or "\n" in full_base_path:
        printerr("Invalid base scene path: " + base_scene_path)
        quit(1)
        return
    if not FileAccess.file_exists(full_base_path):
        printerr("Base scene does not exist at: " + full_base_path)
        quit(1)
        return

    var content = "[gd_scene load_steps=2 format=3]\n\n"
    content += "[ext_resource type=\"PackedScene\" path=\"" + full_base_path + "\" id=\"1_base\"]\n\n"
    content += "[node name=\"root\" instance=ExtResource(\"1_base\")]\n"

    var file = FileAccess.open(full_scene_path, FileAccess.WRITE)
    if file == null:
        printerr("Failed to create scene: " + full_scene_path)
        quit(1)
        return
    file.store_string(content)
    file.close()
    if not FileAccess.file_exists(full_scene_path):
        printerr("Failed to create scene: " + full_scene_path)
        quit(1)
        return
    print("Scene created successfully at: " + project_relative(full_scene_path))

# Create a new scene with a specified root node type, or an inherited scene when
# baseScenePath names another scene.
func create_scene(params):
    print("Creating scene: " + params.scenePath)
    
    # Normalize the scene path
    var full_scene_path = params.scenePath
    if not full_scene_path.begins_with("res://"):
        full_scene_path = "res://" + full_scene_path
    if not _ensure_scene_directory(full_scene_path):
        quit(1)
        return

    # An inherited scene's root is an instance of the base scene rather than a
    # fresh node, so baseScenePath replaces rootNodeType entirely.
    var base_scene_path = str(params.get("baseScenePath", ""))
    if not base_scene_path.is_empty():
        _create_inherited_scene(full_scene_path, base_scene_path)
        return

    # Use traditional if-else statement for better compatibility
    var root_node_type = "Node2D"  # Default value
    if params.has("rootNodeType"):
        root_node_type = params.rootNodeType
    # Create the root node
    var scene_root = track_temp(instantiate_class(root_node_type))
    if not scene_root:
        printerr("Failed to instantiate node of type: " + root_node_type)
        printerr("Make sure the class exists and can be instantiated")
        printerr("Check if the class is registered in ClassDB or available as a script")
        quit(1)
    
    scene_root.name = "root"
    # Pack the scene
    var packed_scene = PackedScene.new()
    var result = packed_scene.pack(scene_root)
    if result == OK:
        # Save the scene
        var save_error = ResourceSaver.save(packed_scene, full_scene_path)
        if save_error == OK:
            # Check the scene file was actually written
            var file_exists = FileAccess.file_exists(full_scene_path)
            if file_exists:
                print("Scene created successfully at: " + params.scenePath)
            else:
                printerr("Failed to create scene: " + params.scenePath)
                quit(1)
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
            
            printerr(error_message)
            quit(1)
    else:
        printerr("Failed to pack scene: " + str(result))
        printerr("Error code: " + str(result))
        quit(1)

# Add a node to an existing scene
func add_node(params):
    print("Adding node to scene: " + params.scenePath)
    
    var full_scene_path = params.scenePath
    if not full_scene_path.begins_with("res://"):
        full_scene_path = "res://" + full_scene_path
    var absolute_scene_path = ProjectSettings.globalize_path(full_scene_path)
    if not FileAccess.file_exists(absolute_scene_path):
        printerr("Scene file does not exist at: " + absolute_scene_path)
        quit(1)
    if _reject_inherited_scene(full_scene_path):
        quit(1)
        return
    
    var scene = load(full_scene_path)
    if not scene:
        printerr("Failed to load scene: " + full_scene_path)
        quit(1)
    
    var scene_root = track_temp(scene.instantiate())
    # Use traditional if-else statement for better compatibility
    var parent_path = "root"  # Default value
    if params.has("parentNodePath"):
        parent_path = params.parentNodePath
    var parent = scene_root
    if parent_path != "root":
        parent = scene_root.get_node(parent_path.replace("root/", ""))
        if not parent:
            printerr("Parent node not found: " + parent_path)
            quit(1)
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
            printerr("Instance scene does not exist at: " + full_instance_path)
            quit(1)
        var packed_instance = load(full_instance_path)
        if not (packed_instance is PackedScene):
            printerr("Not a scene file: " + full_instance_path)
            quit(1)
        new_node = track_temp(packed_instance.instantiate())
        if not new_node:
            printerr("Failed to instantiate scene: " + full_instance_path)
            quit(1)
    else:
        new_node = track_temp(instantiate_class(params.nodeType))
        if not new_node:
            printerr("Failed to instantiate node of type: " + params.nodeType)
            printerr("Make sure the class exists and can be instantiated")
            printerr("Check if the class is registered in ClassDB or available as a script")
            quit(1)
    new_node.name = params.nodeName
    if params.has("properties"):
        if not _apply_properties(new_node, params.properties):
            printerr("Invalid property value: " + _decode_error)
            quit(1)
            return

    parent.add_child(new_node)
    new_node.owner = scene_root
    var packed_scene = PackedScene.new()
    var result = packed_scene.pack(scene_root)
    if result == OK:
        var save_error = ResourceSaver.save(packed_scene, absolute_scene_path)
        if save_error == OK:
            if instance_path.is_empty():
                print("Node '" + params.nodeName + "' of type '" + params.nodeType + "' added successfully")
            else:
                print("Scene '" + instance_path + "' instanced as '" + params.nodeName + "' added successfully")
        else:
            printerr("Failed to save scene: " + str(save_error))
            quit(1)
    else:
        printerr("Failed to pack scene: " + str(result))
        quit(1)

# Export a scene as a MeshLibrary resource
func export_mesh_library(params):
    print("Exporting MeshLibrary from scene: " + params.scenePath)
    
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
        printerr("Scene file does not exist at: " + full_scene_path)
        # Get the absolute path for reference
        var absolute_path = ProjectSettings.globalize_path(full_scene_path)
        printerr("Absolute file path that doesn't exist: " + absolute_path)
        quit(1)
    
    # Load the scene
    var scene = load(full_scene_path)
    if not scene:
        printerr("Failed to load scene: " + full_scene_path)
        quit(1)
    
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
            printerr("Failed to open res:// directory")
            printerr("DirAccess error: " + str(DirAccess.get_open_error()))
            quit(1)
        if output_dir != "res://" and not dir.dir_exists(output_dir.substr(6)):  # Remove "res://" prefix
            var make_error = dir.make_dir_recursive(output_dir.substr(6))  # Remove "res://" prefix
            if make_error != OK:
                printerr("Failed to create directory: " + output_dir + ", error: " + str(make_error))
                quit(1)
    elif not DirAccess.dir_exists_absolute(output_dir):
        var make_error = DirAccess.make_dir_recursive_absolute(output_dir)
        if make_error != OK:
            printerr("Failed to create directory: " + output_dir + ", error: " + str(make_error))
            quit(1)
    
    # Save the mesh library
    if item_id > 0:
        var error = ResourceSaver.save(mesh_library, full_output_path)
        if error == OK:
            # Verify the file was actually created
            print("MeshLibrary exported successfully with " + str(item_id) + " items to: " + full_output_path)
        else:
            printerr("Failed to save MeshLibrary: " + str(error))
            quit(1)
    else:
        printerr("No valid meshes found in the scene")
        quit(1)

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

# Get UID for a specific file
func get_uid(params):
    if not params.has("filePath"):
        printerr("File path is required")
        quit(1)
    
    # Ensure the file path starts with res:// for Godot's resource system
    var file_path = params.filePath
    if not file_path.begins_with("res://"):
        file_path = "res://" + file_path
    
    print("Getting UID for file: " + file_path)
    # Get the absolute path for reference
    var absolute_path = ProjectSettings.globalize_path(file_path)
    # Ensure the file exists
    var file_check = FileAccess.file_exists(file_path)
    if not file_check:
        printerr("File does not exist at: " + file_path)
        printerr("Absolute file path that doesn't exist: " + absolute_path)
        quit(1)
    
    # Check if the UID file exists
    var uid_path = file_path + ".uid"
    var uid_check = FileAccess.file_exists(uid_path)
    var f = FileAccess.open(uid_path, FileAccess.READ)
    
    if f:
        # Read the UID content
        var uid_content = f.get_as_text()
        f.close()
        # Return the UID content
        var result = {
            "file": project_relative(file_path),
            "absolutePath": absolute_path,
            "uid": uid_content.strip_edges(),
            "exists": true
        }
        print(JSON_START)
        print(JSON.stringify(result))
        print(JSON_END)
    else:
        # UID file doesn't exist
        var result = {
            "file": project_relative(file_path),
            "absolutePath": absolute_path,
            "exists": false,
            "message": "UID file does not exist for this file. Use update_project_uids to scan the project and generate UIDs."
        }
        print(JSON_START)
        print(JSON.stringify(result))
        print(JSON_END)

# Read a scene file as a JSON node tree
func read_scene(params):
    if not params.has("scenePath"):
        printerr("scenePath is required")
        quit(1)

    var full_scene_path = params.scenePath
    if not full_scene_path.begins_with("res://"):
        full_scene_path = "res://" + full_scene_path

    log_info("Reading scene: " + full_scene_path)

    if not FileAccess.file_exists(full_scene_path):
        printerr("Scene file does not exist at: " + full_scene_path)
        quit(1)

    # load() succeeds for any resource, so scenePath can point at a non-scene
    # such as a .tres. That has no instantiate() and would otherwise die as a
    # SCRIPT ERROR, so anything that is not a PackedScene takes the raw-text
    # fallback. A null load is a parse/IO failure whose engine errors already
    # reach stderr, so no extra printerr here: it would only duplicate them.
    var scene = load(full_scene_path)
    if scene == null or not (scene is PackedScene):
        var f = FileAccess.open(full_scene_path, FileAccess.READ)
        if f:
            var raw_content = f.get_as_text()
            f.close()
            print(JSON_START)
            print(JSON.stringify({"reason": "Failed to instantiate scene, returning raw text", "raw": raw_content}))
            print(JSON_END)
            return
        quit(1)

    var scene_root = track_temp(scene.instantiate())
    if scene_root == null:
        printerr("Failed to instantiate scene: " + full_scene_path)
        quit(1)

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
                printerr("Node not found: " + str(params.nodePath))
                quit(1)
                return

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

    print(JSON_START)
    print(JSON.stringify(tree_data))
    print(JSON_END)

    scene_root.queue_free()

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
func _reject_inherited_scene(full_scene_path):
    var base_scene_path = _inherited_scene_base(full_scene_path)
    if base_scene_path.is_empty():
        return false
    printerr("Cannot edit inherited scene: " + full_scene_path)
    printerr("Its root instances " + base_scene_path + "; editing it would flatten the base scene into this file")
    printerr("Edit the base scene instead, or recreate this scene with create_scene")
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

# Set every property, decoding each one first. Returns false and leaves the
# shared decode error set when a value cannot be decoded, so the caller aborts
# before writing anything.
func _apply_properties(target, properties) -> bool:
    for prop_name in properties:
        var decoded = _convert_property_value(target, prop_name, properties[prop_name])
        if _decode_error != "":
            return false
        target.set(prop_name, decoded)
    return true

# Convert a caller-supplied value into the type a target property expects. The
# value must already be in the tool value contract, with a $type tag for any
# engine-only type; a bare JSON object is a plain Dictionary, never a Vector2.
func _convert_property_value(target, prop_name, value):
    for prop in target.get_property_list():
        if prop["name"] == prop_name:
            return _convert_value_for_type(prop.get("type", 0), value)
    return _decode_value(value)

# Convert a value to the Godot type identified by a Variant type id. Native
# JSON scalars are coerced to the property's declared type; engine-only types
# must arrive as a matching $type object.
func _convert_value_for_type(type_id, value):
    if value is Dictionary and value.has("$type"):
        return _decode_typed(value)
    match type_id:
        TYPE_VECTOR2, TYPE_VECTOR2I, TYPE_VECTOR3, TYPE_VECTOR3I, TYPE_VECTOR4, TYPE_VECTOR4I, \
        TYPE_COLOR, TYPE_RECT2, TYPE_RECT2I, TYPE_TRANSFORM2D, TYPE_QUATERNION, \
        TYPE_BASIS, TYPE_TRANSFORM3D, TYPE_AABB, TYPE_PLANE:
            return _decode_fail("expected a tagged {\"$type\": ...} value for a %s property" % type_string(type_id))
        TYPE_BOOL:
            if value is bool:
                return value
            return _decode_fail("expected a boolean")
        TYPE_INT:
            if value is int or value is float:
                return int(value)
            return _decode_fail("expected an integer")
        TYPE_FLOAT:
            if value is int or value is float:
                return float(value)
            return _decode_fail("expected a number")
        TYPE_STRING:
            if value is String:
                return value
            return _decode_fail("expected a string")
        TYPE_STRING_NAME:
            if value is String:
                return StringName(value)
            return _decode_fail("expected a string")
        TYPE_NODE_PATH:
            if value is String:
                return NodePath(value)
            return _decode_fail("expected a string path")
        TYPE_PACKED_STRING_ARRAY:
            if value is Array:
                var out = PackedStringArray()
                for item in value:
                    if not (item is String):
                        return _decode_fail("expected an array of strings")
                    out.append(item)
                return out
            return _decode_fail("expected an array of strings")
        TYPE_OBJECT:
            return _decode_object(value)
    return _decode_value(value)

# Decode a value with no destination type. Tagged objects dispatch on their
# $type; plain dictionaries and arrays are decoded recursively so tagged values
# nested inside them still work. An unrecognized $type is an error, never a
# silent pass-through.
func _decode_value(value):
    if value is Dictionary:
        if value.has("$type"):
            return _decode_typed(value)
        var result = {}
        for key in value:
            var decoded = _decode_value(value[key])
            if _decode_error != "":
                return null
            result[key] = decoded
        return result
    if value is Array:
        var items = []
        for item in value:
            var decoded = _decode_value(item)
            if _decode_error != "":
                return null
            items.append(decoded)
        return items
    return value

# Decode a tagged value by its $type. Every branch validates its fields and
# reports through _decode_error; an unlisted tag is rejected.
func _decode_typed(value):
    var tag = value.get("$type", "")
    match tag:
        "Vector2":
            return Vector2(_number_field(value, "x"), _number_field(value, "y"))
        "Vector2i":
            return Vector2i(int(_number_field(value, "x")), int(_number_field(value, "y")))
        "Vector3":
            return Vector3(_number_field(value, "x"), _number_field(value, "y"), _number_field(value, "z"))
        "Vector3i":
            return Vector3i(int(_number_field(value, "x")), int(_number_field(value, "y")), int(_number_field(value, "z")))
        "Vector4":
            return Vector4(_number_field(value, "x"), _number_field(value, "y"), _number_field(value, "z"), _number_field(value, "w"))
        "Vector4i":
            return Vector4i(int(_number_field(value, "x")), int(_number_field(value, "y")), int(_number_field(value, "z")), int(_number_field(value, "w")))
        "Color":
            return Color(_number_field(value, "r"), _number_field(value, "g"), _number_field(value, "b"), _number_field_or(value, "a", 1.0))
        "Rect2":
            return Rect2(_vector_field(value, "position", TYPE_VECTOR2), _vector_field(value, "size", TYPE_VECTOR2))
        "Rect2i":
            return Rect2i(_vector_field(value, "position", TYPE_VECTOR2I), _vector_field(value, "size", TYPE_VECTOR2I))
        "Transform2D":
            return Transform2D(_vector_field(value, "x", TYPE_VECTOR2), _vector_field(value, "y", TYPE_VECTOR2), _vector_field(value, "origin", TYPE_VECTOR2))
        "Quaternion":
            return Quaternion(_number_field(value, "x"), _number_field(value, "y"), _number_field(value, "z"), _number_field_or(value, "w", 1.0))
        "Basis":
            return Basis(_vector_field(value, "x", TYPE_VECTOR3), _vector_field(value, "y", TYPE_VECTOR3), _vector_field(value, "z", TYPE_VECTOR3))
        "Transform3D":
            var basis = _typed_field(value, "basis", "Basis")
            var origin = _vector_field(value, "origin", TYPE_VECTOR3)
            if _decode_error != "":
                return null
            return Transform3D(basis, origin)
        "AABB":
            return AABB(_vector_field(value, "position", TYPE_VECTOR3), _vector_field(value, "size", TYPE_VECTOR3))
        "Plane":
            return Plane(_vector_field(value, "normal", TYPE_VECTOR3), _number_field(value, "d"))
        "NodePath":
            return NodePath(_string_field(value, "path"))
        "StringName":
            return StringName(_string_field(value, "value"))
        "PackedStringArray":
            var raw = value.get("values", null)
            if not (raw is Array):
                return _decode_fail("$type PackedStringArray needs a \"values\" array")
            var out = PackedStringArray()
            for item in raw:
                if not (item is String):
                    return _decode_fail("PackedStringArray values must be strings")
                out.append(item)
            return out
        "Resource":
            return _decode_resource(value)
        "GodotText":
            # Best-effort reparsing lets a scene or resource slot accept an
            # escape-hatch value; a type var_to_str cannot rebuild (an Object)
            # falls back to the literal text.
            var text = _string_field(value, "text")
            if _decode_error != "":
                return null
            var parsed = str_to_var(text)
            if parsed != null:
                return parsed
            return text
        "Dictionary":
            var inner = value.get("value", null)
            if inner is Dictionary:
                return _decode_value(inner)
            return _decode_fail("$type Dictionary needs a \"value\" object")
    return _decode_fail("unknown $type: " + str(tag))

# Decode a nested tagged value, requiring the given tag.
func _typed_field(value, field, tag):
    var inner = value.get(field, null)
    if inner is Dictionary and inner.get("$type", "") == tag:
        return _decode_typed(inner)
    return _decode_fail("field \"" + field + "\" must be a {\"$type\": \"" + tag + "\"} value")

# Decode a nested Vector field, requiring and converting the given type. On
# failure it reports and returns that type's zero vector, so a composite
# constructor is never handed null (which the engine logs as a separate
# SCRIPT ERROR before the caller's decode error).
func _vector_field(value, field, type_id):
    var inner = value.get(field, null)
    if inner is Dictionary and inner.has("$type"):
        var decoded = _decode_typed(inner)
        if _decode_error != "":
            return _zero_vector(type_id)
        if typeof(decoded) == type_id:
            return decoded
        _decode_error = "field \"" + field + "\" has the wrong vector type"
        return _zero_vector(type_id)
    _decode_error = "field \"" + field + "\" must be a tagged vector value"
    return _zero_vector(type_id)

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
func _decode_object(value):
    if value is String:
        if value.begins_with("res://") or value.begins_with("user://"):
            return _decode_resource_path(value)
        return _decode_fail("expected a res:// path or a tagged value for an object property")
    return _decode_value(value)

func _decode_resource(value):
    var path = value.get("path", null)
    if not (path is String):
        return _decode_fail("$type Resource needs a \"path\" string")
    return _decode_resource_path(path)

func _decode_resource_path(path):
    if ResourceLoader.exists(path):
        var res = load(path)
        if res != null:
            return res
    return _decode_fail("failed to load resource: " + path)

# Field readers. Each reports through _decode_error and returns a zero value on
# failure; the caller's error check discards that zero value.
func _number_field(value, field) -> float:
    var raw = value.get(field, null)
    if raw is int or raw is float:
        return float(raw)
    _decode_error = "expected a number for \"" + field + "\" in " + _tag_label(value)
    return 0.0

func _number_field_or(value, field, fallback) -> float:
    if not value.has(field):
        return fallback
    return _number_field(value, field)

func _string_field(value, field) -> String:
    var raw = value.get(field, null)
    if raw is String:
        return raw
    _decode_error = "expected a string for \"" + field + "\" in " + _tag_label(value)
    return ""

func _tag_label(value) -> String:
    return "$type " + str(value.get("$type", "value"))

# _decode_fail records a decode error and returns null; handlers check
# _decode_error and abort before saving.
func _decode_fail(message):
    _decode_error = message
    return null

# Modify a node's properties in a scene file
func modify_node(params):
    if not params.has("scenePath") or not params.has("nodePath") or not params.has("properties"):
        printerr("scenePath, nodePath, and properties are required")
        quit(1)

    var full_scene_path = params.scenePath
    if not full_scene_path.begins_with("res://"):
        full_scene_path = "res://" + full_scene_path

    log_info("Modifying node in scene: " + full_scene_path)

    if not FileAccess.file_exists(full_scene_path):
        printerr("Scene file does not exist at: " + full_scene_path)
        quit(1)
    if _reject_inherited_scene(full_scene_path):
        quit(1)
        return

    var scene = load(full_scene_path)
    if not scene:
        printerr("Failed to load scene: " + full_scene_path)
        quit(1)

    var scene_root = track_temp(scene.instantiate())

    # Find the target node
    var node_path = params.nodePath
    var target = scene_root
    if node_path != "root" and node_path != ".":
        if node_path.begins_with("root/"):
            node_path = node_path.substr(5)
        target = scene_root.get_node_or_null(node_path)

    if target == null:
        printerr("Node not found: " + params.nodePath)
        quit(1)

    # Set properties with type conversion
    if not _apply_properties(target, params.properties):
        printerr("Invalid property value: " + _decode_error)
        quit(1)
        return

    # Repack and save
    var packed_scene = PackedScene.new()
    var result = packed_scene.pack(scene_root)
    if result != OK:
        printerr("Failed to pack scene after modification: " + str(result))
        quit(1)

    var save_error = ResourceSaver.save(packed_scene, full_scene_path)
    if save_error != OK:
        printerr("Failed to save modified scene: " + str(save_error))
        quit(1)

    print("Node modified successfully in: " + full_scene_path)

# Remove a node from a scene file
func remove_node(params):
    if not params.has("scenePath") or not params.has("nodePath"):
        printerr("scenePath and nodePath are required")
        quit(1)

    var full_scene_path = params.scenePath
    if not full_scene_path.begins_with("res://"):
        full_scene_path = "res://" + full_scene_path

    log_info("Removing node from scene: " + full_scene_path)

    if not FileAccess.file_exists(full_scene_path):
        printerr("Scene file does not exist at: " + full_scene_path)
        quit(1)
    if _reject_inherited_scene(full_scene_path):
        quit(1)
        return

    var scene = load(full_scene_path)
    if not scene:
        printerr("Failed to load scene: " + full_scene_path)
        quit(1)

    var scene_root = track_temp(scene.instantiate())

    # Find the target node
    var node_path = params.nodePath
    var target = scene_root
    if node_path != "root" and node_path != ".":
        if node_path.begins_with("root/"):
            node_path = node_path.substr(5)
        target = scene_root.get_node_or_null(node_path)

    if target == null:
        printerr("Node not found: " + params.nodePath)
        quit(1)

    if target == scene_root:
        printerr("Failed to remove the root node of a scene")
        quit(1)

    var removed_name = target.name
    target.get_parent().remove_child(target)
    target.queue_free()

    # Repack and save
    var packed_scene = PackedScene.new()
    var result = packed_scene.pack(scene_root)
    if result != OK:
        printerr("Failed to pack scene after removal: " + str(result))
        quit(1)

    var save_error = ResourceSaver.save(packed_scene, full_scene_path)
    if save_error != OK:
        printerr("Failed to save scene after removal: " + str(save_error))
        quit(1)

    print("Node '" + removed_name + "' removed successfully from: " + full_scene_path)

# List/add/remove signal connections in a scene file
func manage_scene_signals(params):
    var scene_path = params.get("scenePath", "")
    var action = params.get("action", "list")
    var full_path = scene_path
    if not full_path.begins_with("res://"):
        full_path = "res://" + full_path

    if not FileAccess.file_exists(full_path):
        printerr("Scene not found: " + full_path)
        quit(1)
        return

    var content = FileAccess.get_file_as_string(full_path)

    if action == "list":
        var connections = []
        var lines = content.split("\n")
        for line in lines:
            if line.begins_with("[connection"):
                connections.append({
                    "signal": _connection_field(line, "signal"),
                    "from": _connection_field(line, "from"),
                    "to": _connection_field(line, "to"),
                    "method": _connection_field(line, "method"),
                })
        print(JSON_START)
        print(JSON.stringify({"connections": connections}))
        print(JSON_END)
    elif action == "add":
        var signal_name = params.get("signalName", "")
        var source_path = params.get("sourcePath", "")
        var target_path = params.get("targetPath", "")
        var method = params.get("method", "")
        for field in [signal_name, source_path, target_path, method]:
            if field.is_empty():
                printerr("signal_name, source_path, target_path, and method are required for add")
                quit(1)
                return
            if not _connection_field_is_safe(field):
                printerr("Signal connection fields cannot contain quotes, backslashes, or newlines")
                quit(1)
                return

        var scene = load(full_path) as PackedScene
        if scene == null:
            printerr("Failed to load scene: " + full_path)
            quit(1)
            return
        var scene_root = track_temp(scene.instantiate())
        if scene_root == null:
            printerr("Failed to instantiate scene: " + full_path)
            quit(1)
            return
        if _resolve_scene_node(scene_root, source_path) == null:
            printerr("Source node not found: " + source_path)
            quit(1)
            return
        if _resolve_scene_node(scene_root, target_path) == null:
            printerr("Target node not found: " + target_path)
            quit(1)
            return

        var from_path = _connection_node_path(source_path)
        var to_path = _connection_node_path(target_path)
        var conn_line = '[connection signal="%s" from="%s" to="%s" method="%s"]' % [signal_name, from_path, to_path, method]
        for line in content.split("\n"):
            if line.strip_edges() == conn_line:
                print("Signal connection already exists: " + conn_line)
                return
        content += "\n" + conn_line + "\n"
        var file = FileAccess.open(full_path, FileAccess.WRITE)
        file.store_string(content)
        file.close()
        print("Signal connection added: " + conn_line)
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
                printerr("signal_name, source_path, and target_path are required for remove")
                quit(1)
                return

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
            print("No matching signal connection removed")
            return
        var file = FileAccess.open(full_path, FileAccess.WRITE)
        if file == null:
            printerr("Failed to open scene for writing: " + full_path)
            quit(1)
            return
        file.store_string("\n".join(new_lines))
        file.close()
        print("Signal connection removed: %d" % [removed])
    else:
        printerr("Unknown manage_scene_signals action: " + action)
        quit(1)

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

# Rename/duplicate/move nodes within a scene file
func manage_scene_structure(params):
    var scene_path = params.get("scenePath", "")
    var action = params.get("action", "rename")
    var node_path_str = params.get("nodePath", "")
    var full_path = scene_path
    if not full_path.begins_with("res://"):
        full_path = "res://" + full_path

    if not FileAccess.file_exists(full_path):
        printerr("Scene not found: " + full_path)
        quit(1)
        return
    if _reject_inherited_scene(full_path):
        quit(1)
        return

    var scene = load(full_path) as PackedScene
    if scene == null:
        printerr("Failed to load scene: " + full_path)
        quit(1)
        return

    var root = track_temp(scene.instantiate())
    var target = _resolve_scene_node(root, node_path_str)
    if target == null:
        printerr("Node not found: " + node_path_str)
        quit(1)
        return

    if action == "rename":
        var new_name = params.get("newName", "")
        if new_name.is_empty():
            printerr("newName is required for rename")
            quit(1)
            return
        target.name = new_name
        print("Node renamed to '%s'" % target.name)
    elif action == "duplicate":
        if target == root:
            printerr("Cannot duplicate the root node")
            quit(1)
            return
        var dup = target.duplicate()
        target.get_parent().add_child(dup, true)
        _set_owner_recursive(dup, root)
        print("Node duplicated: %s (as '%s')" % [node_path_str, dup.name])
    elif action == "move":
        var new_parent_path = params.get("newParentPath", "")
        if new_parent_path.is_empty():
            printerr("newParentPath is required for move")
            quit(1)
            return
        if target == root:
            printerr("Cannot move the root node")
            quit(1)
            return
        var new_parent = _resolve_scene_node(root, new_parent_path)
        if new_parent == null:
            printerr("New parent not found: " + new_parent_path)
            quit(1)
            return
        if new_parent == target or _is_ancestor(target, new_parent):
            printerr("Cannot move a node into itself or one of its descendants")
            quit(1)
            return
        target.get_parent().remove_child(target)
        # Clear the owner before reparenting; keeping the old owner makes the
        # engine warn that the owner is inconsistent. It is reset below.
        target.owner = null
        new_parent.add_child(target, true)
        _set_owner_recursive(target, root)
        print("Node moved: %s -> parent %s (as '%s')" % [node_path_str, new_parent_path, target.name])
    else:
        printerr("Unknown manage_scene_structure action: " + action)
        quit(1)
        return

    var packed = PackedScene.new()
    var pack_result = packed.pack(root)
    if pack_result != OK:
        printerr("Failed to pack scene: " + str(pack_result))
        quit(1)
        return
    var save_error = ResourceSaver.save(packed, full_path)
    if save_error != OK:
        printerr("Failed to save scene: " + str(save_error))
        quit(1)
        return
    print("Scene structure saved: " + full_path)

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
func attach_script(params):
    if not params.has("scenePath") or not params.has("nodePath") or not params.has("scriptPath"):
        printerr("scenePath, nodePath, and scriptPath are required")
        quit(1)

    var full_scene_path = params.scenePath
    if not full_scene_path.begins_with("res://"):
        full_scene_path = "res://" + full_scene_path

    var full_script_path = params.scriptPath
    if not full_script_path.begins_with("res://"):
        full_script_path = "res://" + full_script_path

    log_info("Attaching script " + full_script_path + " to node in scene: " + full_scene_path)

    if not FileAccess.file_exists(full_scene_path):
        printerr("Scene file does not exist at: " + full_scene_path)
        quit(1)
    if _reject_inherited_scene(full_scene_path):
        quit(1)
        return

    if not FileAccess.file_exists(full_script_path):
        printerr("Script file does not exist at: " + full_script_path)
        quit(1)

    var scene = load(full_scene_path)
    if not scene:
        printerr("Failed to load scene: " + full_scene_path)
        quit(1)

    var scene_root = track_temp(scene.instantiate())

    # Find the target node
    var node_path = params.nodePath
    var target = scene_root
    if node_path != "root" and node_path != ".":
        if node_path.begins_with("root/"):
            node_path = node_path.substr(5)
        target = scene_root.get_node_or_null(node_path)

    if target == null:
        printerr("Node not found: " + params.nodePath)
        quit(1)

    # Load and attach the script
    var script = load(full_script_path)
    if not script:
        printerr("Failed to load script: " + full_script_path)
        quit(1)

    target.set_script(script)

    # Repack and save
    var packed_scene = PackedScene.new()
    var result = packed_scene.pack(scene_root)
    if result != OK:
        printerr("Failed to pack scene after attaching script: " + str(result))
        quit(1)

    var save_error = ResourceSaver.save(packed_scene, full_scene_path)
    if save_error != OK:
        printerr("Failed to save scene after attaching script: " + str(save_error))
        quit(1)

    print("Script '" + full_script_path + "' attached successfully to node in: " + full_scene_path)

# Create a resource file (.tres)
func manage_resource(params):
    var resource_path = params.get("resourcePath", "")
    var action = params.get("action", "read")
    var full_path = resource_path
    if not full_path.begins_with("res://"):
        full_path = "res://" + full_path

    if action == "create":
        var resource_type = params.get("resourceType", "")
        if resource_type.is_empty():
            printerr("resourceType is required to create a resource")
            quit(1)
            return

        log_info("Creating resource of type " + resource_type + " at: " + full_path)

        if not ClassDB.class_exists(resource_type):
            printerr("Unknown resource type: " + resource_type)
            printerr("Must be a valid Godot class name (e.g., StandardMaterial3D, AudioStreamPlayer, Theme)")
            quit(1)
            return

        if not ClassDB.can_instantiate(resource_type):
            printerr("Cannot instantiate resource type: " + resource_type)
            quit(1)
            return

        var resource = ClassDB.instantiate(resource_type)
        if resource == null:
            printerr("Failed to instantiate resource of type: " + resource_type)
            quit(1)
            return

        if not resource is Resource:
            printerr("Type " + resource_type + " is not a Resource subclass")
            quit(1)
            return

        if not _apply_resource_properties(resource, params.get("properties", {})):
            printerr("Invalid property value: " + _decode_error)
            quit(1)
            return

        if not _save_resource(resource, full_path):
            return

        print("Resource created: " + full_path)
    elif action == "read":
        var resource = _load_resource(full_path)
        if resource == null:
            return
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
        print(JSON_START)
        print(JSON.stringify({"type": resource.get_class(), "path": project_relative(full_path), "properties": props}))
        print(JSON_END)
    elif action == "modify":
        var resource = _load_resource(full_path)
        if resource == null:
            return
        if not _apply_resource_properties(resource, params.get("properties", {})):
            printerr("Invalid property value: " + _decode_error)
            quit(1)
            return
        if not _save_resource(resource, full_path):
            return
        print("Resource modified: " + full_path)
    else:
        printerr("Unknown manage_resource action: " + action)
        quit(1)


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
# null after signalling failure, because quit() does not stop the current call.
func _load_resource(full_path):
    if not ResourceLoader.exists(full_path):
        printerr("Resource not found: " + full_path)
        quit(1)
        return null
    var resource = ResourceLoader.load(full_path)
    if resource == null:
        printerr("Failed to load resource: " + full_path)
        quit(1)
        return null
    return resource


# Save a resource, creating its directory first. Returns false after signalling
# failure so the caller can bail out instead of reporting a false success.
func _save_resource(resource, full_path) -> bool:
    var dir_path = full_path.get_base_dir()
    var dir_relative = dir_path.substr(6)  # Remove "res://"
    if not dir_relative.is_empty():
        var dir = DirAccess.open("res://")
        if dir and not dir.dir_exists(dir_relative):
            dir.make_dir_recursive(dir_relative)
    var save_error = ResourceSaver.save(resource, full_path)
    if save_error != OK:
        printerr("Failed to save resource: " + str(save_error))
        quit(1)
        return false
    return true


# Apply a caller's properties to a resource. Theme items need their value type
# inferred from the item path, so Themes go through the theme codec; every other
# resource uses the plain property decoder.
func _apply_resource_properties(resource, properties) -> bool:
    if resource is Theme:
        return _apply_theme_properties(resource, properties)
    return _apply_properties(resource, properties)


# Apply a caller's properties map to a Theme with type conversion. Theme items
# are keyed by composite names such as "Label/colors/font_color", so the value
# type is derived from the matching property entry when the item already exists
# and from the key's category when this call creates it. Returns false and
# leaves the decode error set when a value cannot be decoded.
func _apply_theme_properties(theme, properties) -> bool:
    for key in properties:
        var decoded = _convert_theme_property(theme, key, properties[key])
        if _decode_error != "":
            return false
        theme.set(key, decoded)
    return true

func _convert_theme_property(theme, key, value):
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
        return value
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
func manage_animation(params):
    var resource_path = params.get("resourcePath", "")
    var action = params.get("action", "read")
    var full_path = resource_path
    if not full_path.begins_with("res://"):
        full_path = "res://" + full_path

    if action == "read":
        _read_animation(full_path)
        return

    if action == "create":
        var created = Animation.new()
        if not _apply_animation_properties(created, params):
            printerr("Invalid animation property: " + _decode_error)
            quit(1)
            return
        if not _save_resource(created, full_path):
            return
        print("Animation created: " + full_path)
        return

    var animation = _load_animation(full_path)
    if animation == null:
        return

    if action == "modify":
        if not _apply_animation_properties(animation, params):
            printerr("Invalid animation property: " + _decode_error)
            quit(1)
            return
    elif action == "add":
        if not _add_animation_track(animation, params):
            printerr("Invalid animation track: " + _decode_error)
            quit(1)
            return
    elif action == "set":
        if not _set_animation_track(animation, params):
            printerr("Invalid animation track: " + _decode_error)
            quit(1)
            return
    elif action == "remove":
        if not _remove_animation_track(animation, params):
            printerr("Invalid animation track: " + _decode_error)
            quit(1)
            return
    else:
        printerr("Unknown manage_animation action: " + action)
        quit(1)
        return

    if not _save_resource(animation, full_path):
        return
    print("Animation " + action + " succeeded: " + full_path)

# Load an Animation resource with the shared not-found and type checks. Returns
# null after signalling failure, because quit() does not stop the current call.
func _load_animation(full_path):
    if not ResourceLoader.exists(full_path):
        printerr("Animation resource not found: " + full_path)
        quit(1)
        return null
    var resource = ResourceLoader.load(full_path)
    if resource == null:
        printerr("Failed to load animation: " + full_path)
        quit(1)
        return null
    if not (resource is Animation):
        printerr("Not an Animation resource: " + full_path)
        quit(1)
        return null
    return resource

# Apply the animation-level properties length, loopMode, and step when present.
func _apply_animation_properties(animation, params) -> bool:
    if params.has("length"):
        var length = params["length"]
        if not (length is int or length is float):
            _decode_error = "length must be a number"
            return false
        animation.length = float(length)
    if params.has("loopMode"):
        var mode = str(params["loopMode"])
        if not ANIMATION_LOOP_MODES.has(mode):
            _decode_error = "unknown loopMode: " + mode
            return false
        animation.loop_mode = ANIMATION_LOOP_MODES[mode]
    if params.has("step"):
        var step = params["step"]
        if not (step is int or step is float):
            _decode_error = "step must be a number"
            return false
        animation.step = float(step)
    return true

# Append a track built from the caller's trackType, trackPath, interpolation,
# and keys.
func _add_animation_track(animation, params) -> bool:
    var type_id = _track_type_id(params.get("trackType", ""))
    if type_id == -1:
        _decode_error = "unknown trackType: " + str(params.get("trackType", ""))
        return false
    var track_index = animation.add_track(type_id)
    return _configure_animation_track(animation, track_index, params)

# Update an existing track selected by trackIndex.
func _set_animation_track(animation, params) -> bool:
    var track_index = _track_index(animation, params)
    if track_index < 0:
        return false
    return _configure_animation_track(animation, track_index, params)

# Configure a track's path, interpolation, loop wrap, enabled flag, and keys.
# A keys list replaces the track's existing keys wholesale, so set is
# deterministic rather than appending to whatever was there.
func _configure_animation_track(animation, track_index, params) -> bool:
    if params.has("trackPath"):
        animation.track_set_path(track_index, NodePath(str(params["trackPath"])))
    if params.has("interpolation"):
        var name = str(params["interpolation"])
        if not ANIMATION_INTERPOLATIONS.has(name):
            _decode_error = "unknown interpolation: " + name
            return false
        animation.track_set_interpolation_type(track_index, ANIMATION_INTERPOLATIONS[name])
    if params.has("loopWrap"):
        animation.track_set_interpolation_loop_wrap(track_index, bool(params["loopWrap"]))
    if params.has("enabled"):
        animation.track_set_enabled(track_index, bool(params["enabled"]))
    if params.has("keys"):
        return _insert_animation_keys(animation, track_index, params["keys"])
    return true

# Remove the track selected by trackIndex.
func _remove_animation_track(animation, params) -> bool:
    var track_index = _track_index(animation, params)
    if track_index < 0:
        return false
    animation.remove_track(track_index)
    return true

# Resolve and bounds-check the caller's trackIndex.
func _track_index(animation, params) -> int:
    if not params.has("trackIndex"):
        _decode_error = "trackIndex is required"
        return -1
    var track_index = int(params["trackIndex"])
    if track_index < 0 or track_index >= animation.get_track_count():
        _decode_error = "trackIndex out of range: " + str(track_index)
        return -1
    return track_index

# Replace a track's keys with the caller's list, then insert each one.
func _insert_animation_keys(animation, track_index, keys) -> bool:
    if not (keys is Array):
        _decode_error = "keys must be an array"
        return false
    var type_id = animation.track_get_type(track_index)
    while animation.track_get_key_count(track_index) > 0:
        animation.track_remove_key(track_index, 0)
    for key in keys:
        if not (key is Dictionary) or not key.has("time"):
            _decode_error = "each key needs a \"time\" number"
            return false
        var time = key["time"]
        if not (time is int or time is float):
            _decode_error = "each key needs a \"time\" number"
            return false
        if not _insert_animation_key(animation, track_index, type_id, float(time), key):
            return false
    return true

# Insert one key, decoding its value for the track's type. Value tracks take a
# value in the tool value contract, 3D tracks take the matching tagged vector,
# method tracks take {method, args}, and bezier tracks take a number plus
# optional inHandle/outHandle offsets.
func _insert_animation_key(animation, track_index, type_id, time, key) -> bool:
    var value = key.get("value", null)
    if type_id == Animation.TYPE_VALUE:
        var decoded = _decode_value(value)
        if _decode_error != "":
            return false
        animation.track_insert_key(track_index, time, decoded, _animation_number_default(key.get("transition", 1.0), "transition"))
    elif type_id == Animation.TYPE_POSITION_3D:
        var position = _require_tagged_type(value, TYPE_VECTOR3, "a position key value")
        if _decode_error != "":
            return false
        animation.position_track_insert_key(track_index, time, position)
    elif type_id == Animation.TYPE_ROTATION_3D:
        var rotation = _require_tagged_type(value, TYPE_QUATERNION, "a rotation key value")
        if _decode_error != "":
            return false
        animation.rotation_track_insert_key(track_index, time, rotation)
    elif type_id == Animation.TYPE_SCALE_3D:
        var scale = _require_tagged_type(value, TYPE_VECTOR3, "a scale key value")
        if _decode_error != "":
            return false
        animation.scale_track_insert_key(track_index, time, scale)
    elif type_id == Animation.TYPE_BLEND_SHAPE:
        animation.blend_shape_track_insert_key(track_index, time, _animation_number(value, "a blend shape key value"))
    elif type_id == Animation.TYPE_METHOD:
        animation.track_insert_key(track_index, time, _decode_method_key(value))
    elif type_id == Animation.TYPE_BEZIER:
        var bezier_value = _animation_number(value, "a bezier key value")
        if _decode_error != "":
            return false
        var in_handle = _animation_vector2_or_zero(key.get("inHandle", null))
        if _decode_error != "":
            return false
        var out_handle = _animation_vector2_or_zero(key.get("outHandle", null))
        if _decode_error != "":
            return false
        animation.bezier_track_insert_key(track_index, time, bezier_value, in_handle, out_handle)
    elif type_id == Animation.TYPE_AUDIO:
        var stream = _animation_resource_value(value)
        if _decode_error != "":
            return false
        animation.audio_track_insert_key(track_index, time, stream, _animation_number_default(key.get("startOffset", 0.0), "startOffset"), _animation_number_default(key.get("endOffset", 0.0), "endOffset"))
    elif type_id == Animation.TYPE_ANIMATION:
        var nested = _animation_resource_value(value)
        if _decode_error != "":
            return false
        animation.animation_track_insert_key(track_index, time, nested)
    else:
        _decode_error = "unsupported track type"
        return false
    return _decode_error == ""

# Decode a method-track key value, an object with a "method" string and an
# optional "args" array.
func _decode_method_key(value):
    if not (value is Dictionary) or not (value.get("method", null) is String):
        _decode_error = "a method key value must be an object with a \"method\" string"
        return null
    var args = []
    if value.has("args"):
        var decoded = _decode_value(value["args"])
        if _decode_error != "":
            return null
        if not (decoded is Array):
            _decode_error = "a method key's \"args\" must be an array"
            return null
        args = decoded
    return {"method": StringName(value["method"]), "args": args}

# Decode a value that must be a tagged object of the given Variant type.
func _require_tagged_type(value, type_id, label):
    if not (value is Dictionary) or not value.has("$type"):
        _decode_error = "expected a tagged {\"$type\": ...} value for " + label
        return null
    var decoded = _decode_typed(value)
    if _decode_error != "":
        return null
    if typeof(decoded) != type_id:
        _decode_error = label + " must be a " + type_string(type_id)
        return null
    return decoded

# Decode a Vector2 handle, defaulting to zero when the caller omits it.
func _animation_vector2_or_zero(value) -> Vector2:
    if value == null:
        return Vector2.ZERO
    var decoded = _require_tagged_type(value, TYPE_VECTOR2, "a bezier handle")
    if _decode_error != "":
        return Vector2.ZERO
    return decoded

# Read a number, reporting through the shared decode error.
func _animation_number(value, label) -> float:
    if value is int or value is float:
        return float(value)
    _decode_error = label + " must be a number"
    return 0.0

# Read a number that has a default when the caller omits the field.
func _animation_number_default(value, label) -> float:
    return _animation_number(value, label)

# Decode a resource reference for an audio or animation track value: a res://
# path or a tagged Resource.
func _animation_resource_value(value):
    if value is String:
        return _decode_resource_path(value)
    if value is Dictionary and value.has("$type"):
        return _decode_typed(value)
    _decode_error = "expected a res:// path or tagged Resource for this track's value"
    return null

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
func _read_animation(full_path):
    var animation = _load_animation(full_path)
    if animation == null:
        return
    var tracks = []
    for index in range(animation.get_track_count()):
        tracks.append(_animation_track_to_json(animation, index))
    var result = {
        "path": project_relative(full_path),
        "length": animation.length,
        "loopMode": _loop_mode_name(animation.loop_mode),
        "step": animation.step,
        "tracks": tracks,
    }
    print(JSON_START)
    print(JSON.stringify(result))
    print(JSON_END)

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
func manage_tileset(params):
    var resource_path = params.get("resourcePath", "")
    var action = params.get("action", "read")
    var full_path = resource_path
    if not full_path.begins_with("res://"):
        full_path = "res://" + full_path

    if action == "read":
        _read_tileset(full_path)
        return

    if action == "create":
        var created = TileSet.new()
        if not _apply_tileset_properties(created, params):
            printerr("Invalid tileset property: " + _decode_error)
            quit(1)
            return
        if not _save_resource(created, full_path):
            return
        print("TileSet created: " + full_path)
        return

    var tile_set = _load_tileset(full_path)
    if tile_set == null:
        return

    match action:
        "modify":
            if not _apply_tileset_properties(tile_set, params):
                printerr("Invalid tileset property: " + _decode_error)
                quit(1)
                return
        "addSource":
            if not _add_tileset_source(tile_set, params):
                printerr("Invalid tileset source: " + _decode_error)
                quit(1)
                return
        "removeSource":
            if not _remove_tileset_source(tile_set, params):
                printerr("Invalid tileset source: " + _decode_error)
                quit(1)
                return
        "addTile":
            if not _add_tileset_tile(tile_set, params):
                printerr("Invalid tileset tile: " + _decode_error)
                quit(1)
                return
        "removeTile":
            if not _remove_tileset_tile(tile_set, params):
                printerr("Invalid tileset tile: " + _decode_error)
                quit(1)
                return
        "addAlternative":
            if not _add_tile_alternative(tile_set, params):
                printerr("Invalid tile alternative: " + _decode_error)
                quit(1)
                return
        "removeAlternative":
            if not _remove_tile_alternative(tile_set, params):
                printerr("Invalid tile alternative: " + _decode_error)
                quit(1)
                return
        "setTileData":
            if not _set_tile_data(tile_set, params):
                printerr("Invalid tile data: " + _decode_error)
                quit(1)
                return
        _:
            printerr("Unknown manage_tileset action: " + action)
            quit(1)
            return

    if not _save_resource(tile_set, full_path):
        return
    print("TileSet " + action + " succeeded: " + full_path)

# Load a TileSet with the shared not-found and type checks. Returns null after
# signalling failure, because quit() does not stop the current call.
func _load_tileset(full_path):
    if not ResourceLoader.exists(full_path):
        printerr("TileSet resource not found: " + full_path)
        quit(1)
        return null
    var resource = ResourceLoader.load(full_path)
    if resource == null:
        printerr("Failed to load tileset: " + full_path)
        quit(1)
        return null
    if not (resource is TileSet):
        printerr("Not a TileSet resource: " + full_path)
        quit(1)
        return null
    return resource

# Apply the tileset-level properties tileSize, tileShape, tileLayout,
# tileOffsetAxis, and uvClipping when present.
func _apply_tileset_properties(tile_set, params) -> bool:
    if params.has("tileSize"):
        var size = _require_tagged_type(params["tileSize"], TYPE_VECTOR2I, "tileSize")
        if _decode_error != "":
            return false
        tile_set.tile_size = size
    if params.has("tileShape"):
        var shape = str(params["tileShape"])
        if not TILESET_SHAPES.has(shape):
            _decode_error = "unknown tileShape: " + shape
            return false
        tile_set.tile_shape = TILESET_SHAPES[shape]
    if params.has("tileLayout"):
        var layout = str(params["tileLayout"])
        if not TILESET_LAYOUTS.has(layout):
            _decode_error = "unknown tileLayout: " + layout
            return false
        tile_set.tile_layout = TILESET_LAYOUTS[layout]
    if params.has("tileOffsetAxis"):
        var axis = str(params["tileOffsetAxis"])
        if not TILESET_OFFSET_AXES.has(axis):
            _decode_error = "unknown tileOffsetAxis: " + axis
            return false
        tile_set.tile_offset_axis = TILESET_OFFSET_AXES[axis]
    if params.has("uvClipping"):
        tile_set.uv_clipping = bool(params["uvClipping"])
    return true

# Add an atlas or scenes-collection source to the tileset. An optional sourceId
# overrides the engine-assigned id; the chosen id is printed for the caller.
func _add_tileset_source(tile_set, params) -> bool:
    var source_type = str(params.get("sourceType", ""))
    var source = null
    if source_type == "atlas":
        source = TileSetAtlasSource.new()
        if params.has("texture"):
            var texture_value = params["texture"]
            if texture_value is String and not texture_value.begins_with("res://") and not texture_value.begins_with("user://"):
                texture_value = "res://" + texture_value
            var texture = _decode_object(texture_value)
            if _decode_error != "":
                return false
            if not (texture is Texture2D):
                _decode_error = "texture must be a Texture2D resource"
                return false
            source.texture = texture
        if params.has("margins"):
            var margins = _require_tagged_type(params["margins"], TYPE_VECTOR2I, "margins")
            if _decode_error != "":
                return false
            source.margins = margins
        if params.has("separation"):
            var separation = _require_tagged_type(params["separation"], TYPE_VECTOR2I, "separation")
            if _decode_error != "":
                return false
            source.separation = separation
        if params.has("textureRegionSize"):
            var region = _require_tagged_type(params["textureRegionSize"], TYPE_VECTOR2I, "textureRegionSize")
            if _decode_error != "":
                return false
            source.texture_region_size = region
        if params.has("useTexturePadding"):
            source.use_texture_padding = bool(params["useTexturePadding"])
    elif source_type == "scenes":
        source = TileSetScenesCollectionSource.new()
    else:
        _decode_error = "sourceType must be atlas or scenes"
        return false
    var override_id = -1
    if params.has("sourceId"):
        override_id = int(params["sourceId"])
    var source_id = tile_set.add_source(source, override_id)
    print("TileSet source added: " + str(source_id))
    return true

# Remove the source selected by sourceId.
func _remove_tileset_source(tile_set, params) -> bool:
    var source_id = _require_source_id(tile_set, params)
    if _decode_error != "":
        return false
    tile_set.remove_source(source_id)
    return true

# Resolve and check the caller's sourceId against the tileset.
func _require_source_id(tile_set, params) -> int:
    if not params.has("sourceId"):
        _decode_error = "sourceId is required"
        return -1
    var source_id = int(params["sourceId"])
    if not tile_set.has_source(source_id):
        _decode_error = "unknown sourceId: " + str(source_id)
        return -1
    return source_id

# Resolve a sourceId and require it to name an atlas source.
func _require_atlas_source(tile_set, params):
    var source_id = _require_source_id(tile_set, params)
    if _decode_error != "":
        return null
    var source = tile_set.get_source(source_id)
    if not (source is TileSetAtlasSource):
        _decode_error = "source " + str(source_id) + " is not an atlas source"
        return null
    return source

# Add a tile. An atlas source takes atlasCoords (and optional sizeInAtlas); a
# scenes-collection source takes the scenePath of the PackedScene to place.
func _add_tileset_tile(tile_set, params) -> bool:
    var source_id = _require_source_id(tile_set, params)
    if _decode_error != "":
        return false
    var source = tile_set.get_source(source_id)
    if source is TileSetAtlasSource:
        var coords = _require_tagged_type(params.get("atlasCoords"), TYPE_VECTOR2I, "atlasCoords")
        if _decode_error != "":
            return false
        if source.has_tile(coords):
            _decode_error = "a tile already exists at " + str(coords)
            return false
        var size = Vector2i.ONE
        if params.has("sizeInAtlas"):
            size = _require_tagged_type(params["sizeInAtlas"], TYPE_VECTOR2I, "sizeInAtlas")
            if _decode_error != "":
                return false
        source.create_tile(coords, size)
        return true
    if source is TileSetScenesCollectionSource:
        var scene_path = str(params.get("scenePath", ""))
        if scene_path.is_empty():
            _decode_error = "scenePath is required to add a scene tile"
            return false
        if not scene_path.begins_with("res://"):
            scene_path = "res://" + scene_path
        var packed = load(scene_path)
        if not (packed is PackedScene):
            _decode_error = "scenePath must name a PackedScene: " + scene_path
            return false
        var override_id = -1
        if params.has("sceneTileId"):
            override_id = int(params["sceneTileId"])
        var tile_id = source.create_scene_tile(packed, override_id)
        if params.has("displayPlaceholder"):
            source.set_scene_tile_display_placeholder(tile_id, bool(params["displayPlaceholder"]))
        return true
    _decode_error = "source " + str(source_id) + " has no tiles"
    return false

# Remove a tile. An atlas source is addressed by atlasCoords, a scenes-collection
# source by sceneTileId.
func _remove_tileset_tile(tile_set, params) -> bool:
    var source_id = _require_source_id(tile_set, params)
    if _decode_error != "":
        return false
    var source = tile_set.get_source(source_id)
    if source is TileSetAtlasSource:
        var coords = _require_tagged_type(params.get("atlasCoords"), TYPE_VECTOR2I, "atlasCoords")
        if _decode_error != "":
            return false
        if not source.has_tile(coords):
            _decode_error = "no tile at " + str(coords)
            return false
        source.remove_tile(coords)
        return true
    if source is TileSetScenesCollectionSource:
        if not params.has("sceneTileId"):
            _decode_error = "sceneTileId is required to remove a scene tile"
            return false
        var tile_id = int(params["sceneTileId"])
        if not source.has_scene_tile_id(tile_id):
            _decode_error = "unknown sceneTileId: " + str(tile_id)
            return false
        source.remove_scene_tile(tile_id)
        return true
    _decode_error = "source " + str(source_id) + " has no tiles"
    return false

# Add an alternative tile to the atlas tile at atlasCoords.
func _add_tile_alternative(tile_set, params) -> bool:
    var atlas = _require_atlas_source(tile_set, params)
    if _decode_error != "":
        return false
    var coords = _require_tagged_type(params.get("atlasCoords"), TYPE_VECTOR2I, "atlasCoords")
    if _decode_error != "":
        return false
    if not atlas.has_tile(coords):
        _decode_error = "no tile at " + str(coords)
        return false
    var override_id = -1
    if params.has("alternativeId"):
        override_id = int(params["alternativeId"])
    var alternative_id = atlas.create_alternative_tile(coords, override_id)
    print("Tile alternative added: " + str(alternative_id))
    return true

# Remove the alternative tile selected by alternativeId.
func _remove_tile_alternative(tile_set, params) -> bool:
    var atlas = _require_atlas_source(tile_set, params)
    if _decode_error != "":
        return false
    var coords = _require_tagged_type(params.get("atlasCoords"), TYPE_VECTOR2I, "atlasCoords")
    if _decode_error != "":
        return false
    if not params.has("alternativeId"):
        _decode_error = "alternativeId is required"
        return false
    atlas.remove_alternative_tile(coords, int(params["alternativeId"]))
    return true

# Apply caller-supplied properties to the TileData of the tile at atlasCoords.
# Alt 0 is the tile's base data; pass alternativeId to target another one.
func _set_tile_data(tile_set, params) -> bool:
    var atlas = _require_atlas_source(tile_set, params)
    if _decode_error != "":
        return false
    var coords = _require_tagged_type(params.get("atlasCoords"), TYPE_VECTOR2I, "atlasCoords")
    if _decode_error != "":
        return false
    if not atlas.has_tile(coords):
        _decode_error = "no tile at " + str(coords)
        return false
    var alternative_id = 0
    if params.has("alternativeId"):
        alternative_id = int(params["alternativeId"])
    var tile_data = atlas.get_tile_data(coords, alternative_id)
    if tile_data == null:
        _decode_error = "no tile data for alternative " + str(alternative_id) + " at " + str(coords)
        return false
    if params.has("properties"):
        if not _apply_properties(tile_data, params["properties"]):
            return false
    return true

# Emit a TileSet as a JSON payload: the tileset-level settings and one object
# per source. An atlas source lists its tiles and each tile's alternatives.
func _read_tileset(full_path):
    var tile_set = _load_tileset(full_path)
    if tile_set == null:
        return
    var sources = []
    for index in range(tile_set.get_source_count()):
        sources.append(_tileset_source_to_json(tile_set, tile_set.get_source_id(index)))
    var result = {
        "path": project_relative(full_path),
        "tileSize": _variant_to_json(tile_set.tile_size),
        "tileShape": _enum_name(TILESET_SHAPES, tile_set.tile_shape),
        "tileLayout": _enum_name(TILESET_LAYOUTS, tile_set.tile_layout),
        "tileOffsetAxis": _enum_name(TILESET_OFFSET_AXES, tile_set.tile_offset_axis),
        "uvClipping": tile_set.uv_clipping,
        "sources": sources,
    }
    print(JSON_START)
    print(JSON.stringify(result))
    print(JSON_END)

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
func manage_theme(params):
    var resource_path = params.get("resourcePath", "")
    var action = params.get("action", "read")
    var full_path = resource_path
    if not full_path.begins_with("res://"):
        full_path = "res://" + full_path

    if action == "read":
        _read_theme(full_path)
        return

    if action == "create":
        var created = Theme.new()
        if not _apply_theme_defaults(created, params):
            printerr("Invalid theme property: " + _decode_error)
            quit(1)
            return
        if not _save_resource(created, full_path):
            return
        print("Theme created: " + full_path)
        return

    var theme = _load_theme(full_path)
    if theme == null:
        return

    match action:
        "modify":
            if not _apply_theme_defaults(theme, params):
                printerr("Invalid theme property: " + _decode_error)
                quit(1)
                return
        "removeType":
            theme.remove_type(StringName(str(params.get("themeType", ""))))
        "setTypeVariation":
            theme.set_type_variation(StringName(str(params.get("themeType", ""))), StringName(str(params.get("baseType", ""))))
        "clearTypeVariation":
            theme.clear_type_variation(StringName(str(params.get("themeType", ""))))
        "setItem":
            if not _set_theme_item(theme, params):
                printerr("Invalid theme item: " + _decode_error)
                quit(1)
                return
        "clearItem":
            if not _clear_theme_item(theme, params):
                printerr("Invalid theme item: " + _decode_error)
                quit(1)
                return
        _:
            printerr("Unknown manage_theme action: " + action)
            quit(1)
            return

    if not _save_resource(theme, full_path):
        return
    print("Theme " + action + " succeeded: " + full_path)

# Load a Theme with the shared not-found and type checks. Returns null after
# signalling failure, because quit() does not stop the current call.
func _load_theme(full_path):
    if not ResourceLoader.exists(full_path):
        printerr("Theme resource not found: " + full_path)
        quit(1)
        return null
    var resource = ResourceLoader.load(full_path)
    if resource == null:
        printerr("Failed to load theme: " + full_path)
        quit(1)
        return null
    if not (resource is Theme):
        printerr("Not a Theme resource: " + full_path)
        quit(1)
        return null
    return resource

# Apply the theme-level defaults defaultFont, defaultFontSize, and
# defaultBaseScale when present.
func _apply_theme_defaults(theme, params) -> bool:
    if params.has("defaultFont"):
        var value = params["defaultFont"]
        if value is String and not value.begins_with("res://") and not value.begins_with("user://"):
            value = "res://" + value
        var font = _decode_object(value)
        if _decode_error != "":
            return false
        if not (font is Font):
            _decode_error = "defaultFont must be a Font resource"
            return false
        theme.default_font = font
    if params.has("defaultFontSize"):
        var size = params["defaultFontSize"]
        if not (size is int or size is float):
            _decode_error = "defaultFontSize must be a number"
            return false
        theme.default_font_size = int(size)
    if params.has("defaultBaseScale"):
        var scale = params["defaultBaseScale"]
        if not (scale is int or scale is float):
            _decode_error = "defaultBaseScale must be a number"
            return false
        theme.default_base_scale = float(scale)
    return true

# Set one theme item, converting the value to the item category's engine type.
func _set_theme_item(theme, params) -> bool:
    var type_id = _theme_item_type_id(params.get("itemType", ""))
    if type_id == -1:
        _decode_error = "unknown itemType: " + str(params.get("itemType", ""))
        return false
    var value = _convert_theme_item_value(type_id, params.get("value", null))
    if _decode_error != "":
        return false
    theme.set_theme_item(type_id, StringName(str(params.get("itemName", ""))), StringName(str(params.get("themeType", ""))), value)
    return true

# Remove one theme item from a type.
func _clear_theme_item(theme, params) -> bool:
    var type_id = _theme_item_type_id(params.get("itemType", ""))
    if type_id == -1:
        _decode_error = "unknown itemType: " + str(params.get("itemType", ""))
        return false
    theme.clear_theme_item(type_id, StringName(str(params.get("itemName", ""))), StringName(str(params.get("themeType", ""))))
    return true

# Convert a caller's value to the engine type its item category requires: a
# tagged Color, an integer for constant/fontSize, or a resource for
# font/icon/stylebox.
func _convert_theme_item_value(type_id, value):
    match type_id:
        Theme.DATA_TYPE_COLOR:
            return _require_tagged_type(value, TYPE_COLOR, "a color item value")
        Theme.DATA_TYPE_CONSTANT, Theme.DATA_TYPE_FONT_SIZE:
            return _convert_value_for_type(TYPE_INT, value)
        Theme.DATA_TYPE_FONT, Theme.DATA_TYPE_ICON, Theme.DATA_TYPE_STYLEBOX:
            if value is String and not value.begins_with("res://") and not value.begins_with("user://"):
                value = "res://" + value
            return _convert_value_for_type(TYPE_OBJECT, value)
    _decode_error = "unsupported item type"
    return null

func _theme_item_type_id(name) -> int:
    if THEME_ITEM_TYPES.has(name):
        return THEME_ITEM_TYPES[name]
    return -1

# Emit a Theme as a JSON payload: the theme-level defaults and one object per
# type with its items grouped by category.
func _read_theme(full_path):
    var theme = _load_theme(full_path)
    if theme == null:
        return
    var types = []
    for type_name in theme.get_type_list():
        types.append(_theme_type_to_json(theme, type_name))
    var result = {
        "path": project_relative(full_path),
        "defaultFont": _variant_to_json(theme.default_font),
        "defaultFontSize": theme.default_font_size if theme.has_default_font_size() else null,
        "defaultBaseScale": theme.default_base_scale if theme.has_default_base_scale() else null,
        "types": types,
    }
    print(JSON_START)
    print(JSON.stringify(result))
    print(JSON_END)

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
func manage_tilemap(params):
    var scene_path = params.get("scenePath", "")
    var action = params.get("action", "read")
    var node_path = str(params.get("nodePath", ""))
    var full_path = scene_path
    if not full_path.begins_with("res://"):
        full_path = "res://" + full_path

    if not FileAccess.file_exists(full_path):
        printerr("Scene file does not exist at: " + full_path)
        quit(1)
        return
    var scene = load(full_path)
    if not (scene is PackedScene):
        printerr("Failed to load scene: " + full_path)
        quit(1)
        return
    var scene_root = track_temp(scene.instantiate())
    var target = _resolve_scene_node(scene_root, node_path)
    if target == null:
        printerr("Node not found: " + node_path)
        quit(1)
        return
    if not (target is TileMapLayer):
        printerr("Node is not a TileMapLayer: " + node_path)
        quit(1)
        return

    if action == "read":
        _read_tilemap_node(target, full_path, node_path)
        return

    match action:
        "setCell":
            if not _set_tilemap_cell(target, params):
                printerr("Invalid tilemap cell: " + _decode_error)
                quit(1)
                return
        "eraseCell":
            var coords = _require_tagged_type(params.get("coords"), TYPE_VECTOR2I, "coords")
            if _decode_error != "":
                printerr("Invalid tilemap cell: " + _decode_error)
                quit(1)
                return
            target.erase_cell(coords)
        "clear":
            target.clear()
        _:
            printerr("Unknown manage_tilemap action: " + action)
            quit(1)
            return

    var packed_scene = PackedScene.new()
    var result = packed_scene.pack(scene_root)
    if result != OK:
        printerr("Failed to pack scene after tilemap edit: " + str(result))
        quit(1)
        return
    var save_error = ResourceSaver.save(packed_scene, full_path)
    if save_error != OK:
        printerr("Failed to save tilemap scene: " + str(save_error))
        quit(1)
        return
    print("TileMap " + action + " succeeded: " + full_path)

# Place one cell, checking that the node has a tileset that owns sourceId.
func _set_tilemap_cell(target, params) -> bool:
    var coords = _require_tagged_type(params.get("coords"), TYPE_VECTOR2I, "coords")
    if _decode_error != "":
        return false
    var atlas_coords = _require_tagged_type(params.get("atlasCoords"), TYPE_VECTOR2I, "atlasCoords")
    if _decode_error != "":
        return false
    if not params.has("sourceId"):
        _decode_error = "sourceId is required"
        return false
    var source_id = int(params["sourceId"])
    if target.tile_set == null:
        _decode_error = "the node has no tileSet"
        return false
    if not target.tile_set.has_source(source_id):
        _decode_error = "unknown sourceId: " + str(source_id)
        return false
    var alternative_id = 0
    if params.has("alternativeTile"):
        alternative_id = int(params["alternativeTile"])
    target.set_cell(coords, source_id, atlas_coords, alternative_id)
    return true

# Emit a TileMapLayer's tileset path and used cells, sorted for stable output.
func _read_tilemap_node(target, full_path, node_path):
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
    var result = {
        "scenePath": project_relative(full_path),
        "nodePath": node_path,
        "tileSet": tile_set_path,
        "usedCells": cells,
    }
    print(JSON_START)
    print(JSON.stringify(result))
    print(JSON_END)

# Validate that Godot itself loaded project.godot cleanly. ProjectSettings
# parses project.godot during engine startup, before _init runs; when the file
# is malformed the engine logs "Error parsing '<path>/project.godot'" to stderr
# and leaves the affected settings at their defaults. Starting the engine
# against the project is the whole check, and the caller combines that stderr
# diagnostic with the exit code, so this operation deliberately does no work of
# its own and emits no payload for anyone to read.
func validate_project(_params):
    pass
