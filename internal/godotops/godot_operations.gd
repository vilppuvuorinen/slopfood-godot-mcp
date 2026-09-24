#!/usr/bin/env -S godot --headless --script
# Adapted from tugcantopaloglu/godot-mcp (src/scripts/godot_operations.gd), a
# fork of Coding-Solo/godot-mcp (both MIT-licensed); most of this file is
# copied verbatim from those projects.
extends SceneTree

# Debug mode flag
var debug_mode = false

const JSON_START = "<<<GO_DOT_MCP_JSON_START>>>"
const JSON_END = "<<<GO_DOT_MCP_JSON_END>>>"

func _init():
    var args = OS.get_cmdline_args()
    
    # Check for debug flag
    debug_mode = "--debug-godot" in args
    
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
    
    # Log all arguments for debugging
    log_debug("All arguments: " + str(args))
    log_debug("Script index: " + str(script_index))
    log_debug("Operation index: " + str(operation_index))
    log_debug("Params index: " + str(params_index))
    
    var operation = args[operation_index]
    var params_json = args[params_index]
    
    log_info("Operation: " + operation)
    log_debug("Params JSON: " + params_json)
    
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
    
    match operation:
        "create_scene":
            create_scene(params)
        "add_node":
            add_node(params)
        "load_sprite":
            load_sprite(params)
        "export_mesh_library":
            export_mesh_library(params)
        "save_scene":
            save_scene(params)
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
        "create_resource":
            create_resource(params)
        "manage_resource":
            manage_resource(params)
        "manage_theme_resource":
            manage_theme_resource(params)
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
func log_debug(message):
    if debug_mode:
        print("[DEBUG] " + message)

func log_info(message):
    print("[INFO] " + message)

func log_error(message):
    printerr("[ERROR] " + message)

# Get a script by registered class name.
# Only looks up names via the project's global class registry. Raw paths
# (e.g. "res://evil.gd") are intentionally not accepted here to prevent
# arbitrary script instantiation from agent-supplied input.
func get_script_by_name(name_of_class):
    if debug_mode:
        print("Attempting to get script for class: " + name_of_class)

    # Search for it in the global class registry if it's a class name
    var global_classes = ProjectSettings.get_global_class_list()
    if debug_mode:
        print("Searching through " + str(global_classes.size()) + " global classes")
    
    for global_class in global_classes:
        var found_name_of_class = global_class["class"]
        var found_path = global_class["path"]
        
        if found_name_of_class == name_of_class:
            if debug_mode:
                print("Found matching class in registry: " + found_name_of_class + " at path: " + found_path)
            var script = load(found_path) as Script
            if script:
                if debug_mode:
                    print("Successfully loaded script from registry")
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
    if debug_mode:
        print("Attempting to instantiate class: " + name_of_class)
    
    # Check if it's a built-in class
    if ClassDB.class_exists(name_of_class):
        if debug_mode:
            print("Class exists in ClassDB, using ClassDB.instantiate()")
        if ClassDB.can_instantiate(name_of_class):
            result = ClassDB.instantiate(name_of_class)
            if result == null:
                printerr("ClassDB.instantiate() returned null for class: " + name_of_class)
        else:
            printerr("Class exists but cannot be instantiated: " + name_of_class)
            printerr("This may be an abstract class or interface that cannot be directly instantiated")
    else:
        # Try to get the script
        if debug_mode:
            print("Class not found in ClassDB, trying to get script")
        var script = get_script_by_name(name_of_class)
        if script is GDScript:
            if debug_mode:
                print("Found GDScript, creating instance")
            result = script.new()
        else:
            printerr("Failed to get script for class: " + name_of_class)
            return null
    
    if result == null:
        printerr("Failed to instantiate class: " + name_of_class)
    elif debug_mode:
        print("Successfully instantiated class: " + name_of_class + " of type: " + result.get_class())
    
    return result

