#!/usr/bin/env -S godot --headless --script
# Derived from tugcantopaloglu/godot-mcp (src/scripts/godot_operations.gd), a
# fork of Coding-Solo/godot-mcp (both MIT-licensed); portions of the handler
# logic originate there, but the file has been substantially reworked.
extends SceneTree

const JSON_START = "<<<GODOT_MCP_JSON_START>>>"
const JSON_END = "<<<GODOT_MCP_JSON_END>>>"

# Caps how many print/error lines a run_script call captures so a chatty script
# cannot grow the frame without bound.
const RUN_SCRIPT_LOG_LIMIT := 2000

class Result extends RefCounted:
    var ok := true
    var stdout := ""
    var stderr := ""

    func out(message: String) -> void:
        stdout += message + "\n"

    func err(message: String) -> void:
        stderr += message + "\n"

    func fail(message: String = "") -> Result:
        if not message.is_empty():
            err(message)
        ok = false
        return self

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

# Captures the print()/push_error() output a run_script target emits while it
# executes; the engine's default logger still forwards each line to the process
# streams, so this only observes.
class ScriptRunLogger extends Logger:
    var messages := PackedStringArray()
    var errors := []
    var limit := 0

    func _log_message(message: String, _error: bool) -> void:
        if limit > 0 and messages.size() >= limit:
            return
        messages.append(message.trim_suffix("\n"))

    func _log_error(function: String, file: String, line: int, code: String, rationale: String, _editor_notify: bool, _error_type: int, _script_backtraces) -> void:
        if limit > 0 and errors.size() >= limit:
            return
        errors.append({
            "function": function,
            "file": file,
            "line": line,
            "code": code,
            "rationale": rationale,
        })

func _init():
    var endpoint := _engine_endpoint()
    if endpoint.is_empty():
        printerr("engine socket: exactly one of --gd-socket <path> or --gd-tcp <host:port> is required; launch the bridge through the engine session")
        quit(1)
        return
    if endpoint["transport"] == "uds":
        _bind_uds(endpoint["path"])
    else:
        _bind_tcp(endpoint["host"], endpoint["port"])

func _engine_endpoint() -> Dictionary:
    var args := OS.get_cmdline_user_args()
    var endpoint := {}
    for index in range(args.size()):
        if args[index] == "--gd-socket" and index + 1 < args.size():
            if not endpoint.is_empty():
                return {}
            endpoint = {"transport": "uds", "path": args[index + 1]}
        elif args[index] == "--gd-tcp" and index + 1 < args.size():
            if not endpoint.is_empty():
                return {}
            var address := args[index + 1]
            var colon := address.rfind(":")
            if colon <= 0 or colon + 1 >= address.length():
                return {}
            endpoint = {
                "transport": "tcp",
                "host": address.substr(0, colon),
                "port": address.substr(colon + 1).to_int(),
            }
    return endpoint

var _server: SocketServer = null
var _peer: StreamPeerSocket = null
var _transport := ""
var _endpoint := {}
var _token := ""
var _recv := PackedByteArray()
var _authenticated := false

var _filesystem: EditorFileSystem = null
var _ready_printed := false
var _scan_seen := false
var _quit_pending := false

var _await_scan := false
var _deferred_id := -1
var _deferred_result: Result = null

var _reconcile_paths_pending: PackedStringArray = PackedStringArray()

func _bind_uds(socket_path: String) -> void:
    # A stale socket or regular file makes listen fail with EINVAL.
    if FileAccess.file_exists(socket_path):
        DirAccess.remove_absolute(socket_path)
    var server := UDSServer.new()
    var listen_error := server.listen(socket_path)
    if listen_error != OK:
        printerr("engine socket: listen failed: " + str(listen_error))
        quit(1)
        return
    _server = server
    _transport = "uds"
    _endpoint = {"path": socket_path}
    _token = Crypto.new().generate_random_bytes(16).hex_encode()

func _bind_tcp(host: String, port: int) -> void:
    var server := TCPServer.new()
    var listen_error := server.listen(port, host)
    if listen_error != OK:
        printerr("engine socket: listen failed: " + str(listen_error))
        quit(1)
        return
    _server = server
    _transport = "tcp"
    _endpoint = {"host": host, "port": server.get_local_port()}
    _token = Crypto.new().generate_random_bytes(16).hex_encode()

