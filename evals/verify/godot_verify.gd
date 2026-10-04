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
			var property := String(check.get("property", ""))
			var actual: Variant = _read_property(node, property)
			passed = node != null and actual != null and _values_equal(actual, check.get("value"))
			detail = "%s=%s want=%s" % [property, str(actual), str(check.get("value"))]
		"resource_property":
			var resource := _load_resource(String(check.get("path", "")))
			var property := String(check.get("property", ""))
			var actual: Variant = _read_property(resource, property)
			passed = resource != null and actual != null and _values_equal(actual, check.get("value"))
			detail = "%s=%s want=%s" % [property, str(actual), str(check.get("value"))]
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
		"resource_distinct":
			var distinct := _resource_identity(check, false)
			passed = bool(distinct.get("passed", false))
			detail = String(distinct.get("detail", ""))
		"resource_same":
			var same := _resource_identity(check, true)
			passed = bool(same.get("passed", false))
			detail = String(same.get("detail", ""))
		"resource_count":
			var count := _resource_count(check)
			passed = bool(count.get("passed", false))
			detail = String(count.get("detail", ""))
		"node_instance":
			var instance := _node_instance(check)
			passed = bool(instance.get("passed", false))
			detail = String(instance.get("detail", ""))
		"node_unchanged":
			var unchanged := _node_unchanged(check)
			passed = bool(unchanged.get("passed", false))
			detail = String(unchanged.get("detail", ""))
		"animation_exists":
			var animation := _animation_exists(check)
			passed = bool(animation.get("passed", false))
			detail = String(animation.get("detail", ""))
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


func _node_at(root: Node, path: String) -> Node:
	if root == null:
		return null
	if path == "" or path == ".":
		return root
	return root.get_node_or_null(NodePath(path))


# Resource identity must not use Object ``==``: GDScript's ``_operator==`` can be
# overridden (Resource compares by value in some subclasses), so two distinct
# instances can compare equal. ``is_same`` / ``get_instance_id`` compare the
# actual object.
func _resource_identity(check: Dictionary, want_same: bool) -> Dictionary:
	var root := _load_scene(check)
	if root == null:
		return {"passed": false, "detail": "scene did not load"}
	var path_a := String(check.get("path_a", ""))
	var path_b := String(check.get("path_b", ""))
	var res_a: Variant = _node_resource(root, path_a, String(check.get("property_a", "")))
	var res_b: Variant = _node_resource(root, path_b, String(check.get("property_b", "")))
	if res_a == null or res_b == null:
		return {"passed": false, "detail": "missing resource (a=%s b=%s)" % [path_a, path_b]}
	var same := _same_instance(res_a, res_b)
	var passed := same == want_same
	return {
		"passed": passed,
		"detail": "a=%s b=%s %s" % [path_a, path_b, "same" if same else "distinct"],
	}


func _node_resource(root: Node, path: String, property: String) -> Variant:
	var node := _node_at(root, path)
	if node == null:
		return null
	if property == "":
		return node
	return _read_property(node, property)


func _same_instance(a: Variant, b: Variant) -> bool:
	if a == null or b == null:
		return false
	if not (a is Object) or not (b is Object):
		return false
	var obj_a := a as Object
	var obj_b := b as Object
	if is_same(obj_a, obj_b):
		return true
	return obj_a.get_instance_id() == obj_b.get_instance_id()


func _resource_count(check: Dictionary) -> Dictionary:
	var scene_path := String(check.get("scene", ""))
	var type := String(check.get("type", ""))
	var text := _read_text(scene_path)
	if text == "":
		return {"passed": false, "detail": "scene text unreadable: %s" % scene_path}
	var count := 0
	var marker := "[sub_resource type=\"%s\"" % type
	var search_from := 0
	while true:
		var found := text.find(marker, search_from)
		if found == -1:
			break
		count += 1
		search_from = found + marker.length()
	# Corroborate with a load so a corrupt/unloadable scene that happens to
	# contain the marker text does not pass.
	if not ResourceLoader.exists(scene_path) or load(scene_path) == null:
		return {"passed": false, "detail": "scene did not load: %s" % scene_path}
	var min_count := int(check.get("min", -1))
	var max_count := int(check.get("max", -1))
	var passed := true
	if min_count >= 0 and count < min_count:
		passed = false
	if max_count >= 0 and count > max_count:
		passed = false
	return {
		"passed": passed,
		"detail": "count=%d min=%s max=%s" % [
			count,
			check.get("min", "-"),
			check.get("max", "-"),
		],
	}


func _node_instance(check: Dictionary) -> Dictionary:
	var root := _load_scene(check)
	if root == null:
		return {"passed": false, "detail": "scene did not load"}
	var path := String(check.get("path", ""))
	var node := _node_at(root, path)
	if node == null:
		return {"passed": false, "detail": "node %s missing" % path}
	var want := String(check.get("scene_file", ""))
	var scene_file := String(node.scene_file_path)
	var passed := want != "" and scene_file == want
	return {
		"passed": passed,
		"detail": "scene_file=%s want=%s" % [scene_file, want],
	}