# Create a new scene with a specified root node type
func create_scene(params):
    print("Creating scene: " + params.scene_path)
    
    # Get project paths and log them for debugging
    var project_res_path = "res://"
    var project_user_path = "user://"
    var global_res_path = ProjectSettings.globalize_path(project_res_path)
    var global_user_path = ProjectSettings.globalize_path(project_user_path)
    
    if debug_mode:
        print("Project paths:")
        print("- res:// path: " + project_res_path)
        print("- user:// path: " + project_user_path)
        print("- Globalized res:// path: " + global_res_path)
        print("- Globalized user:// path: " + global_user_path)
        
        # Print some common environment variables for debugging
        print("Environment variables:")
        var env_vars = ["PATH", "HOME", "USER", "TEMP", "GODOT_PATH"]
        for env_var in env_vars:
            if OS.has_environment(env_var):
                print("  " + env_var + " = " + OS.get_environment(env_var))
    
    # Normalize the scene path
    var full_scene_path = params.scene_path
    if not full_scene_path.begins_with("res://"):
        full_scene_path = "res://" + full_scene_path
    if debug_mode:
        print("Scene path (with res://): " + full_scene_path)
    
    # Convert resource path to an absolute path
    var absolute_scene_path = ProjectSettings.globalize_path(full_scene_path)
    if debug_mode:
        print("Absolute scene path: " + absolute_scene_path)
    
    # Get the scene directory paths
    var scene_dir_res = full_scene_path.get_base_dir()
    var scene_dir_abs = absolute_scene_path.get_base_dir()
    if debug_mode:
        print("Scene directory (resource path): " + scene_dir_res)
        print("Scene directory (absolute path): " + scene_dir_abs)
    
    # Only do extensive testing in debug mode
    if debug_mode:
        # Try to create a simple test file in the project root to verify write access
        var initial_test_file_path = "res://godot_mcp_test_write.tmp"
        var initial_test_file = FileAccess.open(initial_test_file_path, FileAccess.WRITE)
        if initial_test_file:
            initial_test_file.store_string("Test write access")
            initial_test_file.close()
            print("Successfully wrote test file to project root: " + initial_test_file_path)
            
            # Verify the test file exists
            var initial_test_file_exists = FileAccess.file_exists(initial_test_file_path)
            print("Test file exists check: " + str(initial_test_file_exists))
            
            # Clean up the test file
            if initial_test_file_exists:
                var remove_error = DirAccess.remove_absolute(ProjectSettings.globalize_path(initial_test_file_path))
                print("Test file removal result: " + str(remove_error))
        else:
            var write_error = FileAccess.get_open_error()
            printerr("Failed to write test file to project root: " + str(write_error))
            printerr("This indicates a serious permission issue with the project directory")
    
    # Use traditional if-else statement for better compatibility
    var root_node_type = "Node2D"  # Default value
    if params.has("root_node_type"):
        root_node_type = params.root_node_type
    if debug_mode:
        print("Root node type: " + root_node_type)
    
    # Create the root node
    var scene_root = track_temp(instantiate_class(root_node_type))
    if not scene_root:
        printerr("Failed to instantiate node of type: " + root_node_type)
        printerr("Make sure the class exists and can be instantiated")
        printerr("Check if the class is registered in ClassDB or available as a script")
        quit(1)
    
    scene_root.name = "root"
    if debug_mode:
        print("Root node created with name: " + scene_root.name)
    
    # Pack the scene
    var packed_scene = PackedScene.new()
    var result = packed_scene.pack(scene_root)
    if debug_mode:
        print("Pack result: " + str(result) + " (OK=" + str(OK) + ")")
    
    if result == OK:
        # Only do extensive testing in debug mode
        if debug_mode:
            # First, let's verify we can write to the project directory
            print("Testing write access to project directory...")
            var test_write_path = "res://test_write_access.tmp"
            var test_write_abs = ProjectSettings.globalize_path(test_write_path)
            var test_file = FileAccess.open(test_write_path, FileAccess.WRITE)
            
            if test_file:
                test_file.store_string("Write test")
                test_file.close()
                print("Successfully wrote test file to project directory")
                
                # Clean up test file
                if FileAccess.file_exists(test_write_path):
                    var remove_error = DirAccess.remove_absolute(test_write_abs)
                    print("Test file removal result: " + str(remove_error))
            else:
                var write_error = FileAccess.get_open_error()
                printerr("Failed to write test file to project directory: " + str(write_error))
                printerr("This may indicate permission issues with the project directory")
                # Continue anyway, as the scene directory might still be writable
        
        # Ensure the scene directory exists using DirAccess
        if debug_mode:
            print("Ensuring scene directory exists...")
        
        # Get the scene directory relative to res://
        var scene_dir_relative = scene_dir_res.substr(6)  # Remove "res://" prefix
        if debug_mode:
            print("Scene directory (relative to res://): " + scene_dir_relative)
        
        # Create the directory if needed
        if not scene_dir_relative.is_empty():
            # First check if it exists
            var dir_exists = DirAccess.dir_exists_absolute(scene_dir_abs)
            if debug_mode:
                print("Directory exists check (absolute): " + str(dir_exists))
            
            if not dir_exists:
                if debug_mode:
                    print("Directory doesn't exist, creating: " + scene_dir_relative)
                
                # Try to create the directory using DirAccess
                var dir = DirAccess.open("res://")
                if dir == null:
                    var open_error = DirAccess.get_open_error()
                    printerr("Failed to open res:// directory: " + str(open_error))
                    
                    # Try alternative approach with absolute path
                    if debug_mode:
                        print("Trying alternative directory creation approach...")
                    var make_dir_error = DirAccess.make_dir_recursive_absolute(scene_dir_abs)
                    if debug_mode:
                        print("Make directory result (absolute): " + str(make_dir_error))
                    
                    if make_dir_error != OK:
                        printerr("Failed to create directory using absolute path")
                        printerr("Error code: " + str(make_dir_error))
                        quit(1)
                else:
                    # Create the directory using the DirAccess instance
                    if debug_mode:
                        print("Creating directory using DirAccess: " + scene_dir_relative)
                    var make_dir_error = dir.make_dir_recursive(scene_dir_relative)
                    if debug_mode:
                        print("Make directory result: " + str(make_dir_error))
                    
                    if make_dir_error != OK:
                        printerr("Failed to create directory: " + scene_dir_relative)
                        printerr("Error code: " + str(make_dir_error))
                        quit(1)
                
                # Verify the directory was created
                dir_exists = DirAccess.dir_exists_absolute(scene_dir_abs)
                if debug_mode:
                    print("Directory exists check after creation: " + str(dir_exists))
                
                if not dir_exists:
                    printerr("Directory reported as created but does not exist: " + scene_dir_abs)
                    printerr("This may indicate a problem with path resolution or permissions")
                    quit(1)
            elif debug_mode:
                print("Directory already exists: " + scene_dir_abs)
        
        # Save the scene
        if debug_mode:
            print("Saving scene to: " + full_scene_path)
        var save_error = ResourceSaver.save(packed_scene, full_scene_path)
        if debug_mode:
            print("Save result: " + str(save_error) + " (OK=" + str(OK) + ")")
        
        if save_error == OK:
            # Only do extensive testing in debug mode
            if debug_mode:
                # Wait a moment to ensure file system has time to complete the write
                print("Waiting for file system to complete write operation...")
                OS.delay_msec(500)  # 500ms delay
                
                # Verify the file was actually created using multiple methods
                var file_check_abs = FileAccess.file_exists(absolute_scene_path)
                print("File exists check (absolute path): " + str(file_check_abs))
                
                var file_check_res = FileAccess.file_exists(full_scene_path)
                print("File exists check (resource path): " + str(file_check_res))
                
                var res_exists = ResourceLoader.exists(full_scene_path)
                print("Resource exists check: " + str(res_exists))
                
                # If file doesn't exist by absolute path, try to create a test file in the same directory
                if not file_check_abs and not file_check_res:
                    printerr("Scene file not found after save. Trying to diagnose the issue...")
                    
                    # Try to write a test file to the same directory
                    var test_scene_file_path = scene_dir_res + "/test_scene_file.tmp"
                    var test_scene_file = FileAccess.open(test_scene_file_path, FileAccess.WRITE)
                    
                    if test_scene_file:
                        test_scene_file.store_string("Test scene directory write")
                        test_scene_file.close()
                        print("Successfully wrote test file to scene directory: " + test_scene_file_path)
                        
                        # Check if the test file exists
                        var test_file_exists = FileAccess.file_exists(test_scene_file_path)
                        print("Test file exists: " + str(test_file_exists))
                        
                        if test_file_exists:
                            # Directory is writable, so the issue is with scene saving
                            printerr("Directory is writable but scene file wasn't created.")
                            printerr("This suggests an issue with ResourceSaver.save() or the packed scene.")
                            
                            # Try saving with a different approach
                            print("Trying alternative save approach...")
                            var alt_save_error = ResourceSaver.save(packed_scene, test_scene_file_path + ".tscn")
                            print("Alternative save result: " + str(alt_save_error))
                            
                            # Clean up test files
                            DirAccess.remove_absolute(ProjectSettings.globalize_path(test_scene_file_path))
                            if alt_save_error == OK:
                                DirAccess.remove_absolute(ProjectSettings.globalize_path(test_scene_file_path + ".tscn"))
                        else:
                            printerr("Test file couldn't be verified. This suggests filesystem access issues.")
                    else:
                        var write_error = FileAccess.get_open_error()
                        printerr("Failed to write test file to scene directory: " + str(write_error))
                        printerr("This confirms there are permission or path issues with the scene directory.")
                    
                    # Return error since we couldn't create the scene file
                    printerr("Failed to create scene: " + params.scene_path)
                    quit(1)
                
                # If we get here, at least one of our file checks passed
                if file_check_abs or file_check_res or res_exists:
                    print("Scene file verified to exist!")
                    
                    # Try to load the scene to verify it's valid
                    var test_load = ResourceLoader.load(full_scene_path)
                    if test_load:
                        print("Scene created and verified successfully at: " + params.scene_path)
                        print("Scene file can be loaded correctly.")
                    else:
                        print("Scene file exists but cannot be loaded. It may be corrupted or incomplete.")
                        # Continue anyway since the file exists
                    
                    print("Scene created successfully at: " + params.scene_path)
                else:
                    printerr("All file existence checks failed despite successful save operation.")
                    printerr("This indicates a serious issue with file system access or path resolution.")
                    quit(1)
            else:
                # In non-debug mode, just check if the file exists
                var file_exists = FileAccess.file_exists(full_scene_path)
                if file_exists:
                    print("Scene created successfully at: " + params.scene_path)
                else:
                    printerr("Failed to create scene: " + params.scene_path)
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
    print("Adding node to scene: " + params.scene_path)
    
    var full_scene_path = params.scene_path
    if not full_scene_path.begins_with("res://"):
        full_scene_path = "res://" + full_scene_path
    if debug_mode:
        print("Scene path (with res://): " + full_scene_path)
    
    var absolute_scene_path = ProjectSettings.globalize_path(full_scene_path)
    if debug_mode:
        print("Absolute scene path: " + absolute_scene_path)
    
    if not FileAccess.file_exists(absolute_scene_path):
        printerr("Scene file does not exist at: " + absolute_scene_path)
        quit(1)
    
    var scene = load(full_scene_path)
    if not scene:
        printerr("Failed to load scene: " + full_scene_path)
        quit(1)
    
    if debug_mode:
        print("Scene loaded successfully")
    var scene_root = track_temp(scene.instantiate())
    if debug_mode:
        print("Scene instantiated")
    
    # Use traditional if-else statement for better compatibility
    var parent_path = "root"  # Default value
    if params.has("parent_node_path"):
        parent_path = params.parent_node_path
    if debug_mode:
        print("Parent path: " + parent_path)
    
    var parent = scene_root
    if parent_path != "root":
        parent = scene_root.get_node(parent_path.replace("root/", ""))
        if not parent:
            printerr("Parent node not found: " + parent_path)
            quit(1)
    if debug_mode:
        print("Parent node found: " + parent.name)
    
    if debug_mode:
        print("Instantiating node of type: " + params.node_type)
    var new_node = track_temp(instantiate_class(params.node_type))
    if not new_node:
        printerr("Failed to instantiate node of type: " + params.node_type)
        printerr("Make sure the class exists and can be instantiated")
        printerr("Check if the class is registered in ClassDB or available as a script")
        quit(1)
    new_node.name = params.node_name
    if debug_mode:
        print("New node created with name: " + new_node.name)
    
    if params.has("properties"):
        if debug_mode:
            print("Setting properties on node")
        var properties = params.properties
        for property in properties:
            if debug_mode:
                print("Setting property: " + property + " = " + str(properties[property]))
            var value = properties[property]
            if typeof(value) == TYPE_STRING and value.begins_with("res://"):
                value = load(value)
                if debug_mode:
                    print("Loaded resource for property: " + property + " -> " + str(value))
            new_node.set(property, value)
    
    parent.add_child(new_node)
    new_node.owner = scene_root
    if debug_mode:
        print("Node added to parent and ownership set")
    
    var packed_scene = PackedScene.new()
    var result = packed_scene.pack(scene_root)
    if debug_mode:
        print("Pack result: " + str(result) + " (OK=" + str(OK) + ")")
    
    if result == OK:
        if debug_mode:
            print("Saving scene to: " + absolute_scene_path)
        var save_error = ResourceSaver.save(packed_scene, absolute_scene_path)
        if debug_mode:
            print("Save result: " + str(save_error) + " (OK=" + str(OK) + ")")
        if save_error == OK:
            if debug_mode:
                var file_check_after = FileAccess.file_exists(absolute_scene_path)
                print("File exists check after save: " + str(file_check_after))
                if file_check_after:
                    print("Node '" + params.node_name + "' of type '" + params.node_type + "' added successfully")
                else:
                    printerr("File reported as saved but does not exist at: " + absolute_scene_path)
                    quit(1)
            else:
                print("Node '" + params.node_name + "' of type '" + params.node_type + "' added successfully")
        else:
            printerr("Failed to save scene: " + str(save_error))
            quit(1)
    else:
        printerr("Failed to pack scene: " + str(result))
        quit(1)