func _process(_delta: float) -> bool:
    if _server == null:
        return false
    if not _ready_printed:
        if not _filesystem_ready():
            return false
        _ready_printed = true
        var ready := {"ready": true, "transport": _transport, "token": _token}
        ready.merge(_endpoint)
        print(JSON.stringify(ready))
    if _quit_pending and (_filesystem == null or not _filesystem.is_scanning()):
        quit(0)
        return false
    if _peer == null and _server.is_connection_available():
        _peer = _server.take_connection()
        _recv = PackedByteArray()
        _authenticated = false
    if _peer != null:
        # A StreamPeerTCP from take_connection may not be connected yet; poll until STATUS_CONNECTED.
        _peer.poll()
        if _transport == "tcp" and _peer.get_status() != StreamPeerSocket.STATUS_CONNECTED:
            return false
        _read_peer()
    if _reconcile_paths_pending.size() > 0 and (_filesystem == null or not _filesystem.is_scanning()):
        for path in _reconcile_paths_pending:
            _apply_reconcile(path)
        _reconcile_paths_pending = PackedStringArray()
    if _deferred_result != null and (_filesystem == null or not _filesystem.is_scanning()):
        _send_socket_response(_deferred_id, _deferred_result.ok, _deferred_result.stdout, _deferred_result.stderr)
        _deferred_result = null
        _deferred_id = -1
    return false

func _filesystem_ready() -> bool:
    if not Engine.is_editor_hint():
        return true
    if _filesystem == null:
        _filesystem = EditorInterface.get_resource_filesystem()
        if _filesystem == null:
            return false
    if _filesystem.is_scanning():
        _scan_seen = true
        return false
    return _scan_seen

func _reconcile_path(path: String) -> void:
    if not Engine.is_editor_hint() or not path.begins_with("res://"):
        return
    var filesystem := _filesystem
    if filesystem == null:
        filesystem = EditorInterface.get_resource_filesystem()
        if filesystem == null:
            return
    if not _apply_reconcile(path):
        filesystem.scan_sources()
        _reconcile_paths_pending.append(path)

func _apply_reconcile(path: String) -> bool:
    var filesystem := _filesystem
    if filesystem == null:
        filesystem = EditorInterface.get_resource_filesystem()
    if filesystem == null:
        return false
    if FileAccess.file_exists(path + ".import"):
        filesystem.reimport_files(PackedStringArray([path]))
        return true
    filesystem.update_file(path)
    # update_file silently no-ops on an untracked path; a tracked script gains its .uid.
    return not path.ends_with(".gd") or FileAccess.file_exists(path + ".uid")

func _reconcile_paths(paths: PackedStringArray) -> void:
    for path in paths:
        _reconcile_path(path)

func _read_peer() -> void:
    var available := _peer.get_available_bytes()
    while available > 0:
        var chunk := _peer.get_partial_data(available)
        if chunk[0] != OK:
            _drop_peer()
            return
        _recv.append_array(chunk[1])
        available = _peer.get_available_bytes()
    while true:
        var newline := _recv.find(10)
        if newline == -1:
            return
        var line := _recv.slice(0, newline).get_string_from_utf8()
        _recv = _recv.slice(newline + 1)
        if not _handle_socket_request(line):
            return

func _handle_socket_request(line: String) -> bool:
    var json := JSON.new()
    if json.parse(line) != OK:
        _send_socket_response(-1, false, "", "malformed request: " + json.get_error_message())
        return true
    var request = json.get_data()
    if not (request is Dictionary):
        _send_socket_response(-1, false, "", "malformed request: not an object")
        return true
    if not _authenticated:
        var presented = request.get("token", null)
        if not (presented is String) or presented != _token:
            _drop_peer()
            return false
        _authenticated = true
    var raw_id = request.get("id", null)
    var operation = request.get("operation", null)
    if not (raw_id is float or raw_id is int) or not (operation is String):
        _send_socket_response(-1, false, "", "malformed request: missing id or operation")
        return true
    var id := int(raw_id)
    # Acknowledge quit so the caller unblocks, then leave the main loop.
    if operation == "quit":
        _send_socket_response(id, true, "", "")
        # Exiting mid-scan aborts the scan thread and prints an extra warning, so defer the exit.
        if Engine.is_editor_hint() and _filesystem != null and _filesystem.is_scanning():
            _quit_pending = true
        else:
            quit(0)
        return true
    var params = request.get("params", {})
    if not (params is Dictionary):
        params = {}
    var result := dispatch(operation, params)
    if _await_scan or _reconcile_paths_pending.size() > 0:
        _await_scan = false
        _deferred_id = id
        _deferred_result = result
        return true
    _send_socket_response(id, result.ok, result.stdout, result.stderr)
    return true

