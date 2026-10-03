extends SceneTree

# Generic verifier for the evals suite. Reads res://evals_checks.json, evaluates
# each check against the project as loaded by the pinned Godot engine, and
# prints a single JSON line prefixed with EVALS_RESULT. Run with:
#   godot --headless --path <project> --script res://evals_verify.gd


func _initialize() -> void:
	var file := FileAccess.open("res://evals_checks.json", FileAccess.READ)
	if file == null:
		_emit({"error": "cannot open res://evals_checks.json"})
		quit(1)
		return
	var parsed: Variant = JSON.parse_string(file.get_as_text())
	file.close()
	if typeof(parsed) != TYPE_ARRAY:
		_emit({"error": "checks file is not a JSON array"})
		quit(1)
		return
	var results: Array = []
	for check: Variant in parsed:
		results.append(_run_check(check as Dictionary))
	_emit({"results": results})
	quit(0)


func _emit(payload: Dictionary) -> void:
	print("EVALS_RESULT " + JSON.stringify(payload))


func _run_check(check: Dictionary) -> Dictionary:
	var kind := String(check.get("kind", ""))
	var id := String(check.get("id", kind))
	var passed := false
	var detail := ""
	match kind:
		"node_exists":
			var node := _node(check)
			passed = node != null
			detail = "node=%s" % ("found" if passed else "missing")
		"node_not_exists":
			passed = _node(check) == null
			detail = "node=%s" % ("absent" if passed else "still present")
		"node_type":
			var node := _node(check)
			passed = node != null and _type_matches(node, String(check.get("value", "")))
			detail = "type=%s want=%s" % ["" if node == null else node.get_class(), check.get("value", "")]
		"node_property":
			var node := _node(check)
			var actual: Variant = null if node == null else node.get(String(check.get("property", "")))
			passed = node != null and _values_equal(actual, check.get("value"))
			detail = "actual=%s want=%s" % [str(actual), str(check.get("value"))]
		"node_script":
			var node := _node(check)
			var script: Variant = null if node == null else node.get_script()
			var path := "" if script == null else String(script.resource_path)
			passed = node != null and path.ends_with(String(check.get("value", "")))
			detail = "script=%s" % path
		"scene_root_type":
			var root := _load_scene(check)
			passed = root != null and _type_matches(root, String(check.get("value", "")))
			detail = "root=%s want=%s" % ["" if root == null else root.get_class(), check.get("value", "")]
		"file_exists":
			passed = FileAccess.file_exists(String(check.get("path", "")))
			detail = "path=%s" % check.get("path", "")
		"file_contains":
			var path := String(check.get("path", ""))
			var text := _read_text(path)
			passed = text.find(String(check.get("value", ""))) != -1
			detail = "path=%s" % path
		"script_has_method":
			passed = _script_has_method(String(check.get("path", "")), String(check.get("value", "")))
			detail = "path=%s method=%s" % [check.get("path", ""), check.get("value", "")]
		"project_setting":
			var actual: Variant = ProjectSettings.get_setting(String(check.get("key", "")))
			passed = _values_equal(actual, check.get("value"))
			detail = "actual=%s want=%s" % [str(actual), str(check.get("value"))]
		_:
			detail = "unknown check kind %s" % kind
	return {"id": id, "kind": kind, "passed": passed, "detail": detail}


func _node(check: Dictionary) -> Node:
	var root := _load_scene(check)
	if root == null:
		return null
	var path := String(check.get("path", ""))
	if path == "" or path == ".":
		return root
	return root.get_node_or_null(NodePath(path))


func _load_scene(check: Dictionary) -> Node:
	var scene_path := String(check.get("scene", ""))
	if scene_path == "" or not ResourceLoader.exists(scene_path):
		return null
	var resource: Variant = load(scene_path)
	if resource is PackedScene:
		return (resource as PackedScene).instantiate()
	if resource is Node:
		return resource as Node
	return null


func _type_matches(node: Node, want: String) -> bool:
	if want == "":
		return false
	if node.get_class() == want:
		return true
	var script: Variant = node.get_script()
	if script != null and String(script.get_global_name()) == want:
		return true
	return false


func _script_has_method(path: String, method: String) -> bool:
	if path == "" or not ResourceLoader.exists(path):
		return false
	var resource: Variant = load(path)
	if not (resource is Script):
		return false
	for entry: Dictionary in (resource as Script).get_script_method_list():
		if String(entry.get("name", "")) == method:
			return true
	return false


func _read_text(path: String) -> String:
	if path == "" or not FileAccess.file_exists(path):
		return ""
	var file := FileAccess.open(path, FileAccess.READ)
	if file == null:
		return ""
	var text := file.get_as_text()
	file.close()
	return text


func _values_equal(actual: Variant, expected: Variant) -> bool:
	if (actual is float or actual is int) and (expected is float or expected is int):
		return absf(float(actual) - float(expected)) <= 0.0001
	if expected is bool:
		return bool(actual) == bool(expected)
	var a := str(actual)
	var e := str(expected)
	# Preserve spaces for plain strings; only normalize typed values such as
	# "Vector2(1, 2)" so their string formatting differences do not matter.
	if _is_typed_value(a) or _is_typed_value(e):
		return _normalize(a) == _normalize(e)
	return a == e


func _is_typed_value(value: String) -> bool:
	return (
		value.begins_with("Vector")
		or value.begins_with("Color")
		or value.begins_with("Rect")
		or value.begins_with("Transform")
		or value.begins_with("Quaternion")
		or value.begins_with("Basis")
		or value.begins_with("Plane")
		or value.begins_with("AABB")
		or value.begins_with("(")
	)


func _normalize(value: String) -> String:
	var stripped := (
		value
		.replace("Vector2", "")
		.replace("Vector3", "")
		.replace("Vector4", "")
		.replace("Color", "")
		.replace(" ", "")
	)
	# Godot stringifies whole floats with a trailing ".0"; drop it inside
	# typed values so "(10.0, 20.0)" matches "(10, 20)".
	var re := RegEx.new()
	re.compile("\\.0(?=[,)\\]])")
	return re.sub(stripped, "", true)