# Load a sprite into a Sprite2D node
func load_sprite(params):
    print("Loading sprite into scene: " + params.scene_path)
    
    # Ensure the scene path starts with res:// for Godot's resource system
    var full_scene_path = params.scene_path
    if not full_scene_path.begins_with("res://"):
        full_scene_path = "res://" + full_scene_path
    
    if debug_mode:
        print("Full scene path (with res://): " + full_scene_path)
    
    # Check if the scene file exists
    var file_check = FileAccess.file_exists(full_scene_path)
    if debug_mode:
        print("Scene file exists check: " + str(file_check))
    
    if not file_check:
        printerr("Scene file does not exist at: " + full_scene_path)
        # Get the absolute path for reference
        var absolute_path = ProjectSettings.globalize_path(full_scene_path)
        printerr("Absolute file path that doesn't exist: " + absolute_path)
        quit(1)
    
    # Ensure the texture path starts with res:// for Godot's resource system
    var full_texture_path = params.texture_path
    if not full_texture_path.begins_with("res://"):
        full_texture_path = "res://" + full_texture_path
    
    if debug_mode:
        print("Full texture path (with res://): " + full_texture_path)
    
    # Load the scene
    var scene = load(full_scene_path)
    if not scene:
        printerr("Failed to load scene: " + full_scene_path)
        quit(1)
    
    if debug_mode:
        print("Scene loaded successfully")
    
    # Instance the scene
    var scene_root = track_temp(scene.instantiate())
    if debug_mode:
        print("Scene instantiated")
    
    # Find the sprite node
    var node_path = params.node_path
    if debug_mode:
        print("Original node path: " + node_path)
    
    if node_path.begins_with("root/"):
        node_path = node_path.substr(5)  # Remove "root/" prefix
        if debug_mode:
            print("Node path after removing 'root/' prefix: " + node_path)
    
    var sprite_node = null
    if node_path == "":
        # If no node path, assume root is the sprite
        sprite_node = scene_root
        if debug_mode:
            print("Using root node as sprite node")
    else:
        sprite_node = scene_root.get_node(node_path)
        if sprite_node and debug_mode:
            print("Found sprite node: " + sprite_node.name)
    
    if not sprite_node:
        printerr("Node not found: " + params.node_path)
        quit(1)
    
    # Check if the node is a Sprite2D or compatible type
    if debug_mode:
        print("Node class: " + sprite_node.get_class())
    if not (sprite_node is Sprite2D or sprite_node is Sprite3D or sprite_node is TextureRect):
        printerr("Node is not a sprite-compatible type: " + sprite_node.get_class())
        quit(1)
    
    # Load the texture
    if debug_mode:
        print("Loading texture from: " + full_texture_path)
    var texture = load(full_texture_path)
    if not texture:
        printerr("Failed to load texture: " + full_texture_path)
        quit(1)
    
    if debug_mode:
        print("Texture loaded successfully")
    
    # Set the texture on the sprite
    if sprite_node is Sprite2D or sprite_node is Sprite3D:
        sprite_node.texture = texture
        if debug_mode:
            print("Set texture on Sprite2D/Sprite3D node")
    elif sprite_node is TextureRect:
        sprite_node.texture = texture
        if debug_mode:
            print("Set texture on TextureRect node")
    
    # Save the modified scene
    var packed_scene = PackedScene.new()
    var result = packed_scene.pack(scene_root)
    if debug_mode:
        print("Pack result: " + str(result) + " (OK=" + str(OK) + ")")
    
    if result == OK:
        if debug_mode:
            print("Saving scene to: " + full_scene_path)
        var error = ResourceSaver.save(packed_scene, full_scene_path)
        if debug_mode:
            print("Save result: " + str(error) + " (OK=" + str(OK) + ")")
        
        if error == OK:
            # Verify the file was actually updated
            if debug_mode:
                var file_check_after = FileAccess.file_exists(full_scene_path)
                print("File exists check after save: " + str(file_check_after))
                
                if file_check_after:
                    print("Sprite loaded successfully with texture: " + full_texture_path)
                    # Get the absolute path for reference
                    var absolute_path = ProjectSettings.globalize_path(full_scene_path)
                    print("Absolute file path: " + absolute_path)
                else:
                    printerr("File reported as saved but does not exist at: " + full_scene_path)
                    quit(1)
            else:
                print("Sprite loaded successfully with texture: " + full_texture_path)
        else:
            printerr("Failed to save scene: " + str(error))
            quit(1)
    else:
        printerr("Failed to pack scene: " + str(result))
        quit(1)