func _send_socket_response(id: int, ok: bool, stdout: String, stderr: String) -> void:
    if _peer == null:
        return
    var frame := JSON.stringify({
        "id": id,
        "ok": ok,
        "stdout": stdout,
        "stderr": stderr,
    }) + "\n"
    _peer.put_data(frame.to_utf8_buffer())

func _drop_peer() -> void:
    if _peer != null:
        _peer.disconnect_from_host()
        _peer = null
    _recv = PackedByteArray()
    _authenticated = false

func dispatch(operation, params) -> Result:
    var result := Result.new()
    _temp_nodes.clear()

    log_info(result, "Operation: " + operation)
    log_info(result, "Executing operation: " + operation)

    # A handler that hits an unhandled engine error returns null; never propagate it to the driver.
    var handler_result = null
    match operation:
        "ping":
            handler_result = ping(result, params)
        "create_scene":
            handler_result = create_scene(result, params)
        "add_node":
            handler_result = add_node(result, params)
        "export_mesh_library":
            handler_result = export_mesh_library(result, params)
        "read_scene":
            handler_result = read_scene(result, params)
        "modify_node":
            handler_result = modify_node(result, params)
        "remove_node":
            handler_result = remove_node(result, params)
        "manage_scene_structure":
            handler_result = manage_scene_structure(result, params)
        "attach_script":
            handler_result = attach_script(result, params)
        "run_script":
            handler_result = run_script(result, params)
        "rescan_project":
            handler_result = rescan_project(result, params)
        "manage_resource":
            handler_result = manage_resource(result, params)
        "manage_animation":
            handler_result = manage_animation(result, params)
        "manage_tileset":
            handler_result = manage_tileset(result, params)
        "manage_theme":
            handler_result = manage_theme(result, params)
        "manage_tilemap":
            handler_result = manage_tilemap(result, params)
        "validate_project":
            handler_result = validate_project(result, params)
        _:
            log_error(result, "Unknown operation: " + operation)
            result.ok = false
            handler_result = result

    if not (handler_result is Result):
        result.fail("operation failed with an unhandled engine error: " + operation)
    else:
        result = handler_result

    _free_temp_nodes()
    return result

# CACHE_MODE_IGNORE forces a fresh read: the long-lived engine would otherwise serve a stale cached resource.
func load_resource(path):
    return ResourceLoader.load(path, "", ResourceLoader.CACHE_MODE_IGNORE)

func load_object(path, type_hint):
    return ResourceLoader.load(path, type_hint, ResourceLoader.CACHE_MODE_IGNORE)

# Temp nodes are never added to the scene tree; without freeing them the engine reports ObjectDB/RID leaks at exit.
var _temp_nodes = []

func track_temp(node):
    _temp_nodes.append(node)
    return node

func _free_temp_nodes():
    for node in _temp_nodes:
        if is_instance_valid(node):
            node.free()
    _temp_nodes.clear()

func rescan_project(result: Result, params) -> Result:
    if not Engine.is_editor_hint():
        result.err("rescan_project requires the editor filesystem; run it through the editor session")
        return result.fail()
    var filesystem := _filesystem
    if filesystem == null:
        filesystem = EditorInterface.get_resource_filesystem()
        if filesystem == null:
            result.err("rescan_project requires the editor filesystem; run it through the editor session")
            return result.fail()
    var touched = params.get("paths", [])
    if touched is Array and touched.size() > 0:
        var paths := PackedStringArray()
        for path in touched:
            paths.append(str(path))
        _reconcile_paths(paths)
        result.out("Reconciled " + str(paths.size()) + " path(s) with the editor filesystem")
        return result
    filesystem.scan_sources()
    _await_scan = true
    result.out("Scanned project sources")
    return result

func ping(result: Result, _params) -> Result:
    result.out("pong")
    return result

func log_info(result: Result, message):
    result.out("[INFO] " + message)

func log_error(result: Result, message):
    result.err("[ERROR] " + message)

func project_relative(path):
    if path.begins_with("res://"):
        return path.substr(6)
    return path

