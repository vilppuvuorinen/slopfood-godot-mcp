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

# Create a new scene with a specified root node type
func create_scene(params):
    print("Creating scene: " + params.scenePath)
    
    # Normalize the scene path
    var full_scene_path = params.scenePath
    if not full_scene_path.begins_with("res://"):
        full_scene_path = "res://" + full_scene_path
    # Convert resource path to an absolute path
    var absolute_scene_path = ProjectSettings.globalize_path(full_scene_path)
    # Get the scene directory paths
    var scene_dir_res = full_scene_path.get_base_dir()
    var scene_dir_abs = absolute_scene_path.get_base_dir()
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
        # Ensure the scene directory exists using DirAccess
        # Get the scene directory relative to res://
        var scene_dir_relative = scene_dir_res.substr(6)  # Remove "res://" prefix
        # Create the directory if needed
        if not scene_dir_relative.is_empty():
            # First check if it exists
            var dir_exists = DirAccess.dir_exists_absolute(scene_dir_abs)
            if not dir_exists:
                # Try to create the directory using DirAccess
                var dir = DirAccess.open("res://")
                if dir == null:
                    var open_error = DirAccess.get_open_error()
                    printerr("Failed to open res:// directory: " + str(open_error))
                    
                    # Try alternative approach with absolute path
                    var make_dir_error = DirAccess.make_dir_recursive_absolute(scene_dir_abs)
                    if make_dir_error != OK:
                        printerr("Failed to create directory using absolute path")
                        printerr("Error code: " + str(make_dir_error))
                        quit(1)
                else:
                    # Create the directory using the DirAccess instance
                    var make_dir_error = dir.make_dir_recursive(scene_dir_relative)
                    if make_dir_error != OK:
                        printerr("Failed to create directory: " + scene_dir_relative)
                        printerr("Error code: " + str(make_dir_error))
                        quit(1)
                
                # Verify the directory was created
                dir_exists = DirAccess.dir_exists_absolute(scene_dir_abs)
                if not dir_exists:
                    printerr("Directory reported as created but does not exist: " + scene_dir_abs)
                    printerr("This may indicate a problem with path resolution or permissions")
                    quit(1)
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
    var new_node = track_temp(instantiate_class(params.nodeType))
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
            print("Node '" + params.nodeName + "' of type '" + params.nodeType + "' added successfully")
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

    print(JSON_START)
    print(JSON.stringify(tree_data))
    print(JSON_END)

    scene_root.queue_free()

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

# Validate that Godot itself loaded project.godot cleanly. ProjectSettings
# parses project.godot during engine startup, before _init runs; when the file
# is malformed the engine logs "Error parsing '<path>/project.godot'" to stderr
# and leaves the affected settings at their defaults. Starting the engine
# against the project is the whole check, and the caller combines that stderr
# diagnostic with the exit code, so this operation deliberately does no work of
# its own and emits no payload for anyone to read.
func validate_project(_params):
    pass