# Export a scene as a MeshLibrary resource
func export_mesh_library(params):
    print("Exporting MeshLibrary from scene: " + params.scene_path)
    
    # Ensure the scene path starts with res:// for Godot's resource system
    var full_scene_path = params.scene_path
    if not full_scene_path.begins_with("res://"):
        full_scene_path = "res://" + full_scene_path
    
    if debug_mode:
        print("Full scene path (with res://): " + full_scene_path)
    
    # Ensure the output path starts with res:// for Godot's resource system
    var full_output_path = params.output_path
    if not full_output_path.begins_with("res://"):
        full_output_path = "res://" + full_output_path
    
    if debug_mode:
        print("Full output path (with res://): " + full_output_path)
    
    # Check if the scene file exists
    var file_check = FileAccess.file_exists(full_scene_path)
    if debug_mode:
        print("Scene file exists check: " + str(file_check))
    
    if not file_check:
        printerr("Scene file does not exist at: " + full_scene_path)
        # Get the absolute path for reference
        var absolute_path = ProjectSettings.globalize_path(full_scene_path)
        printerr("Absolute file path that doesn't exist: " + absolute_path)
        quit(1)
    
    # Load the scene
    if debug_mode:
        print("Loading scene from: " + full_scene_path)
    var scene = load(full_scene_path)
    if not scene:
        printerr("Failed to load scene: " + full_scene_path)
        quit(1)
    
    if debug_mode:
        print("Scene loaded successfully")
    
    # Instance the scene
    var scene_root = track_temp(scene.instantiate())
    if debug_mode:
        print("Scene instantiated")
    
    # Create a new MeshLibrary
    var mesh_library = MeshLibrary.new()
    if debug_mode:
        print("Created new MeshLibrary")
    
    # Get mesh item names if provided
    var mesh_item_names = params.mesh_item_names if params.has("mesh_item_names") else []
    var use_specific_items = mesh_item_names.size() > 0
    
    if debug_mode:
        if use_specific_items:
            print("Using specific mesh items: " + str(mesh_item_names))
        else:
            print("Using all mesh items in the scene")
    
    # Process all child nodes
    var item_id = 0
    if debug_mode:
        print("Processing child nodes...")
    
    for child in scene_root.get_children():
        if debug_mode:
            print("Checking child node: " + child.name)
        
        # Skip if not using all items and this item is not in the list
        if use_specific_items and not (child.name in mesh_item_names):
            if debug_mode:
                print("Skipping node " + child.name + " (not in specified items list)")
            continue
            
        # Check if the child has a mesh
        var mesh_instance = null
        if child is MeshInstance3D:
            mesh_instance = child
            if debug_mode:
                print("Node " + child.name + " is a MeshInstance3D")
        else:
            # Try to find a MeshInstance3D in the child's descendants
            if debug_mode:
                print("Searching for MeshInstance3D in descendants of " + child.name)
            for descendant in child.get_children():
                if descendant is MeshInstance3D:
                    mesh_instance = descendant
                    if debug_mode:
                        print("Found MeshInstance3D in descendant: " + descendant.name)
                    break
        
        if mesh_instance and mesh_instance.mesh:
            if debug_mode:
                print("Adding mesh: " + child.name)
            
            # Add the mesh to the library
            mesh_library.create_item(item_id)
            mesh_library.set_item_name(item_id, child.name)
            mesh_library.set_item_mesh(item_id, mesh_instance.mesh)
            if debug_mode:
                print("Added mesh to library with ID: " + str(item_id))
            
            # Add collision shape if available
            var collision_added = false
            for collision_child in child.get_children():
                if collision_child is CollisionShape3D and collision_child.shape:
                    mesh_library.set_item_shapes(item_id, [collision_child.shape])
                    if debug_mode:
                        print("Added collision shape from: " + collision_child.name)
                    collision_added = true
                    break
            
            if debug_mode and not collision_added:
                print("No collision shape found for mesh: " + child.name)
            
            # Add preview if available
            if mesh_instance.mesh:
                mesh_library.set_item_preview(item_id, mesh_instance.mesh)
                if debug_mode:
                    print("Added preview for mesh: " + child.name)
            
            item_id += 1
        elif debug_mode:
            print("Node " + child.name + " has no valid mesh")
    
    if debug_mode:
        print("Processed " + str(item_id) + " meshes")
    
    # Create directory if it doesn't exist
    var dir = DirAccess.open("res://")
    if dir == null:
        printerr("Failed to open res:// directory")
        printerr("DirAccess error: " + str(DirAccess.get_open_error()))
        quit(1)
        
    var output_dir = full_output_path.get_base_dir()
    if debug_mode:
        print("Output directory: " + output_dir)
    
    if output_dir != "res://" and not dir.dir_exists(output_dir.substr(6)):  # Remove "res://" prefix
        if debug_mode:
            print("Creating directory: " + output_dir)
        var error = dir.make_dir_recursive(output_dir.substr(6))  # Remove "res://" prefix
        if error != OK:
            printerr("Failed to create directory: " + output_dir + ", error: " + str(error))
            quit(1)
    
    # Save the mesh library
    if item_id > 0:
        if debug_mode:
            print("Saving MeshLibrary to: " + full_output_path)
        var error = ResourceSaver.save(mesh_library, full_output_path)
        if debug_mode:
            print("Save result: " + str(error) + " (OK=" + str(OK) + ")")
        
        if error == OK:
            # Verify the file was actually created
            if debug_mode:
                var file_check_after = FileAccess.file_exists(full_output_path)
                print("File exists check after save: " + str(file_check_after))
                
                if file_check_after:
                    print("MeshLibrary exported successfully with " + str(item_id) + " items to: " + full_output_path)
                    # Get the absolute path for reference
                    var absolute_path = ProjectSettings.globalize_path(full_output_path)
                    print("Absolute file path: " + absolute_path)
                else:
                    printerr("File reported as saved but does not exist at: " + full_output_path)
                    quit(1)
            else:
                print("MeshLibrary exported successfully with " + str(item_id) + " items to: " + full_output_path)
        else:
            printerr("Failed to save MeshLibrary: " + str(error))
            quit(1)
    else:
        printerr("No valid meshes found in the scene")
        quit(1)