# Only the global class registry is consulted; raw paths are rejected to prevent arbitrary script instantiation.
func get_script_by_name(result: Result, name_of_class):
    var global_classes = ProjectSettings.get_global_class_list()
    for global_class in global_classes:
        var found_name_of_class = global_class["class"]
        var found_path = global_class["path"]
        
        if found_name_of_class == name_of_class:
            var script = load_resource(found_path) as Script
            if script:
                return script
            else:
                result.err("Failed to load script from registry path: " + found_path)
                break
    
    result.err("Could not find script for class: " + name_of_class)
    return null

func instantiate_class(result: Result, name_of_class):
    if name_of_class.is_empty():
        result.err("Cannot instantiate class: name is empty")
        return null
    
    var instance = null
    if ClassDB.class_exists(name_of_class):
        if ClassDB.can_instantiate(name_of_class):
            instance = ClassDB.instantiate(name_of_class)
            if instance == null:
                result.err("ClassDB.instantiate() returned null for class: " + name_of_class)
        else:
            result.err("Class exists but cannot be instantiated: " + name_of_class)
            result.err("This may be an abstract class or interface that cannot be directly instantiated")
    else:
        var script = get_script_by_name(result, name_of_class)
        if script is GDScript:
            instance = script.new()
        else:
            result.err("Failed to get script for class: " + name_of_class)
            return null
    
    if instance == null:
        result.err("Failed to instantiate class: " + name_of_class)
    return instance

func _ensure_scene_directory(result: Result, full_scene_path):
    var scene_dir_res = full_scene_path.get_base_dir()
    var scene_dir_abs = ProjectSettings.globalize_path(scene_dir_res)
    var scene_dir_relative = scene_dir_res.substr(6)
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

# The engine cannot author an inherited scene at runtime (pack flattens instances), so the minimal file is written directly.
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
    _reconcile_path(full_scene_path)
    result.out("Scene created successfully at: " + project_relative(full_scene_path))

func create_scene(result: Result, params) -> Result:
    result.out("Creating scene: " + params.scenePath)
    
    var full_scene_path = params.scenePath
    if not full_scene_path.begins_with("res://"):
        full_scene_path = "res://" + full_scene_path
    if not _ensure_scene_directory(result, full_scene_path):
        return result.fail()

    var base_scene_path = str(params.get("baseScenePath", ""))
    if not base_scene_path.is_empty():
        _create_inherited_scene(result, full_scene_path, base_scene_path)
        return result

    var root_node_type = "Node2D"
    if params.has("rootNodeType"):
        root_node_type = params.rootNodeType
    var scene_root = track_temp(instantiate_class(result, root_node_type))
    if not scene_root:
        result.err("Failed to instantiate node of type: " + root_node_type)
        result.err("Make sure the class exists and can be instantiated")
        result.err("Check if the class is registered in ClassDB or available as a script")
        return result.fail()
    
    scene_root.name = "root"
    var packed_scene = PackedScene.new()
    var pack_error = packed_scene.pack(scene_root)
    if pack_error == OK:
        var save_error = ResourceSaver.save(packed_scene, full_scene_path)
        if save_error == OK:
            var file_exists = FileAccess.file_exists(full_scene_path)
            if file_exists:
                _reconcile_path(full_scene_path)
                result.out("Scene created successfully at: " + params.scenePath)
            else:
                result.err("Failed to create scene: " + params.scenePath)
                return result.fail()
        else:
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
    
    var scene = load_object(full_scene_path, "PackedScene")
    if not scene:
        result.err("Failed to load scene: " + full_scene_path)
        return result.fail()
    
    var scene_root = track_temp(scene.instantiate())
    var parent_path = "root"
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
        # instantiate() stamps the root's scene_file_path, so packing records an ExtResource-backed instance.
        var full_instance_path = instance_path
        if not full_instance_path.begins_with("res://"):
            full_instance_path = "res://" + full_instance_path
        if not FileAccess.file_exists(full_instance_path):
            result.err("Instance scene does not exist at: " + full_instance_path)
            return result.fail()
        var packed_instance = load_object(full_instance_path, "PackedScene")
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
            _reconcile_path(full_scene_path)
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