func _node_unchanged(check: Dictionary) -> Dictionary:
	var root := _load_scene(check)
	if root == null:
		return {"passed": false, "detail": "scene did not load"}
	var path := String(check.get("path", ""))
	var node := _node_at(root, path)
	if node == null:
		return {"passed": false, "detail": "node %s missing" % path}
	var fixture_path := _fixture_scene_path(check)
	if fixture_path == "" or not ResourceLoader.exists(fixture_path):
		return {"passed": false, "detail": "fixture scene missing: %s" % fixture_path}
	var fixture_resource: Variant = load(fixture_path)
	var fixture_root: Node = null
	if fixture_resource is PackedScene:
		fixture_root = (fixture_resource as PackedScene).instantiate()
	elif fixture_resource is Node:
		fixture_root = fixture_resource as Node
	var fixture_node := _node_at(fixture_root, path)
	if fixture_node == null:
		return {"passed": false, "detail": "fixture node %s missing" % path}
	var diffs: Array = []
	for entry: Dictionary in fixture_node.get_property_list():
		var property_name := String(entry.get("name", ""))
		if not _is_persisted_property(property_name):
			continue
		var want: Variant = fixture_node.get(property_name)
		var actual: Variant = node.get(property_name)
		if not _values_equal(actual, want):
			diffs.append("%s: %s != %s" % [property_name, str(actual), str(want)])
	var passed := diffs.is_empty()
	return {
		"passed": passed,
		"detail": "unchanged" if passed else "diffs: %s" % ", ".join(diffs),
	}


# Serialized user/script properties only; skip engine internals (``_``-prefixed),
# identity fields (name/type/owner) and the script so a rename or a different
# instance of the same node is not reported as a value change.
func _is_persisted_property(property_name: String) -> bool:
	if property_name.begins_with("_"):
		return false
	return not (
		property_name == "name"
		or property_name == "type"
		or property_name == "owner"
		or property_name == "script"
	)


func _animation_exists(check: Dictionary) -> Dictionary:
	var root := _load_scene(check)
	if root == null:
		return {"passed": false, "detail": "scene did not load"}
	var node := _node_at(root, String(check.get("node", "")))
	if node == null or not (node is AnimationPlayer):
		return {"passed": false, "detail": "no AnimationPlayer at %s" % check.get("node", "")}
	var player := node as AnimationPlayer
	var animation_name := String(check.get("name", ""))
	if not player.has_animation(animation_name):
		return {"passed": false, "detail": "animation %s missing" % animation_name}
	var animation: Animation = player.get_animation(animation_name)
	var length := float(animation.length)
	var want_length := float(check.get("length", -1.0))
	var want_tracks := int(check.get("tracks", -1))
	var has_length := check.has("length")
	var passed := true
	if has_length and absf(length - want_length) > 0.0001:
		passed = false
	if want_tracks >= 0 and animation.get_track_count() != want_tracks:
		passed = false
	var expected_paths: Array = check.get("paths", [])
	var actual_paths := _animation_track_paths(animation)
	for expected: Variant in expected_paths:
		if not actual_paths.has(String(expected)):
			passed = false
	return {
		"passed": passed,
		"detail": "length=%s tracks=%d paths=%s" % [
			str(length),
			animation.get_track_count(),
			str(actual_paths),
		],
	}


func _animation_track_paths(animation: Animation) -> Array:
	var paths: Array = []
	for track: int in range(animation.get_track_count()):
		var path := animation.track_get_path(track)
		paths.append(String(path))
	return paths


func _fixture_scene_path(check: Dictionary) -> String:
	var scene := String(check.get("scene", ""))
	if scene == "":
		return ""
	var relative := scene.trim_prefix("res://")
	return "res://evals_fixture/%s" % relative


# A dotted "property" walks nested objects, e.g. "shape.size" reads the embedded
# Resource on a CollisionShape2D. A missing step yields null, which fails the
# check.
func _read_property(root: Object, property: String) -> Variant:
	if root == null:
		return null
	var current: Variant = root
	for part: String in property.split("."):
		if current == null or not (current is Object):
			return null
		if not _has_property(current as Object, part):
			return null
		current = current.get(part)
	return current


func _has_property(obj: Object, property: String) -> bool:
	for entry: Dictionary in obj.get_property_list():
		if String(entry.get("name", "")) == property:
			return true
	return false


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


func _load_resource(path: String) -> Object:
	if path == "" or not ResourceLoader.exists(path):
		return null
	var resource: Variant = load(path)
	if resource is Object:
		return resource as Object
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