# Get UID for a specific file
func get_uid(params):
    if not params.has("file_path"):
        printerr("File path is required")
        quit(1)
    
    # Ensure the file path starts with res:// for Godot's resource system
    var file_path = params.file_path
    if not file_path.begins_with("res://"):
        file_path = "res://" + file_path
    
    print("Getting UID for file: " + file_path)
    if debug_mode:
        print("Full file path (with res://): " + file_path)
    
    # Get the absolute path for reference
    var absolute_path = ProjectSettings.globalize_path(file_path)
    if debug_mode:
        print("Absolute file path: " + absolute_path)
    
    # Ensure the file exists
    var file_check = FileAccess.file_exists(file_path)
    if debug_mode:
        print("File exists check: " + str(file_check))
    
    if not file_check:
        printerr("File does not exist at: " + file_path)
        printerr("Absolute file path that doesn't exist: " + absolute_path)
        quit(1)
    
    # Check if the UID file exists
    var uid_path = file_path + ".uid"
    if debug_mode:
        print("UID file path: " + uid_path)
    
    var uid_check = FileAccess.file_exists(uid_path)
    if debug_mode:
        print("UID file exists check: " + str(uid_check))
    
    var f = FileAccess.open(uid_path, FileAccess.READ)
    
    if f:
        # Read the UID content
        var uid_content = f.get_as_text()
        f.close()
        if debug_mode:
            print("UID content read successfully")
        
        # Return the UID content
        var result = {
            "file": file_path,
            "absolutePath": absolute_path,
            "uid": uid_content.strip_edges(),
            "exists": true
        }
        if debug_mode:
            print("UID result: " + JSON.stringify(result))
        print(JSON_START)
        print(JSON.stringify(result))
        print(JSON_END)
    else:
        if debug_mode:
            print("UID file does not exist or could not be opened")
        
        # UID file doesn't exist
        var result = {
            "file": file_path,
            "absolutePath": absolute_path,
            "exists": false,
            "message": "UID file does not exist for this file. Use update_project_uids to scan the project and generate UIDs."
        }
        if debug_mode:
            print("UID result: " + JSON.stringify(result))
        print(JSON_START)
        print(JSON.stringify(result))
        print(JSON_END)