func export_mesh_library(result: Result, params) -> Result:
    result.out("Exporting MeshLibrary from scene: " + params.scenePath)
    
    var full_scene_path = params.scenePath
    if not full_scene_path.begins_with("res://"):
        full_scene_path = "res://" + full_scene_path
    
    # Absolute host paths and user:// are kept as-is; prefixing res:// would be invalid.
    var full_output_path = params.outputPath
    if not full_output_path.begins_with("res://") and not full_output_path.begins_with("user://") and not full_output_path.is_absolute_path():
        full_output_path = "res://" + full_output_path
    
    var file_check = FileAccess.file_exists(full_scene_path)
    if not file_check:
        result.err("Scene file does not exist at: " + full_scene_path)
        var absolute_path = ProjectSettings.globalize_path(full_scene_path)
        result.err("Absolute file path that doesn't exist: " + absolute_path)
        return result.fail()
    
    var scene = load_object(full_scene_path, "PackedScene")
    if not scene:
        result.err("Failed to load scene: " + full_scene_path)
        return result.fail()
    
    var scene_root = track_temp(scene.instantiate())
    var mesh_library = MeshLibrary.new()
    var mesh_item_names = params.meshItemNames if params.has("meshItemNames") else []
    var use_specific_items = mesh_item_names.size() > 0
    
    var item_id = 0
    for child in scene_root.get_children():
        if use_specific_items and not (child.name in mesh_item_names):
            continue
            
        var mesh_instance = null
        if child is MeshInstance3D:
            mesh_instance = child
        else:
            for descendant in child.get_children():
                if descendant is MeshInstance3D:
                    mesh_instance = descendant
                    break
        
        if mesh_instance and mesh_instance.mesh:
            mesh_library.create_item(item_id)
            mesh_library.set_item_name(item_id, child.name)
            mesh_library.set_item_mesh(item_id, mesh_instance.mesh)
            for collision_child in child.get_children():
                if collision_child is CollisionShape3D and collision_child.shape:
                    mesh_library.set_item_shapes(item_id, [collision_child.shape])
                    break
            
            # MeshLibrary previews must be textures; the albedo texture is the only one available at runtime.
            var preview_texture = _mesh_preview_texture(mesh_instance.mesh)
            if preview_texture:
                mesh_library.set_item_preview(item_id, preview_texture)
            item_id += 1
    var output_dir = full_output_path.get_base_dir()
    if full_output_path.begins_with("res://"):
        var dir = DirAccess.open("res://")
        if dir == null:
            result.err("Failed to open res:// directory")
            result.err("DirAccess error: " + str(DirAccess.get_open_error()))
            return result.fail()
        if output_dir != "res://" and not dir.dir_exists(output_dir.substr(6)):
            var make_error = dir.make_dir_recursive(output_dir.substr(6))
            if make_error != OK:
                result.err("Failed to create directory: " + output_dir + ", error: " + str(make_error))
                return result.fail()
    elif not DirAccess.dir_exists_absolute(output_dir):
        var make_error = DirAccess.make_dir_recursive_absolute(output_dir)
        if make_error != OK:
            result.err("Failed to create directory: " + output_dir + ", error: " + str(make_error))
            return result.fail()
    
    if item_id > 0:
        var error = ResourceSaver.save(mesh_library, full_output_path)
        if error == OK:
            _reconcile_path(full_output_path)
            result.out("MeshLibrary exported successfully with " + str(item_id) + " items to: " + full_output_path)
        else:
            result.err("Failed to save MeshLibrary: " + str(error))
            return result.fail()
    else:
        result.err("No valid meshes found in the scene")
        return result.fail()
    return result

func _mesh_preview_texture(mesh) -> Texture2D:
    if not mesh:
        return null
    for surface_index in range(mesh.get_surface_count()):
        var material = mesh.surface_get_material(surface_index)
        if material is BaseMaterial3D and material.albedo_texture:
            return material.albedo_texture
    return null

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

    # A non-PackedScene is a controlled failure rather than an instantiate() SCRIPT ERROR.
    var scene = load_resource(full_scene_path)
    if scene == null:
        return result.fail()
    if not (scene is PackedScene):
        result.err("Not a scene file: " + full_scene_path + " (use read_binary_resource for binary resources)")
        return result.fail()

    var scene_root = track_temp(scene.instantiate())
    if scene_root == null:
        result.err("Failed to instantiate scene: " + full_scene_path)
        return result.fail()

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

    var depth = -1
    if params.has("depth") and params.depth != null:
        depth = int(params.depth)

    var tree_data = _walk_scene_tree(start_node, depth)

    # The in-memory tree cannot reveal inheritance; the raw file is the only signal.
    var base_scene_path = _inherited_scene_base(full_scene_path)
    if not base_scene_path.is_empty():
        tree_data["baseScenePath"] = base_scene_path

    tree_data["connections"] = _scene_connections(FileAccess.get_file_as_string(full_scene_path))

    result.out(JSON_START)
    result.out(JSON.stringify(tree_data))
    result.out(JSON_END)

    return result

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