# Save changes to a scene file
func save_scene(params):
    print("Saving scene: " + params.scene_path)
    
    # Ensure the scene path starts with res:// for Godot's resource system
    var full_scene_path = params.scene_path
    if not full_scene_path.begins_with("res://"):
        full_scene_path = "res://" + full_scene_path
    
    if debug_mode:
        print("Full scene path (with res://): " + full_scene_path)
    
    # Check if the scene file exists
    var file_check = FileAccess.file_exists(full_scene_path)
    if debug_mode:
        print("Scene file exists check: " + str(file_check))
    
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
    
    if debug_mode:
        print("Scene loaded successfully")
    
    # Instance the scene
    var scene_root = track_temp(scene.instantiate())
    if debug_mode:
        print("Scene instantiated")
    
    # Determine save path
    var save_path = params.new_path if params.has("new_path") else full_scene_path
    if params.has("new_path") and not save_path.begins_with("res://"):
        save_path = "res://" + save_path
    
    if debug_mode:
        print("Save path: " + save_path)
    
    # Create directory if it doesn't exist
    if params.has("new_path"):
        var dir = DirAccess.open("res://")
        if dir == null:
            printerr("Failed to open res:// directory")
            printerr("DirAccess error: " + str(DirAccess.get_open_error()))
            quit(1)
            
        var scene_dir = save_path.get_base_dir()
        if debug_mode:
            print("Scene directory: " + scene_dir)
        
        if scene_dir != "res://" and not dir.dir_exists(scene_dir.substr(6)):  # Remove "res://" prefix
            if debug_mode:
                print("Creating directory: " + scene_dir)
            var error = dir.make_dir_recursive(scene_dir.substr(6))  # Remove "res://" prefix
            if error != OK:
                printerr("Failed to create directory: " + scene_dir + ", error: " + str(error))
                quit(1)
    
    # Create a packed scene
    var packed_scene = PackedScene.new()
    var result = packed_scene.pack(scene_root)
    if debug_mode:
        print("Pack result: " + str(result) + " (OK=" + str(OK) + ")")
    
    if result == OK:
        if debug_mode:
            print("Saving scene to: " + save_path)
        var error = ResourceSaver.save(packed_scene, save_path)
        if debug_mode:
            print("Save result: " + str(error) + " (OK=" + str(OK) + ")")
        
        if error == OK:
            # Verify the file was actually created/updated
            if debug_mode:
                var file_check_after = FileAccess.file_exists(save_path)
                print("File exists check after save: " + str(file_check_after))
                
                if file_check_after:
                    print("Scene saved successfully to: " + save_path)
                    # Get the absolute path for reference
                    var absolute_path = ProjectSettings.globalize_path(save_path)
                    print("Absolute file path: " + absolute_path)
                else:
                    printerr("File reported as saved but does not exist at: " + save_path)
                    quit(1)
            else:
                print("Scene saved successfully to: " + save_path)
        else:
            printerr("Failed to save scene: " + str(error))
            quit(1)
    else:
        printerr("Failed to pack scene: " + str(result))
        quit(1)

# Read a scene file as a JSON node tree
func read_scene(params):
    if not params.has("scene_path"):
        printerr("scene_path is required")
        quit(1)

    var full_scene_path = params.scene_path
    if not full_scene_path.begins_with("res://"):
        full_scene_path = "res://" + full_scene_path

    log_info("Reading scene: " + full_scene_path)

    if not FileAccess.file_exists(full_scene_path):
        printerr("Scene file does not exist at: " + full_scene_path)
        quit(1)

    var scene = load(full_scene_path)
    if not scene:
        printerr("Failed to load scene: " + full_scene_path)
        printerr("The scene may reference missing external resources.")
        var f = FileAccess.open(full_scene_path, FileAccess.READ)
        if f:
            var raw_content = f.get_as_text()
            f.close()
            print(JSON_START)
            print(JSON.stringify({"error": "Failed to instantiate scene, returning raw text", "raw": raw_content.substr(0, 4096)}))
            print(JSON_END)
            return
        quit(1)

    var scene_root = track_temp(scene.instantiate())
    if scene_root == null:
        printerr("Failed to instantiate scene: " + full_scene_path)
        quit(1)

    var tree_data = _walk_scene_tree(scene_root)

    print(JSON_START)
    print(JSON.stringify(tree_data))
    print(JSON_END)

    scene_root.queue_free()

# Walk a scene tree and build a dictionary describing each node
func _walk_scene_tree(node) -> Dictionary:
    var info = {
        "name": node.name,
        "type": node.get_class(),
    }

    var node_script = node.get_script()
    if node_script != null and node_script is Script:
        info["script"] = node_script.resource_path

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

    var children_arr = []
    for child in node.get_children():
        children_arr.append(_walk_scene_tree(child))

    if children_arr.size() > 0:
        info["children"] = children_arr

    return info

# Convert a Variant value to a JSON-serializable value. JSON-native types
# (String, bool, numbers, Array, Dictionary) are converted recursively so they
# survive as real JSON; every other Variant has no JSON equivalent (Vector2,
# Color, NodePath, ...) and is kept as its exact engine string via str().
func _variant_to_json(value):
    match typeof(value):
        TYPE_NIL:
            return null
        TYPE_BOOL, TYPE_INT, TYPE_STRING:
            return value
        TYPE_FLOAT:
            # JSON has no NaN/Infinity; stringify would warn on NaN.
            if not is_finite(value):
                return str(value)
            return value
        TYPE_STRING_NAME:
            return String(value)
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
        _:
            return str(value)