# The name must start its own attribute, so an id lookup does not match inside uid=.
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

# pack() cannot re-serialize an inherited root as an instance; it would replace the base reference with a full copy.
func _reject_inherited_scene(result: Result, full_scene_path):
    var base_scene_path = _inherited_scene_base(full_scene_path)
    if base_scene_path.is_empty():
        return false
    result.err("Cannot edit inherited scene: " + full_scene_path)
    result.err("Its root instances " + base_scene_path + "; editing it would flatten the base scene into this file")
    result.err("Edit the base scene instead, or recreate this scene with create_scene")
    return true

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

# Escape hatch for values with no structured mapping; var_to_str is both complete and reparsable.
func _godot_text(value):
    return {"$type": "GodotText", "text": var_to_str(value)}

func _apply_properties(target, properties) -> DecodeResult:
    for prop_name in properties:
        var decoded = _convert_property_value(target, prop_name, properties[prop_name])
        if not decoded.ok:
            return decoded
        target.set(prop_name, decoded.value)
    return DecodeResult.success(null)

func _convert_property_value(target, prop_name, value) -> DecodeResult:
    for prop in target.get_property_list():
        if prop["name"] == prop_name:
            return _convert_value_for_type(prop.get("type", 0), value)
    return _decode_value(value)

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
            if value is String and (value.begins_with("res://") or value.begins_with("user://")):
                return _decode_resource_path(value)
            return DecodeResult.failure("expected a res:// path or a tagged value for an object property")
    return _decode_value(value)

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

func _decoded(decoded, results) -> DecodeResult:
    var failure = _last_failure(results)
    if failure != null:
        return failure
    return DecodeResult.success(decoded)

func _last_failure(results):
    var failure = null
    for result in results:
        if not result.ok:
            failure = result
    return failure

func _typed_field(value, field, tag) -> DecodeResult:
    var inner = value.get(field, null)
    if inner is Dictionary and inner.get("$type", "") == tag:
        return _decode_typed(inner)
    return DecodeResult.failure("field \"" + field + "\" must be a {\"$type\": \"" + tag + "\"} value")

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

func _decode_resource(value) -> DecodeResult:
    var path = value.get("path", null)
    if not (path is String):
        return DecodeResult.failure("$type Resource needs a \"path\" string")
    return _decode_resource_path(path)