# Helper: Convert a JSON value to the correct Godot type based on a node's property type
func _convert_property_value(node, prop_name, value):
    for prop in node.get_property_list():
        if prop["name"] == prop_name:
            var type_id = prop.get("type", 0)
            match type_id:
                TYPE_VECTOR2:
                    if value is Dictionary and value.has("x") and value.has("y"):
                        return Vector2(float(value.get("x", 0)), float(value.get("y", 0)))
                TYPE_VECTOR2I:
                    if value is Dictionary and value.has("x") and value.has("y"):
                        return Vector2i(int(value.get("x", 0)), int(value.get("y", 0)))
                TYPE_VECTOR3:
                    if value is Dictionary and value.has("x") and value.has("y"):
                        return Vector3(float(value.get("x", 0)), float(value.get("y", 0)), float(value.get("z", 0)))
                TYPE_VECTOR3I:
                    if value is Dictionary and value.has("x") and value.has("y"):
                        return Vector3i(int(value.get("x", 0)), int(value.get("y", 0)), int(value.get("z", 0)))
                TYPE_COLOR:
                    if value is Dictionary and value.has("r") and value.has("g") and value.has("b"):
                        return Color(float(value.get("r", 0)), float(value.get("g", 0)), float(value.get("b", 0)), float(value.get("a", 1.0)))
                    if value is String and value.begins_with("#"):
                        return Color.html(value)
                TYPE_QUATERNION:
                    if value is Dictionary:
                        return Quaternion(float(value.get("x", 0)), float(value.get("y", 0)), float(value.get("z", 0)), float(value.get("w", 1)))
                TYPE_RECT2:
                    if value is Dictionary and value.has("position") and value.has("size"):
                        var pos = value["position"]
                        var sz = value["size"]
                        return Rect2(float(pos.get("x", 0)), float(pos.get("y", 0)), float(sz.get("x", 0)), float(sz.get("y", 0)))
                TYPE_AABB:
                    if value is Dictionary and value.has("position") and value.has("size"):
                        var pos = value["position"]
                        var sz = value["size"]
                        return AABB(
                            Vector3(float(pos.get("x", 0)), float(pos.get("y", 0)), float(pos.get("z", 0))),
                            Vector3(float(sz.get("x", 0)), float(sz.get("y", 0)), float(sz.get("z", 0)))
                        )
                TYPE_BASIS:
                    if value is Dictionary and value.has("x") and value.has("y") and value.has("z"):
                        var bx = value["x"]
                        var by = value["y"]
                        var bz = value["z"]
                        return Basis(
                            Vector3(float(bx.get("x", 0)), float(bx.get("y", 0)), float(bx.get("z", 0))),
                            Vector3(float(by.get("x", 0)), float(by.get("y", 0)), float(by.get("z", 0))),
                            Vector3(float(bz.get("x", 0)), float(bz.get("y", 0)), float(bz.get("z", 0)))
                        )
                TYPE_TRANSFORM3D:
                    if value is Dictionary and value.has("basis") and value.has("origin"):
                        var basis_d = value["basis"]
                        var origin_d = value["origin"]
                        var basis = Basis.IDENTITY
                        if basis_d is Dictionary and basis_d.has("x"):
                            var bx = basis_d["x"]
                            var by = basis_d["y"]
                            var bz = basis_d["z"]
                            basis = Basis(
                                Vector3(float(bx.get("x", 0)), float(bx.get("y", 0)), float(bx.get("z", 0))),
                                Vector3(float(by.get("x", 0)), float(by.get("y", 0)), float(by.get("z", 0))),
                                Vector3(float(bz.get("x", 0)), float(bz.get("y", 0)), float(bz.get("z", 0)))
                            )
                        var origin = Vector3(float(origin_d.get("x", 0)), float(origin_d.get("y", 0)), float(origin_d.get("z", 0)))
                        return Transform3D(basis, origin)
                TYPE_TRANSFORM2D:
                    if value is Dictionary and value.has("x") and value.has("y") and value.has("origin"):
                        var tx = value["x"]
                        var ty = value["y"]
                        var t_origin = value["origin"]
                        return Transform2D(
                            Vector2(float(tx.get("x", 0)), float(tx.get("y", 0))),
                            Vector2(float(ty.get("x", 0)), float(ty.get("y", 0))),
                            Vector2(float(t_origin.get("x", 0)), float(t_origin.get("y", 0)))
                        )
                TYPE_BOOL:
                    if value is String:
                        return value.to_lower() == "true"
                    return bool(value)
                TYPE_INT:
                    return int(value)
                TYPE_FLOAT:
                    return float(value)
                TYPE_STRING:
                    return str(value)
                TYPE_NODE_PATH:
                    return NodePath(str(value))
                TYPE_OBJECT:
                    if value is String and value.begins_with("res://"):
                        if ResourceLoader.exists(value):
                            var res = load(value)
                            if res != null:
                                return res
                        printerr("Failed to load resource from path: " + value)
                        quit(1)
                    return value
            break
    return value

# Modify a node's properties in a scene file
func modify_node(params):
    if not params.has("scene_path") or not params.has("node_path") or not params.has("properties"):
        printerr("scene_path, node_path, and properties are required")
        quit(1)

    var full_scene_path = params.scene_path
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
    var node_path = params.node_path
    var target = scene_root
    if node_path != "root" and node_path != ".":
        if node_path.begins_with("root/"):
            node_path = node_path.substr(5)
        target = scene_root.get_node_or_null(node_path)

    if target == null:
        printerr("Node not found: " + params.node_path)
        quit(1)

    # Set properties with type conversion
    var properties = params.properties
    for prop_name in properties:
        var raw_value = properties[prop_name]
        var converted_value = _convert_property_value(target, prop_name, raw_value)
        log_info("Setting " + prop_name + " = " + str(converted_value) + " (from " + str(raw_value) + ")")
        target.set(prop_name, converted_value)

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
    if not params.has("scene_path") or not params.has("node_path"):
        printerr("scene_path and node_path are required")
        quit(1)

    var full_scene_path = params.scene_path
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
    var node_path = params.node_path
    var target = scene_root
    if node_path != "root" and node_path != ".":
        if node_path.begins_with("root/"):
            node_path = node_path.substr(5)
        target = scene_root.get_node_or_null(node_path)

    if target == null:
        printerr("Node not found: " + params.node_path)
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
    var scene_path = params.get("scene_path", "")
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
                connections.append(line.strip_edges())
        print(JSON_START)
        print(JSON.stringify({"connections": connections}))
        print(JSON_END)
    elif action == "add":
        var signal_name = params.get("signal_name", "")
        var source_path = params.get("source_path", "")
        var target_path = params.get("target_path", "")
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
        var signal_name = params.get("signal_name", "")
        var source_path = params.get("source_path", "")
        var target_path = params.get("target_path", "")
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
    var scene_path = params.get("scene_path", "")
    var action = params.get("action", "rename")
    var node_path_str = params.get("node_path", "")
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
        var new_name = params.get("new_name", "")
        if new_name.is_empty():
            printerr("new_name is required for rename")
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
        var new_parent_path = params.get("new_parent_path", "")
        if new_parent_path.is_empty():
            printerr("new_parent_path is required for move")
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
    if not params.has("scene_path") or not params.has("node_path") or not params.has("script_path"):
        printerr("scene_path, node_path, and script_path are required")
        quit(1)

    var full_scene_path = params.scene_path
    if not full_scene_path.begins_with("res://"):
        full_scene_path = "res://" + full_scene_path

    var full_script_path = params.script_path
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
    var node_path = params.node_path
    var target = scene_root
    if node_path != "root" and node_path != ".":
        if node_path.begins_with("root/"):
            node_path = node_path.substr(5)
        target = scene_root.get_node_or_null(node_path)

    if target == null:
        printerr("Node not found: " + params.node_path)
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
func create_resource(params):
    if not params.has("resource_type") or not params.has("resource_path"):
        printerr("resource_type and resource_path are required")
        quit(1)

    var resource_type = params.resource_type
    var full_resource_path = params.resource_path
    if not full_resource_path.begins_with("res://"):
        full_resource_path = "res://" + full_resource_path

    log_info("Creating resource of type " + resource_type + " at: " + full_resource_path)

    # Instantiate the resource
    if not ClassDB.class_exists(resource_type):
        printerr("Unknown resource type: " + resource_type)
        printerr("Must be a valid Godot class name (e.g., StandardMaterial3D, AudioStreamPlayer, Theme)")
        quit(1)

    if not ClassDB.can_instantiate(resource_type):
        printerr("Cannot instantiate resource type: " + resource_type)
        quit(1)

    var resource = ClassDB.instantiate(resource_type)
    if resource == null:
        printerr("Failed to instantiate resource of type: " + resource_type)
        quit(1)

    if not resource is Resource:
        printerr("Type " + resource_type + " is not a Resource subclass")
        quit(1)

    # Set properties if provided
    if params.has("properties"):
        var properties = params.properties
        for prop_name in properties:
            var raw_value = properties[prop_name]
            var converted_value = _convert_property_value(resource, prop_name, raw_value)
            log_info("Setting " + prop_name + " = " + str(converted_value))
            resource.set(prop_name, converted_value)

    # Ensure directory exists
    var dir_path = full_resource_path.get_base_dir()
    var dir_relative = dir_path.substr(6)  # Remove "res://"
    if not dir_relative.is_empty():
        var dir = DirAccess.open("res://")
        if dir and not dir.dir_exists(dir_relative):
            dir.make_dir_recursive(dir_relative)

    # Save the resource
    var save_error = ResourceSaver.save(resource, full_resource_path)
    if save_error != OK:
        printerr("Failed to save resource: " + str(save_error))
        quit(1)

    print("Resource created successfully at: " + full_resource_path)


func manage_resource(params):
    var resource_path = params.get("resource_path", "")
    var action = params.get("action", "read")
    var full_path = resource_path
    if not full_path.begins_with("res://"):
        full_path = "res://" + full_path

    if action == "read":
        if not ResourceLoader.exists(full_path):
            printerr("Resource not found: " + full_path)
            quit(1)
        var res = ResourceLoader.load(full_path)
        if res == null:
            printerr("Failed to load resource: " + full_path)
            quit(1)
        var props = {}
        for prop in res.get_property_list():
            if prop["usage"] & PROPERTY_USAGE_STORAGE:
                props[prop["name"]] = str(res.get(prop["name"]))
        print(JSON_START)
        print(JSON.stringify({"type": res.get_class(), "path": full_path, "properties": props}))
        print(JSON_END)
    elif action == "modify":
        if not ResourceLoader.exists(full_path):
            printerr("Resource not found: " + full_path)
            quit(1)
        var res = ResourceLoader.load(full_path)
        if res == null:
            printerr("Failed to load resource: " + full_path)
            quit(1)
        var properties = params.get("properties", {})
        for prop_name in properties:
            var raw_value = properties[prop_name]
            var converted_value = _convert_property_value(res, prop_name, raw_value)
            res.set(prop_name, converted_value)
        var save_error = ResourceSaver.save(res, full_path)
        if save_error != OK:
            printerr("Failed to save resource: " + str(save_error))
            quit(1)
        print("Resource modified: " + full_path)
    else:
        printerr("Unknown manage_resource action: " + action)
        quit(1)


func manage_theme_resource(params):
    var resource_path = params.get("resource_path", "")
    var action = params.get("action", "read")
    var full_path = resource_path
    if not full_path.begins_with("res://"):
        full_path = "res://" + full_path

    if action == "create":
        var theme = Theme.new()
        var properties = params.get("properties", {})
        for key in properties:
            theme.set(key, properties[key])
        var dir_path = full_path.get_base_dir()
        var dir_relative = dir_path.substr(6)
        if not dir_relative.is_empty():
            var dir = DirAccess.open("res://")
            if dir and not dir.dir_exists(dir_relative):
                dir.make_dir_recursive(dir_relative)
        var save_error = ResourceSaver.save(theme, full_path)
        if save_error != OK:
            printerr("Failed to save theme: " + str(save_error))
            quit(1)
        print("Theme created at: " + full_path)
    elif action == "read":
        if not ResourceLoader.exists(full_path):
            printerr("Theme not found: " + full_path)
            quit(1)
        var theme = ResourceLoader.load(full_path)
        if theme == null:
            printerr("Failed to load theme: " + full_path)
            quit(1)
        var props = {}
        for prop in theme.get_property_list():
            if prop["usage"] & PROPERTY_USAGE_STORAGE:
                props[prop["name"]] = str(theme.get(prop["name"]))
        print(JSON_START)
        print(JSON.stringify({"type": theme.get_class(), "path": full_path, "properties": props}))
        print(JSON_END)
    elif action == "modify":
        if not ResourceLoader.exists(full_path):
            printerr("Theme not found: " + full_path)
            quit(1)
        var theme = ResourceLoader.load(full_path)
        if theme == null:
            printerr("Failed to load theme: " + full_path)
            quit(1)
        var properties = params.get("properties", {})
        for key in properties:
            theme.set(key, properties[key])
        var save_error = ResourceSaver.save(theme, full_path)
        if save_error != OK:
            printerr("Failed to save theme: " + str(save_error))
            quit(1)
        print("Theme modified: " + full_path)
    else:
        printerr("Unknown manage_theme_resource action: " + action)
        quit(1)

# Validate that Godot itself loaded project.godot cleanly and report the
# settings the engine actually applied. PropertySettings parses project.godot
# during engine startup, before _init runs; when the file is malformed the
# engine logs "Error parsing '<path>/project.godot'" to stderr and leaves the
# affected settings at their defaults. This operation surfaces the settings
# that differ from the engine defaults, so a caller can confirm both that the
# project parsed and what Godot read back. A value is emitted as its Godot
# literal (var_to_str) via JSON so the payload stays single-line.
func validate_project(_params):
    var settings = {}
    for prop in ProjectSettings.get_property_list():
        var setting_name = prop.name
        var value = ProjectSettings.get_setting(setting_name)
        if value != ProjectSettings.property_get_revert(setting_name):
            settings[setting_name] = var_to_str(value)
    var result = {
        "projectPath": ProjectSettings.globalize_path("res://"),
        "settingCount": settings.size(),
        "settings": settings,
    }
    print(JSON_START)
    print(JSON.stringify(result))
    print(JSON_END)