func _decode_resource_path(path) -> DecodeResult:
    var res = load_resource(path)
    if res != null:
        return DecodeResult.success(res)
    return DecodeResult.failure("failed to load resource: " + path)

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

    var scene = load_object(full_scene_path, "PackedScene")
    if not scene:
        result.err("Failed to load scene: " + full_scene_path)
        return result.fail()

    var scene_root = track_temp(scene.instantiate())

    var node_path = params.nodePath
    var target = scene_root
    if node_path != "root" and node_path != ".":
        if node_path.begins_with("root/"):
            node_path = node_path.substr(5)
        target = scene_root.get_node_or_null(node_path)

    if target == null:
        result.err("Node not found: " + params.nodePath)
        return result.fail()

    var applied = _apply_properties(target, params.properties)
    if not applied.ok:
        result.err("Invalid property value: " + applied.error)
        return result.fail()

    var packed_scene = PackedScene.new()
    var pack_error = packed_scene.pack(scene_root)
    if pack_error != OK:
        result.err("Failed to pack scene after modification: " + str(pack_error))
        return result.fail()

    var save_error = ResourceSaver.save(packed_scene, full_scene_path)
    if save_error != OK:
        result.err("Failed to save modified scene: " + str(save_error))
        return result.fail()

    _reconcile_path(full_scene_path)
    result.out("Node modified successfully in: " + full_scene_path)
    return result

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

    var scene = load_object(full_scene_path, "PackedScene")
    if not scene:
        result.err("Failed to load scene: " + full_scene_path)
        return result.fail()

    var scene_root = track_temp(scene.instantiate())

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
    # The detached node is out of reach of scene_root cleanup, so track it explicitly.
    track_temp(target)

    var packed_scene = PackedScene.new()
    var pack_error = packed_scene.pack(scene_root)
    if pack_error != OK:
        result.err("Failed to pack scene after removal: " + str(pack_error))
        return result.fail()

    var save_error = ResourceSaver.save(packed_scene, full_scene_path)
    if save_error != OK:
        result.err("Failed to save scene after removal: " + str(save_error))
        return result.fail()

    _reconcile_path(full_scene_path)
    result.out("Node '" + removed_name + "' removed successfully from: " + full_scene_path)
    return result

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

    var scene = load_object(full_path, "PackedScene") as PackedScene
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
        # Clear the owner before reparenting or the engine warns it is inconsistent.
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
    _reconcile_path(full_path)
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

    var scene = load_object(full_scene_path, "PackedScene")
    if not scene:
        result.err("Failed to load scene: " + full_scene_path)
        return result.fail()

    var scene_root = track_temp(scene.instantiate())

    var node_path = params.nodePath
    var target = scene_root
    if node_path != "root" and node_path != ".":
        if node_path.begins_with("root/"):
            node_path = node_path.substr(5)
        target = scene_root.get_node_or_null(node_path)

    if target == null:
        result.err("Node not found: " + params.nodePath)
        return result.fail()

    # A non-Script resource must fail here; set_script's engine error only reaches stderr.
    var script = load_object(full_script_path, "Script")
    if not (script is Script):
        result.err("Parameter should be null or a reference to a valid script")
        return result.fail()

    target.set_script(script)

    var packed_scene = PackedScene.new()
    var pack_error = packed_scene.pack(scene_root)
    if pack_error != OK:
        result.err("Failed to pack scene after attaching script: " + str(pack_error))
        return result.fail()

    var save_error = ResourceSaver.save(packed_scene, full_scene_path)
    if save_error != OK:
        result.err("Failed to save scene after attaching script: " + str(save_error))
        return result.fail()

    _reconcile_path(full_script_path)
    _reconcile_path(full_scene_path)
    result.out("Script '" + full_script_path + "' attached successfully to node in: " + full_scene_path)
    return result

# Compiles the caller's source in-memory and runs it inside the live editor
# session. The engine default logger still prints to stdout; ScriptRunLogger
# observes the same stream so logs and errors can be returned in the frame.
func run_script(result: Result, params) -> Result:
    if not params.has("script") or not (params.script is String):
        result.err("script is required")
        return result.fail()
    var source: String = params.script
    if source.strip_edges().is_empty():
        result.err("script is empty")
        return result.fail()

    # The editor refuses to instantiate a non-@tool script, so force tool mode
    # unless the caller already opted in.
    var injected_tool := not source.contains("@tool")
    if injected_tool:
        source = "@tool\n" + source

    var logger := ScriptRunLogger.new()
    logger.limit = RUN_SCRIPT_LOG_LIMIT
    OS.add_logger(logger)

    var compiled := false
    var value = null
    var serialized = null
    var script := GDScript.new()
    script.source_code = source
    # An in-memory script has no resource path, so reload() is the only parse;
    # parse failures surface through the logger's _log_error.
    var reload_error := script.reload()
    if reload_error == OK and script.can_instantiate():
        compiled = true
        var instance = script.new()
        if instance == null:
            compiled = false
            result.err("script did not instantiate")
        else:
            # run() is the value-returning entry; _initialize() lets a
            # SceneTree-shaped script run as Godot would with --script.
            if instance.has_method("run"):
                value = instance.call("run")
            elif instance.has_method("_initialize"):
                instance.call("_initialize")
            serialized = _variant_to_json(value)
            # A non-RefCounted instance (Node, SceneTree) would otherwise leak.
            if not (instance is RefCounted):
                instance.free()

    OS.remove_logger(logger)

    # The injected @tool line shifts every line in the in-memory script by one;
    # report diagnostics against the caller's source instead.
    if injected_tool:
        for captured in logger.errors:
            if str(captured.get("file", "")).begins_with("gdscript://") and int(captured.get("line", 0)) > 0:
                captured["line"] = int(captured["line"]) - 1

    result.out(JSON_START)
    result.out(JSON.stringify({
        "compiled": compiled,
        "result": serialized,
        "logs": Array(logger.messages),
        "errors": logger.errors,
    }))
    result.out(JSON_END)
    return result

func manage_resource(result: Result, params) -> Result:
    var resource_path = params.get("resourcePath", "")
    var action = params.get("action", "read")
    var full_path = resource_path
    if not full_path.begins_with("res://"):
        full_path = "res://" + full_path

    if action == "read":
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
    else:
        result.err("Unknown manage_resource action: " + action)
        return result.fail()
    return result


func _matches_property_filter(prop_name, filter) -> bool:
    for entry in filter:
        var pattern = str(entry)
        if pattern == prop_name or prop_name.match(pattern):
            return true
    return false


func _load_resource(result: Result, full_path):
    if not ResourceLoader.exists(full_path):
        result.err("Resource not found: " + full_path)
        result.ok = false
        return null
    var resource = load_resource(full_path)
    if resource == null:
        result.err("Failed to load resource: " + full_path)
        result.ok = false
        return null
    return resource

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

func manage_animation(result: Result, params) -> Result:
    var resource_path = params.get("resourcePath", "")
    var full_path = resource_path
    if not full_path.begins_with("res://"):
        full_path = "res://" + full_path

    _read_animation(result, full_path)
    return result

func _load_animation(result: Result, full_path):
    if not ResourceLoader.exists(full_path):
        result.err("Animation resource not found: " + full_path)
        result.ok = false
        return null
    var resource = load_object(full_path, "Animation")
    if resource == null:
        result.err("Failed to load animation: " + full_path)
        result.ok = false
        return null
    if not (resource is Animation):
        result.err("Not an Animation resource: " + full_path)
        result.ok = false
        return null
    return resource

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

func manage_tileset(result: Result, params) -> Result:
    var resource_path = params.get("resourcePath", "")
    var full_path = resource_path
    if not full_path.begins_with("res://"):
        full_path = "res://" + full_path

    _read_tileset(result, full_path)
    return result

func _load_tileset(result: Result, full_path):
    if not ResourceLoader.exists(full_path):
        result.err("TileSet resource not found: " + full_path)
        result.ok = false
        return null
    var resource = load_object(full_path, "TileSet")
    if resource == null:
        result.err("Failed to load tileset: " + full_path)
        result.ok = false
        return null
    if not (resource is TileSet):
        result.err("Not a TileSet resource: " + full_path)
        result.ok = false
        return null
    return resource

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

func _tile_data_properties(tile_data) -> Dictionary:
    var props = {}
    if tile_data == null:
        return props
    for prop in tile_data.get_property_list():
        if prop["usage"] & PROPERTY_USAGE_STORAGE:
            props[prop["name"]] = tile_data.get(prop["name"])
    return props

func _source_resource_path(resource) -> String:
    if resource == null or resource.resource_path == "":
        return ""
    return project_relative(resource.resource_path)

func _enum_name(mapping, value) -> String:
    for name in mapping:
        if mapping[name] == value:
            return name
    return "unknown"

func manage_theme(result: Result, params) -> Result:
    var resource_path = params.get("resourcePath", "")
    var full_path = resource_path
    if not full_path.begins_with("res://"):
        full_path = "res://" + full_path

    _read_theme(result, full_path)
    return result

func _load_theme(result: Result, full_path):
    if not ResourceLoader.exists(full_path):
        result.err("Theme resource not found: " + full_path)
        result.ok = false
        return null
    var resource = load_object(full_path, "Theme")
    if resource == null:
        result.err("Failed to load theme: " + full_path)
        result.ok = false
        return null
    if not (resource is Theme):
        result.err("Not a Theme resource: " + full_path)
        result.ok = false
        return null
    return resource

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

func manage_tilemap(result: Result, params) -> Result:
    var scene_path = params.get("scenePath", "")
    var node_path = str(params.get("nodePath", ""))
    var full_path = scene_path
    if not full_path.begins_with("res://"):
        full_path = "res://" + full_path

    if not FileAccess.file_exists(full_path):
        result.err("Scene file does not exist at: " + full_path)
        return result.fail()
    var scene = load_object(full_path, "PackedScene")
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

    _read_tilemap_node(result, target, full_path, node_path)
    return result

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

# Starting the engine against the project is the whole check; the caller reads the engine's stderr and exit code.
func validate_project(result: Result, _params) -> Result:
    return result
