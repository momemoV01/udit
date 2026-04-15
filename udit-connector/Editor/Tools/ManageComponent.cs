using System;
using System.Collections.Generic;
using System.Globalization;
using System.Linq;
using Newtonsoft.Json.Linq;
using UditConnector.Tools.Common;
using UnityEditor;
using UnityEditor.SceneManagement;
using UnityEngine;

namespace UditConnector.Tools
{
    [UditTool(Description = "Query and mutate components. Actions: list, get, schema, add, remove, set, copy.")]
    public static class ManageComponent
    {
        // SerializedPropertyType cases that `component set` does not yet
        // implement. Each new write-handler removes its entry; the rejection
        // message in TryParseValueForProperty's default branch is computed
        // from this set so each commit's diff is a single-line removal.
        // ExposedReference stays after v0.9.0 (different concept — prefab
        // variant resolution context — likely never set this way from CLI).
        static readonly System.Collections.Generic.HashSet<SerializedPropertyType> s_UnsupportedSet = new()
        {
            SerializedPropertyType.ManagedReference,
            SerializedPropertyType.ExposedReference,
        };

        public class Parameters
        {
            [ToolParameter("Action to perform: list, get, schema, add, remove, set, copy", Required = true)]
            public string Action { get; set; }

            [ToolParameter("Stable ID (go:XXXXXXXX) — required for list, get, add, remove, set, copy")]
            public string Id { get; set; }

            [ToolParameter("Component type name — required for get, schema, add, remove, set, copy")]
            public string Type { get; set; }

            [ToolParameter("Dotted field path to read (get) or write (set). For get, omit to dump every field.")]
            public string Field { get; set; }

            [ToolParameter("Value for set. Parsed based on the target field's SerializedPropertyType.")]
            public string Value { get; set; }

            [ToolParameter("Zero-based index when the GameObject has multiple components of the same type (default 0)")]
            public int Index { get; set; }

            [ToolParameter("Destination GameObject stable ID (copy only)")]
            public string DstId { get; set; }

            [ToolParameter("Dry-run: report what would change without mutating")]
            public bool DryRun { get; set; }
        }

        public static object HandleCommand(JObject @params)
        {
            if (@params == null)
                return new ErrorResponse(ErrorCodes.InvalidParams, "Parameters cannot be null.");

            var p = new ToolParams(@params);
            var actionResult = p.GetRequired("action");
            if (!actionResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, actionResult.ErrorMessage);

            var action = actionResult.Value.ToLowerInvariant();
            switch (action)
            {
                case "list":   return List(p);
                case "get":    return Get(p);
                case "schema": return Schema(p);
                case "add":    return Add(p);
                case "remove": return Remove(p);
                case "set":    return Set(p);
                case "copy":   return Copy(p);
                default:
                    return new ErrorResponse(ErrorCodes.InvalidParams,
                        $"Unknown action '{action}'. Available: list, get, schema, add, remove, set, copy.");
            }
        }

        static object List(ToolParams p)
        {
            var idResult = p.GetRequired("id", "'id' parameter is required for list.");
            if (!idResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, idResult.ErrorMessage);

            if (!StableIdRegistry.TryResolve(idResult.Value, out var go))
                return new ErrorResponse(ErrorCodes.GameObjectNotFound,
                    $"GameObject not found: {idResult.Value}. Run `go find` first if the ID is from a previous session.");

            var components = go.GetComponents<Component>();
            var summaries = new List<object>(components.Length);
            for (int i = 0; i < components.Length; i++)
            {
                var c = components[i];
                summaries.Add(new
                {
                    index = i,
                    type = c == null ? "<Missing Script>" : c.GetType().Name,
                    full_type = c == null ? null : c.GetType().FullName,
                    enabled = (c is Behaviour b) ? (bool?)b.enabled : null,
                });
            }

            return new SuccessResponse(
                $"{components.Length} component(s) on {go.name}.",
                new
                {
                    id = idResult.Value,
                    name = go.name,
                    count = components.Length,
                    components = summaries,
                });
        }

        static object Get(ToolParams p)
        {
            var idResult = p.GetRequired("id", "'id' parameter is required for get.");
            if (!idResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, idResult.ErrorMessage);

            var typeResult = p.GetRequired("type", "'type' parameter is required for get.");
            if (!typeResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, typeResult.ErrorMessage);

            if (!StableIdRegistry.TryResolve(idResult.Value, out var go))
                return new ErrorResponse(ErrorCodes.GameObjectNotFound,
                    $"GameObject not found: {idResult.Value}.");

            var index = Math.Max(0, p.GetInt("index", 0) ?? 0);
            var typeName = typeResult.Value;

            // Match components by short name OR full name, case-insensitive. We
            // keep all matches so --index can pick when multiple of the same
            // type are attached (e.g. a GO with two BoxColliders).
            var matches = new List<Component>();
            foreach (var c in go.GetComponents<Component>())
            {
                if (c == null) continue;
                var t = c.GetType();
                if (string.Equals(t.Name, typeName, StringComparison.OrdinalIgnoreCase) ||
                    string.Equals(t.FullName, typeName, StringComparison.OrdinalIgnoreCase))
                    matches.Add(c);
            }

            if (matches.Count == 0)
            {
                var attached = string.Join(", ", go.GetComponents<Component>()
                    .Where(c => c != null)
                    .Select(c => c.GetType().Name));
                return new ErrorResponse(ErrorCodes.ComponentNotFound,
                    $"Component type '{typeName}' not found on {idResult.Value}. Attached: {attached}.");
            }

            if (index >= matches.Count)
            {
                return new ErrorResponse(ErrorCodes.ComponentNotFound,
                    $"Component index {index} out of range for type '{typeName}' on {idResult.Value} (only {matches.Count} attached).");
            }

            var component = matches[index];
            var field = p.Get("field");

            // Route through SerializedInspect so the field names agents see are
            // exactly the ones returned by `go inspect`. Converting via JObject
            // lets us walk arbitrary dotted paths (e.g. "m_Cameras.elements.0")
            // without a separate resolver on the C# side.
            var dump = SerializedInspect.ComponentToObject(component);
            var jObject = JObject.FromObject(dump);
            var properties = jObject["properties"] as JObject;

            if (string.IsNullOrEmpty(field))
            {
                return new SuccessResponse(
                    $"Component '{component.GetType().Name}' on {go.name}.",
                    new
                    {
                        id = idResult.Value,
                        type = component.GetType().Name,
                        full_type = component.GetType().FullName,
                        index,
                        match_count = matches.Count,
                        enabled = (component is Behaviour b) ? (bool?)b.enabled : null,
                        properties,
                    });
            }

            // Dotted path navigation. Numeric segments index into arrays. Every
            // other segment is a JObject key lookup — it matches the field
            // names used elsewhere in the tool chain, so "position.x" on a
            // Transform resolves to the world-space x coordinate.
            var token = (JToken)properties;
            var visited = new List<string>();
            foreach (var segment in field.Split('.'))
            {
                visited.Add(segment);
                if (token == null) break;

                if (token is JArray arr && int.TryParse(segment, out var arrIndex))
                {
                    if (arrIndex < 0 || arrIndex >= arr.Count) { token = null; break; }
                    token = arr[arrIndex];
                    continue;
                }

                if (token is JObject obj)
                {
                    token = obj[segment];
                    continue;
                }

                token = null;
                break;
            }

            if (token == null)
            {
                var available = properties != null
                    ? string.Join(", ", properties.Properties().Select(pr => pr.Name))
                    : "<no properties>";
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"Field path '{field}' not found on {component.GetType().Name}. Top-level fields: {available}.");
            }

            return new SuccessResponse(
                $"{component.GetType().Name}.{field} on {go.name}.",
                new
                {
                    id = idResult.Value,
                    type = component.GetType().Name,
                    field,
                    value = token,
                });
        }

        static object Schema(ToolParams p)
        {
            var typeResult = p.GetRequired("type", "'type' parameter is required for schema.");
            if (!typeResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, typeResult.ErrorMessage);

            var typeName = typeResult.Value;
            var type = FindComponentType(typeName);
            if (type == null)
                return new ErrorResponse(ErrorCodes.ComponentNotFound,
                    $"Type '{typeName}' not found in loaded assemblies, or not a Component subclass.");

            // Schema v1: probe an existing live instance rather than spawning
            // one, because AddComponent has side effects (RequireComponent
            // chains, internal flags that reject add). Asking the user to have
            // at least one instance in the scene is acceptable for now;
            // building a full reflection-only fallback is a later slice.
            var instance = UnityEngine.Object.FindAnyObjectByType(type, FindObjectsInactive.Include);
            if (instance == null)
            {
                return new ErrorResponse(ErrorCodes.ComponentNotFound,
                    $"No live instance of {type.FullName} in loaded scenes — schema requires a probe instance. " +
                    $"Add one to a scene (or load a scene that has it) and retry.");
            }

            var comp = instance as Component;
            if (comp == null)
            {
                return new ErrorResponse(ErrorCodes.ComponentNotFound,
                    $"Found an instance of {type.FullName} but it is not a Component — schema is Component-only.");
            }

            var fields = new List<object>();
            try
            {
                using var so = new SerializedObject(comp);
                var iter = so.GetIterator();
                bool enterChildren = true;
                while (iter.NextVisible(enterChildren))
                {
                    enterChildren = false;
                    if (iter.name == "m_Script") continue;
                    fields.Add(new
                    {
                        name = iter.name,
                        display_name = iter.displayName,
                        property_type = iter.propertyType.ToString(),
                        is_array = iter.isArray,
                        has_children = iter.hasVisibleChildren,
                    });
                }
            }
            catch (Exception ex)
            {
                // Some internal components throw during iteration; return the
                // partial list plus a note instead of 500-ing.
                return new SuccessResponse(
                    $"Schema for {type.Name} (partial — iteration threw).",
                    new
                    {
                        type = type.Name,
                        full_type = type.FullName,
                        assembly = type.Assembly.GetName().Name,
                        fields,
                        warning = ex.Message,
                    });
            }

            return new SuccessResponse(
                $"Schema for {type.Name}.",
                new
                {
                    type = type.Name,
                    full_type = type.FullName,
                    assembly = type.Assembly.GetName().Name,
                    fields,
                });
        }

        static Type FindComponentType(string name)
        {
            // 1) Exact FullName match across all loaded assemblies.
            foreach (var asm in AppDomain.CurrentDomain.GetAssemblies())
            {
                var t = asm.GetType(name, throwOnError: false, ignoreCase: false);
                if (t != null && typeof(Component).IsAssignableFrom(t))
                    return t;
            }
            foreach (var asm in AppDomain.CurrentDomain.GetAssemblies())
            {
                var t = asm.GetType(name, throwOnError: false, ignoreCase: true);
                if (t != null && typeof(Component).IsAssignableFrom(t))
                    return t;
            }

            // 2) Short-name match. Prefer UnityEngine.* when multiple assemblies
            //    ship a Component with the same simple name (e.g. custom
            //    Transform shadowing the built-in would be surprising).
            Type best = null;
            foreach (var asm in AppDomain.CurrentDomain.GetAssemblies())
            {
                Type[] types;
                try { types = asm.GetTypes(); }
                catch (System.Reflection.ReflectionTypeLoadException) { continue; }

                foreach (var t in types)
                {
                    if (!typeof(Component).IsAssignableFrom(t)) continue;
                    if (!string.Equals(t.Name, name, StringComparison.OrdinalIgnoreCase)) continue;

                    var isUnity = t.Namespace != null && t.Namespace.StartsWith("UnityEngine", StringComparison.Ordinal);
                    if (best == null)
                    {
                        best = t;
                    }
                    else if (isUnity && !(best.Namespace ?? "").StartsWith("UnityEngine", StringComparison.Ordinal))
                    {
                        best = t;
                    }

                    if (isUnity) return t;
                }
            }
            return best;
        }

        // --- Mutations -----------------------------------------------------

        static object Add(ToolParams p)
        {
            if (EditorApplication.isPlayingOrWillChangePlaymode)
                return new ErrorResponse("Cannot add components while in play mode.");

            var idResult = p.GetRequired("id", "'id' parameter is required for add.");
            if (!idResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, idResult.ErrorMessage);

            var typeResult = p.GetRequired("type", "'type' parameter is required for add.");
            if (!typeResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, typeResult.ErrorMessage);

            if (!StableIdRegistry.TryResolve(idResult.Value, out var go))
                return new ErrorResponse(ErrorCodes.GameObjectNotFound,
                    $"GameObject not found: {idResult.Value}.");

            var type = FindComponentType(typeResult.Value);
            if (type == null)
                return new ErrorResponse(ErrorCodes.ComponentNotFound,
                    $"Type '{typeResult.Value}' not found in loaded assemblies, or not a Component subclass.");

            // Transform is added automatically on every GameObject and cannot
            // be re-added. Catch this up front with a clearer message than
            // AddComponent would give.
            if (type == typeof(Transform))
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    "Every GameObject already has a Transform; cannot add another.");

            var dryRun = p.GetBool("dry_run");
            if (dryRun)
            {
                var existing = go.GetComponents(type).Length;
                return new SuccessResponse(
                    $"[dry-run] Would add {type.Name} to '{go.name}' (existing of same type: {existing}).",
                    new
                    {
                        dry_run = true,
                        go_id = idResult.Value,
                        type = type.Name,
                        existing_of_same_type = existing,
                    });
            }

            Undo.IncrementCurrentGroup();
            Undo.SetCurrentGroupName($"udit component add {type.Name}");

            Component added;
            try
            {
                added = Undo.AddComponent(go, type);
            }
            catch (Exception ex)
            {
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"AddComponent({type.Name}) failed: {ex.Message}");
            }

            // AddComponent returns null when Unity refuses
            // (DisallowMultipleComponent conflict, RequireComponent unsatisfied
            // on non-owning GO, etc.). Surface that rather than silently
            // returning a successful response with no-op side effects.
            if (added == null)
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"AddComponent({type.Name}) returned null — likely DisallowMultipleComponent or other Unity restriction.");

            MarkActiveSceneDirty();
            return new SuccessResponse(
                $"Added {type.Name} to '{go.name}'.",
                new
                {
                    go_id = idResult.Value,
                    type = added.GetType().Name,
                    full_type = added.GetType().FullName,
                    total_components = go.GetComponents<Component>().Length,
                });
        }

        static object Remove(ToolParams p)
        {
            if (EditorApplication.isPlayingOrWillChangePlaymode)
                return new ErrorResponse("Cannot remove components while in play mode.");

            var idResult = p.GetRequired("id", "'id' parameter is required for remove.");
            if (!idResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, idResult.ErrorMessage);

            var typeResult = p.GetRequired("type", "'type' parameter is required for remove.");
            if (!typeResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, typeResult.ErrorMessage);

            if (!StableIdRegistry.TryResolve(idResult.Value, out var go))
                return new ErrorResponse(ErrorCodes.GameObjectNotFound,
                    $"GameObject not found: {idResult.Value}.");

            var index = Math.Max(0, p.GetInt("index", 0) ?? 0);
            var match = FindComponentOnGo(go, typeResult.Value, index, out var matchCount);
            if (match.error != null) return match.error;
            var component = match.component;

            // Transform removal would orphan the GameObject's transform state
            // and Unity would throw. Reject cleanly.
            if (component is Transform)
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    "Transform cannot be removed. Destroy the GameObject instead with `udit go destroy`.");

            var dryRun = p.GetBool("dry_run");
            if (dryRun)
            {
                return new SuccessResponse(
                    $"[dry-run] Would remove {component.GetType().Name} from '{go.name}' (index {index} of {matchCount}).",
                    new
                    {
                        dry_run = true,
                        go_id = idResult.Value,
                        type = component.GetType().Name,
                        index,
                        match_count = matchCount,
                    });
            }

            Undo.IncrementCurrentGroup();
            Undo.SetCurrentGroupName($"udit component remove {component.GetType().Name}");
            Undo.DestroyObjectImmediate(component);
            MarkActiveSceneDirty();

            return new SuccessResponse(
                $"Removed {typeResult.Value} from '{go.name}'.",
                new
                {
                    go_id = idResult.Value,
                    type = typeResult.Value,
                    index,
                    total_components = go.GetComponents<Component>().Length,
                });
        }

        static object Set(ToolParams p)
        {
            if (EditorApplication.isPlayingOrWillChangePlaymode)
                return new ErrorResponse("Cannot set component values while in play mode.");

            var idResult = p.GetRequired("id", "'id' parameter is required for set.");
            if (!idResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, idResult.ErrorMessage);

            var typeResult = p.GetRequired("type", "'type' parameter is required for set.");
            if (!typeResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, typeResult.ErrorMessage);

            var fieldResult = p.GetRequired("field", "'field' parameter is required for set.");
            if (!fieldResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, fieldResult.ErrorMessage);

            // Value may legitimately be the empty string (e.g. clearing a
            // name), so GetRaw is used rather than GetRequired which rejects
            // empty. A genuinely missing value is an error.
            if (p.GetRaw("value") == null)
                return new ErrorResponse(ErrorCodes.InvalidParams, "'value' parameter is required for set.");
            var valueStr = p.Get("value") ?? string.Empty;

            if (!StableIdRegistry.TryResolve(idResult.Value, out var go))
                return new ErrorResponse(ErrorCodes.GameObjectNotFound,
                    $"GameObject not found: {idResult.Value}.");

            var index = Math.Max(0, p.GetInt("index", 0) ?? 0);
            var match = FindComponentOnGo(go, typeResult.Value, index, out _);
            if (match.error != null) return match.error;
            var component = match.component;

            var field = fieldResult.Value;
            var dryRun = p.GetBool("dry_run");

            // Transform's virtual fields (position/local_position/etc.) are
            // the ones `component get` exposes but SerializedObject doesn't,
            // so the same names must work here by routing through Transform
            // API directly. Covers the common "move this GO" case without
            // forcing agents to know the m_LocalPosition quirk.
            if (component is Transform t && IsTransformVirtualField(field))
                return SetTransformVirtualField(t, field, valueStr, dryRun, idResult.Value);

            using var so = new SerializedObject(component);
            var prop = so.FindProperty(field);
            if (prop == null)
            {
                var available = CollectTopLevelFieldNames(so);
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"Field '{field}' not found on {component.GetType().Name}. Top-level fields: {available}.");
            }

            if (!TryParseValueForProperty(prop, valueStr, out var oldValue, out var parseError))
                return new ErrorResponse(ErrorCodes.InvalidParams, parseError);

            if (dryRun)
            {
                return new SuccessResponse(
                    $"[dry-run] Would set {component.GetType().Name}.{field} on '{go.name}'.",
                    new
                    {
                        dry_run = true,
                        go_id = idResult.Value,
                        type = component.GetType().Name,
                        field,
                        from = oldValue,
                        to_value_string = valueStr,
                    });
            }

            Undo.IncrementCurrentGroup();
            Undo.SetCurrentGroupName($"udit component set {component.GetType().Name}.{field}");
            Undo.RecordObject(component, "udit component set");
            ApplyParsedValue(prop, valueStr);
            so.ApplyModifiedProperties();
            MarkActiveSceneDirty();

            return new SuccessResponse(
                $"Set {component.GetType().Name}.{field} on '{go.name}'.",
                new
                {
                    go_id = idResult.Value,
                    type = component.GetType().Name,
                    field,
                    from = oldValue,
                    // Re-read to surface the applied value (clamping etc.).
                    to = ReadPropertyCurrentValue(component, field),
                });
        }

        static object Copy(ToolParams p)
        {
            if (EditorApplication.isPlayingOrWillChangePlaymode)
                return new ErrorResponse("Cannot copy components while in play mode.");

            var srcIdResult = p.GetRequired("id", "'id' parameter (source) is required for copy.");
            if (!srcIdResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, srcIdResult.ErrorMessage);

            var typeResult = p.GetRequired("type", "'type' parameter is required for copy.");
            if (!typeResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, typeResult.ErrorMessage);

            var dstIdResult = p.GetRequired("dst_id", "'dst_id' parameter is required for copy.");
            if (!dstIdResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, dstIdResult.ErrorMessage);

            if (!StableIdRegistry.TryResolve(srcIdResult.Value, out var srcGo))
                return new ErrorResponse(ErrorCodes.GameObjectNotFound,
                    $"Source GameObject not found: {srcIdResult.Value}.");

            if (!StableIdRegistry.TryResolve(dstIdResult.Value, out var dstGo))
                return new ErrorResponse(ErrorCodes.GameObjectNotFound,
                    $"Destination GameObject not found: {dstIdResult.Value}.");

            if (srcGo == dstGo)
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    "Source and destination are the same GameObject; copy would be a no-op.");

            var index = Math.Max(0, p.GetInt("index", 0) ?? 0);
            var match = FindComponentOnGo(srcGo, typeResult.Value, index, out _);
            if (match.error != null) return match.error;
            var src = match.component;

            if (src is Transform)
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    "Transform cannot be copied between GameObjects; use `udit go move` or `component set` for position/rotation/scale.");

            var srcType = src.GetType();
            var dryRun = p.GetBool("dry_run");

            // If the destination already has this component type we overwrite
            // via EditorUtility.CopySerialized; if not, AddComponent first.
            // The observable end state (single matching component with copied
            // values) is the same either way.
            var dstExisting = dstGo.GetComponent(srcType);
            var willAdd = dstExisting == null;

            if (dryRun)
            {
                return new SuccessResponse(
                    $"[dry-run] Would copy {srcType.Name} from '{srcGo.name}' to '{dstGo.name}' ({(willAdd ? "add" : "overwrite existing")}).",
                    new
                    {
                        dry_run = true,
                        src_id = srcIdResult.Value,
                        dst_id = dstIdResult.Value,
                        type = srcType.Name,
                        will_add_on_destination = willAdd,
                    });
            }

            Undo.IncrementCurrentGroup();
            Undo.SetCurrentGroupName($"udit component copy {srcType.Name}");

            Component dst;
            if (dstExisting == null)
            {
                try
                {
                    dst = Undo.AddComponent(dstGo, srcType);
                }
                catch (Exception ex)
                {
                    return new ErrorResponse(ErrorCodes.InvalidParams,
                        $"AddComponent({srcType.Name}) on destination failed: {ex.Message}");
                }
                if (dst == null)
                    return new ErrorResponse(ErrorCodes.InvalidParams,
                        $"AddComponent({srcType.Name}) on destination returned null.");
            }
            else
            {
                dst = dstExisting;
                Undo.RecordObject(dst, "udit component copy (overwrite)");
            }

            // Unity 6+ made EditorUtility.CopySerialized return void (it
            // used to return bool). Wrap in try/catch so a failure still
            // surfaces as a structured error instead of an HTTP 500.
            try
            {
                EditorUtility.CopySerialized(src, dst);
            }
            catch (Exception ex)
            {
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"EditorUtility.CopySerialized({srcType.Name}) failed: {ex.Message}");
            }
            MarkActiveSceneDirty();

            return new SuccessResponse(
                $"Copied {srcType.Name} from '{srcGo.name}' to '{dstGo.name}'.",
                new
                {
                    src_id = srcIdResult.Value,
                    dst_id = dstIdResult.Value,
                    type = srcType.Name,
                    added_on_destination = willAdd,
                });
        }

        // --- Mutation helpers ---------------------------------------------

        static (Component component, ErrorResponse error) FindComponentOnGo(GameObject go, string typeName, int index, out int matchCount)
        {
            var matches = new List<Component>();
            foreach (var c in go.GetComponents<Component>())
            {
                if (c == null) continue;
                var t = c.GetType();
                if (string.Equals(t.Name, typeName, StringComparison.OrdinalIgnoreCase) ||
                    string.Equals(t.FullName, typeName, StringComparison.OrdinalIgnoreCase))
                    matches.Add(c);
            }

            matchCount = matches.Count;
            if (matches.Count == 0)
            {
                var attached = string.Join(", ", go.GetComponents<Component>()
                    .Where(c => c != null)
                    .Select(c => c.GetType().Name));
                return (null, new ErrorResponse(ErrorCodes.ComponentNotFound,
                    $"Component type '{typeName}' not found on {StableIdRegistry.ToStableId(go)}. Attached: {attached}."));
            }

            if (index >= matches.Count)
                return (null, new ErrorResponse(ErrorCodes.ComponentNotFound,
                    $"Component index {index} out of range for type '{typeName}' on {StableIdRegistry.ToStableId(go)} (only {matches.Count} attached)."));

            return (matches[index], null);
        }

        static bool IsTransformVirtualField(string name)
        {
            switch (name)
            {
                case "position":
                case "local_position":
                case "rotation_euler":
                case "local_rotation_euler":
                case "local_scale":
                    return true;
                default:
                    return false;
            }
        }

        static object SetTransformVirtualField(Transform t, string field, string value, bool dryRun, string goId)
        {
            if (!TryParseVector3(value, out var v))
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"Transform.{field} expects 'x,y,z' floats, got '{value}'.");

            object oldValue;
            switch (field)
            {
                case "position":             oldValue = new { x = t.position.x,         y = t.position.y,         z = t.position.z }; break;
                case "local_position":       oldValue = new { x = t.localPosition.x,    y = t.localPosition.y,    z = t.localPosition.z }; break;
                case "rotation_euler":       oldValue = new { x = t.eulerAngles.x,      y = t.eulerAngles.y,      z = t.eulerAngles.z }; break;
                case "local_rotation_euler": oldValue = new { x = t.localEulerAngles.x, y = t.localEulerAngles.y, z = t.localEulerAngles.z }; break;
                case "local_scale":          oldValue = new { x = t.localScale.x,       y = t.localScale.y,       z = t.localScale.z }; break;
                default:
                    return new ErrorResponse(ErrorCodes.InvalidParams, $"Unsupported Transform virtual field: {field}");
            }

            if (dryRun)
            {
                return new SuccessResponse(
                    $"[dry-run] Would set Transform.{field} to ({v.x},{v.y},{v.z}).",
                    new
                    {
                        dry_run = true,
                        go_id = goId,
                        type = "Transform",
                        field,
                        from = oldValue,
                        to = new { x = v.x, y = v.y, z = v.z },
                    });
            }

            Undo.IncrementCurrentGroup();
            Undo.SetCurrentGroupName($"udit component set Transform.{field}");
            Undo.RecordObject(t, "udit component set (transform)");

            switch (field)
            {
                case "position":             t.position = v; break;
                case "local_position":       t.localPosition = v; break;
                case "rotation_euler":       t.eulerAngles = v; break;
                case "local_rotation_euler": t.localEulerAngles = v; break;
                case "local_scale":          t.localScale = v; break;
            }
            MarkActiveSceneDirty();

            return new SuccessResponse(
                $"Set Transform.{field}.",
                new
                {
                    go_id = goId,
                    type = "Transform",
                    field,
                    from = oldValue,
                    to = new { x = v.x, y = v.y, z = v.z },
                });
        }

        static bool TryParseValueForProperty(SerializedProperty prop, string value, out object oldJsonValue, out string error)
        {
            error = null;
            oldJsonValue = null;
            switch (prop.propertyType)
            {
                case SerializedPropertyType.Integer:
                case SerializedPropertyType.LayerMask:
                case SerializedPropertyType.ArraySize:
                case SerializedPropertyType.Character:
                    oldJsonValue = prop.intValue;
                    if (!int.TryParse(value, NumberStyles.Integer, CultureInfo.InvariantCulture, out _))
                    { error = $"Field is Integer, got '{value}'."; return false; }
                    return true;

                case SerializedPropertyType.Boolean:
                    oldJsonValue = prop.boolValue;
                    if (!TryParseBool(value, out _))
                    { error = $"Field is Boolean, got '{value}'. Use true/false/1/0."; return false; }
                    return true;

                case SerializedPropertyType.Float:
                    oldJsonValue = prop.floatValue;
                    if (!float.TryParse(value, NumberStyles.Float, CultureInfo.InvariantCulture, out _))
                    { error = $"Field is Float, got '{value}'."; return false; }
                    return true;

                case SerializedPropertyType.String:
                    oldJsonValue = prop.stringValue;
                    return true;

                case SerializedPropertyType.Vector2:
                    oldJsonValue = new { x = prop.vector2Value.x, y = prop.vector2Value.y };
                    if (!TryParseVector2(value, out _))
                    { error = $"Field is Vector2, expected 'x,y', got '{value}'."; return false; }
                    return true;

                case SerializedPropertyType.Vector3:
                    oldJsonValue = new { x = prop.vector3Value.x, y = prop.vector3Value.y, z = prop.vector3Value.z };
                    if (!TryParseVector3(value, out _))
                    { error = $"Field is Vector3, expected 'x,y,z', got '{value}'."; return false; }
                    return true;

                case SerializedPropertyType.Vector4:
                case SerializedPropertyType.Quaternion:
                    oldJsonValue = new { x = prop.vector4Value.x, y = prop.vector4Value.y, z = prop.vector4Value.z, w = prop.vector4Value.w };
                    if (!TryParseVector4(value, out _))
                    { error = $"Field is Vector4/Quaternion, expected 'x,y,z,w', got '{value}'."; return false; }
                    return true;

                case SerializedPropertyType.Color:
                    var c = prop.colorValue;
                    oldJsonValue = new { r = c.r, g = c.g, b = c.b, a = c.a };
                    if (!TryParseColor(value, out _))
                    { error = $"Field is Color, expected 'r,g,b[,a]' or '#RRGGBB[AA]', got '{value}'."; return false; }
                    return true;

                case SerializedPropertyType.Enum:
                    oldJsonValue = new { value = prop.intValue, name = (prop.enumDisplayNames != null && prop.enumValueIndex >= 0 && prop.enumValueIndex < prop.enumDisplayNames.Length) ? prop.enumDisplayNames[prop.enumValueIndex] : null };
                    if (!TryParseEnum(prop, value, out _))
                    { error = $"Field is Enum, value '{value}' is neither a known name nor valid integer. Names: {string.Join(",", prop.enumDisplayNames ?? new string[0])}."; return false; }
                    return true;

                case SerializedPropertyType.ObjectReference:
                    {
                        var existing = prop.objectReferenceValue;
                        oldJsonValue = existing != null
                            ? new
                            {
                                type = existing.GetType().Name,
                                name = existing.name,
                                path = AssetDatabase.GetAssetPath(existing),
                            }
                            : null;
                        // We validate by actually resolving. On the apply path we
                        // call the same resolver again — AssetDatabase caches the
                        // path->guid lookup so the double cost is negligible.
                        if (!TryResolveObjectReference(prop, value, out _, out var resolveError))
                        { error = resolveError; return false; }
                        return true;
                    }

                case SerializedPropertyType.AnimationCurve:
                    {
                        oldJsonValue = SerializedInspect.DescribeAnimationCurve(prop.animationCurveValue);
                        if (!TryParseAnimationCurve(value, out _, out var perr))
                        { error = perr; return false; }
                        return true;
                    }

                case SerializedPropertyType.Gradient:
                    {
                        oldJsonValue = SerializedInspect.DescribeGradient(prop.gradientValue);
                        if (!TryParseGradient(value, out _, out var perr))
                        { error = perr; return false; }
                        return true;
                    }

                default:
                    {
                        var unsupportedNames = string.Join(", ", s_UnsupportedSet);
                        if (s_UnsupportedSet.Contains(prop.propertyType))
                        {
                            error = $"Setting SerializedPropertyType.{prop.propertyType} is not supported yet. " +
                                    $"Pending write support: {unsupportedNames}.";
                        }
                        else
                        {
                            error = $"Setting SerializedPropertyType.{prop.propertyType} is not supported yet. " +
                                    $"Currently writes: Integer, Boolean, Float, String, Vector2/3/4, Quaternion, " +
                                    $"Color, Enum, LayerMask, ObjectReference. " +
                                    $"Pending: {unsupportedNames}.";
                        }
                        return false;
                    }
            }
        }

        static void ApplyParsedValue(SerializedProperty prop, string value)
        {
            switch (prop.propertyType)
            {
                case SerializedPropertyType.Integer:
                case SerializedPropertyType.LayerMask:
                case SerializedPropertyType.ArraySize:
                case SerializedPropertyType.Character:
                    prop.intValue = int.Parse(value, NumberStyles.Integer, CultureInfo.InvariantCulture);
                    break;
                case SerializedPropertyType.Boolean:
                    TryParseBool(value, out var b);
                    prop.boolValue = b;
                    break;
                case SerializedPropertyType.Float:
                    prop.floatValue = float.Parse(value, NumberStyles.Float, CultureInfo.InvariantCulture);
                    break;
                case SerializedPropertyType.String:
                    prop.stringValue = value;
                    break;
                case SerializedPropertyType.Vector2:
                    TryParseVector2(value, out var v2);
                    prop.vector2Value = v2;
                    break;
                case SerializedPropertyType.Vector3:
                    TryParseVector3(value, out var v3);
                    prop.vector3Value = v3;
                    break;
                case SerializedPropertyType.Vector4:
                    TryParseVector4(value, out var v4);
                    prop.vector4Value = v4;
                    break;
                case SerializedPropertyType.Quaternion:
                    TryParseVector4(value, out var q);
                    prop.quaternionValue = new Quaternion(q.x, q.y, q.z, q.w);
                    break;
                case SerializedPropertyType.Color:
                    TryParseColor(value, out var col);
                    prop.colorValue = col;
                    break;
                case SerializedPropertyType.Enum:
                    TryParseEnum(prop, value, out var e);
                    prop.enumValueIndex = e;
                    break;
                case SerializedPropertyType.ObjectReference:
                    // Resolve again here rather than threading the loaded asset
                    // through the parse->apply split. AssetDatabase caches the
                    // path lookup so the cost is trivial, and keeping the two
                    // paths symmetric avoids a "parse said yes but apply
                    // crashed" disconnect if the asset was swapped between
                    // validation and write.
                    if (TryResolveObjectReference(prop, value, out var obj, out _))
                        prop.objectReferenceValue = obj; // null clears the reference
                    break;
                case SerializedPropertyType.AnimationCurve:
                    if (TryParseAnimationCurve(value, out var curve, out _))
                        prop.animationCurveValue = curve;
                    break;
                case SerializedPropertyType.Gradient:
                    if (TryParseGradient(value, out var gradient, out _))
                        prop.gradientValue = gradient;
                    break;
            }
        }

        static object ReadPropertyCurrentValue(Component c, string field)
        {
            using var so = new SerializedObject(c);
            var prop = so.FindProperty(field);
            if (prop == null) return null;
            switch (prop.propertyType)
            {
                case SerializedPropertyType.Integer:
                case SerializedPropertyType.LayerMask:
                case SerializedPropertyType.ArraySize:
                case SerializedPropertyType.Character:
                    return prop.intValue;
                case SerializedPropertyType.Boolean: return prop.boolValue;
                case SerializedPropertyType.Float:   return prop.floatValue;
                case SerializedPropertyType.String:  return prop.stringValue;
                case SerializedPropertyType.Vector2: return new { x = prop.vector2Value.x, y = prop.vector2Value.y };
                case SerializedPropertyType.Vector3: return new { x = prop.vector3Value.x, y = prop.vector3Value.y, z = prop.vector3Value.z };
                case SerializedPropertyType.Vector4:
                case SerializedPropertyType.Quaternion:
                    return new { x = prop.vector4Value.x, y = prop.vector4Value.y, z = prop.vector4Value.z, w = prop.vector4Value.w };
                case SerializedPropertyType.Color:
                    var c2 = prop.colorValue;
                    return new { r = c2.r, g = c2.g, b = c2.b, a = c2.a };
                case SerializedPropertyType.Enum:
                    return new { value = prop.intValue, name = (prop.enumDisplayNames != null && prop.enumValueIndex >= 0 && prop.enumValueIndex < prop.enumDisplayNames.Length) ? prop.enumDisplayNames[prop.enumValueIndex] : null };
                case SerializedPropertyType.ObjectReference:
                    var oref = prop.objectReferenceValue;
                    if (oref == null) return null;
                    return new
                    {
                        type = oref.GetType().Name,
                        name = oref.name,
                        path = AssetDatabase.GetAssetPath(oref),
                    };
                default:
                    return null;
            }
        }

        static string CollectTopLevelFieldNames(SerializedObject so)
        {
            var names = new List<string>();
            var iter = so.GetIterator();
            bool enterChildren = true;
            while (iter.NextVisible(enterChildren))
            {
                enterChildren = false;
                if (iter.name == "m_Script") continue;
                names.Add(iter.name);
            }
            return string.Join(", ", names);
        }

        static bool TryParseBool(string s, out bool b)
        {
            switch ((s ?? string.Empty).Trim().ToLowerInvariant())
            {
                case "true":
                case "1":
                case "yes":
                case "on":  b = true;  return true;
                case "false":
                case "0":
                case "no":
                case "off": b = false; return true;
                default:    b = false; return false;
            }
        }

        static bool TryParseVector2(string s, out Vector2 v)
        {
            v = default;
            if (string.IsNullOrEmpty(s)) return false;
            var parts = s.Split(',');
            if (parts.Length != 2) return false;
            if (!float.TryParse(parts[0].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var x)) return false;
            if (!float.TryParse(parts[1].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var y)) return false;
            v = new Vector2(x, y); return true;
        }

        static bool TryParseVector3(string s, out Vector3 v)
        {
            v = default;
            if (string.IsNullOrEmpty(s)) return false;
            var parts = s.Split(',');
            if (parts.Length != 3) return false;
            if (!float.TryParse(parts[0].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var x)) return false;
            if (!float.TryParse(parts[1].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var y)) return false;
            if (!float.TryParse(parts[2].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var z)) return false;
            v = new Vector3(x, y, z); return true;
        }

        static bool TryParseVector4(string s, out Vector4 v)
        {
            v = default;
            if (string.IsNullOrEmpty(s)) return false;
            var parts = s.Split(',');
            if (parts.Length != 4) return false;
            if (!float.TryParse(parts[0].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var x)) return false;
            if (!float.TryParse(parts[1].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var y)) return false;
            if (!float.TryParse(parts[2].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var z)) return false;
            if (!float.TryParse(parts[3].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var w)) return false;
            v = new Vector4(x, y, z, w); return true;
        }

        static bool TryParseColor(string s, out Color c)
        {
            c = default;
            if (string.IsNullOrEmpty(s)) return false;
            s = s.Trim();

            // Hex form '#RRGGBB' or '#RRGGBBAA'.
            if (s.StartsWith("#", StringComparison.Ordinal))
                return ColorUtility.TryParseHtmlString(s, out c);

            // Comma-separated 'r,g,b' or 'r,g,b,a' — floats in 0..1.
            var parts = s.Split(',');
            if (parts.Length < 3 || parts.Length > 4) return false;
            if (!float.TryParse(parts[0].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var r)) return false;
            if (!float.TryParse(parts[1].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var g)) return false;
            if (!float.TryParse(parts[2].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var b)) return false;
            var a = 1f;
            if (parts.Length == 4 && !float.TryParse(parts[3].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out a)) return false;
            c = new Color(r, g, b, a); return true;
        }

        static bool TryParseEnum(SerializedProperty prop, string value, out int enumValueIndex)
        {
            // Accept either an integer value-index or one of the enum's
            // display names (case-insensitive). We normalize to the
            // value-index because prop.enumValueIndex is the only safe way
            // to assign back (works for both dense and sparse enums).
            if (int.TryParse(value, NumberStyles.Integer, CultureInfo.InvariantCulture, out enumValueIndex))
            {
                return prop.enumDisplayNames != null
                    && enumValueIndex >= 0
                    && enumValueIndex < prop.enumDisplayNames.Length;
            }

            if (prop.enumDisplayNames != null)
            {
                for (int i = 0; i < prop.enumDisplayNames.Length; i++)
                {
                    if (string.Equals(prop.enumDisplayNames[i], value, StringComparison.OrdinalIgnoreCase))
                    {
                        enumValueIndex = i;
                        return true;
                    }
                }
            }
            enumValueIndex = 0;
            return false;
        }

        /// <summary>
        /// Parse the JSON value string into an AnimationCurve ready for
        /// `prop.animationCurveValue` assignment. Shape:
        ///   { "keys": [ { "t":..,"v":..,"inT":..,"outT":.. }, ... ],
        ///     "preWrap": "ClampForever", "postWrap": "ClampForever" }
        /// Defaults: inT/outT=0 (linear tangents); preWrap/postWrap =
        /// ClampForever (Unity's runtime default).
        /// </summary>
        static bool TryParseAnimationCurve(string json, out AnimationCurve curve, out string error)
        {
            curve = null;
            error = null;
            if (string.IsNullOrWhiteSpace(json))
            {
                error = "AnimationCurve: empty value. Expected JSON { \"keys\": [...] }.";
                return false;
            }
            JObject obj;
            try { obj = JObject.Parse(json); }
            catch (System.Exception ex) { error = $"AnimationCurve: invalid JSON — {ex.Message}"; return false; }

            var keysTok = obj["keys"] as JArray;
            var keyCount = keysTok?.Count ?? 0;
            var keys = new Keyframe[keyCount];
            for (int i = 0; i < keyCount; i++)
            {
                var k = keysTok[i];
                if (k is not JObject)
                {
                    error = $"AnimationCurve: keys[{i}] must be a JSON object like {{\"t\":0,\"v\":0}}.";
                    return false;
                }
                float ParseFloat(string field, float defaultValue)
                {
                    var tok = k[field];
                    if (tok == null) return defaultValue;
                    try { return tok.Value<float>(); }
                    catch { return defaultValue; }
                }
                keys[i] = new Keyframe(
                    ParseFloat("t", 0f),
                    ParseFloat("v", 0f),
                    ParseFloat("inT", 0f),
                    ParseFloat("outT", 0f));
            }
            curve = new AnimationCurve(keys);

            if (obj["preWrap"] != null)
            {
                if (!TryParseWrapMode(obj["preWrap"].ToString(), out var pm, out error)) return false;
                curve.preWrapMode = pm;
            }
            else curve.preWrapMode = WrapMode.ClampForever;

            if (obj["postWrap"] != null)
            {
                if (!TryParseWrapMode(obj["postWrap"].ToString(), out var qm, out error)) return false;
                curve.postWrapMode = qm;
            }
            else curve.postWrapMode = WrapMode.ClampForever;

            return true;
        }

        static bool TryParseWrapMode(string s, out WrapMode mode, out string error)
        {
            mode = WrapMode.ClampForever;
            error = null;
            if (System.Enum.TryParse<WrapMode>(s, true, out var parsed) && System.Enum.IsDefined(typeof(WrapMode), parsed))
            {
                mode = parsed;
                return true;
            }
            error = $"Unknown WrapMode '{s}'. Accepted: Default, Once, Loop, PingPong, ClampForever.";
            return false;
        }

        /// <summary>
        /// Parse a Gradient JSON value. Shape:
        ///   { "colorKeys":[{"t":0,"color":"#RRGGBB[AA]"}],
        ///     "alphaKeys":[{"t":0,"a":1}],
        ///     "mode":"Blend" }
        /// Unity requires 2–8 keys per array; violations produce a clear error.
        /// </summary>
        static bool TryParseGradient(string json, out Gradient gradient, out string error)
        {
            gradient = null;
            error = null;
            if (string.IsNullOrWhiteSpace(json))
            {
                error = "Gradient: empty value. Expected JSON { \"colorKeys\": [...], \"alphaKeys\": [...] }.";
                return false;
            }
            JObject obj;
            try { obj = JObject.Parse(json); }
            catch (System.Exception ex) { error = $"Gradient: invalid JSON — {ex.Message}"; return false; }

            var ckTok = obj["colorKeys"] as JArray;
            var akTok = obj["alphaKeys"] as JArray;
            if (ckTok == null || akTok == null)
            {
                error = "Gradient: both colorKeys and alphaKeys arrays are required.";
                return false;
            }
            if (ckTok.Count < 2 || ckTok.Count > 8)
            {
                error = $"Gradient: colorKeys count must be 2..8, got {ckTok.Count}.";
                return false;
            }
            if (akTok.Count < 2 || akTok.Count > 8)
            {
                error = $"Gradient: alphaKeys count must be 2..8, got {akTok.Count}.";
                return false;
            }

            var colorKeys = new GradientColorKey[ckTok.Count];
            for (int i = 0; i < ckTok.Count; i++)
            {
                var k = ckTok[i];
                if (k is not JObject)
                {
                    error = $"Gradient: colorKeys[{i}] must be a JSON object like {{\"t\":0,\"color\":\"#000000\"}}.";
                    return false;
                }
                float t = k["t"] != null ? k["t"].Value<float>() : 0f;
                var colorRaw = k["color"]?.ToString() ?? "#FFFFFF";
                if (!ColorUtility.TryParseHtmlString(colorRaw, out var col))
                {
                    error = $"Gradient: colorKeys[{i}].color '{colorRaw}' is not a valid hex or named color.";
                    return false;
                }
                colorKeys[i] = new GradientColorKey(col, t);
            }

            var alphaKeys = new GradientAlphaKey[akTok.Count];
            for (int i = 0; i < akTok.Count; i++)
            {
                var k = akTok[i];
                if (k is not JObject)
                {
                    error = $"Gradient: alphaKeys[{i}] must be a JSON object like {{\"t\":0,\"a\":1}}.";
                    return false;
                }
                float t = k["t"] != null ? k["t"].Value<float>() : 0f;
                float a = k["a"] != null ? k["a"].Value<float>() : 1f;
                alphaKeys[i] = new GradientAlphaKey(a, t);
            }

            gradient = new Gradient();
            gradient.SetKeys(colorKeys, alphaKeys);

            if (obj["mode"] != null)
            {
                if (!TryParseGradientMode(obj["mode"].ToString(), out var gm, out error)) return false;
                gradient.mode = gm;
            }
            // colorSpace deferred — Unity's default (Gamma) is correct for most cases.

            return true;
        }

        static bool TryParseGradientMode(string s, out GradientMode mode, out string error)
        {
            mode = GradientMode.Blend;
            error = null;
            if (System.Enum.TryParse<GradientMode>(s, true, out var parsed) && System.Enum.IsDefined(typeof(GradientMode), parsed))
            {
                mode = parsed;
                return true;
            }
            error = $"Unknown GradientMode '{s}'. Accepted: Blend, Fixed, PerceptualBlend.";
            return false;
        }

        static bool TryResolveObjectReference(SerializedProperty prop, string value, out UnityEngine.Object obj, out string error)
        {
            obj = null;
            error = null;

            // Clearing the reference: treat "null", "none", and "" as clear.
            // We accept three spellings because agents occasionally pick the
            // one that matches their vocabulary (JSON null, Unity "None").
            if (string.IsNullOrEmpty(value) ||
                string.Equals(value, "null", StringComparison.OrdinalIgnoreCase) ||
                string.Equals(value, "none", StringComparison.OrdinalIgnoreCase))
            {
                return true;
            }

            // Scene-object references (go:XXXXXXXX). Historically rejected
            // (v0.4.1 conservative cut) on the claim that scene refs need a
            // distinct SceneObjectReference payload. Modern Unity accepts
            // `prop.objectReferenceValue = sceneGameObject` directly when
            // the host and target share a scene — supported in v0.9.0
            // with cross-scene + persistent-host guards matching Inspector
            // default behavior (EditorSceneManager.preventCrossSceneReferences).
            if (value.StartsWith("go:", StringComparison.Ordinal))
            {
                if (!StableIdRegistry.TryResolve(value, out var sceneGo))
                {
                    error = $"GameObject not found for stable ID {value}. Run `udit go find` / `udit scene tree` to get a current id.";
                    return false;
                }

                var host = prop.serializedObject.targetObject;

                // Prefab assets, ScriptableObject assets, and in-memory
                // prefab-edit-mode hosts cannot legally hold a scene
                // reference — Unity will strip the write on reload. Reject
                // loudly rather than writing dead data.
                if (EditorUtility.IsPersistent(host))
                {
                    error = $"Cannot assign scene GameObject {value} to a persistent (prefab / asset) host. " +
                            $"Scene refs only make sense on scene-resident hosts.";
                    return false;
                }

                // Expected type extraction. `prop.type` returns the string
                // form used throughout udit already — plan's
                // `objectReferenceTypeString` suggestion turned out to not
                // be a real property on SerializedProperty; stick with the
                // working `prop.type` that the asset path above also uses.
                var sceneExpectedTypeName = StripPPtrWrapper(prop.type);
                var sceneExpectedType = ResolveUnityObjectType(sceneExpectedTypeName);

                if (sceneExpectedType == typeof(GameObject) || sceneExpectedType == null)
                {
                    // GameObject field — same-scene check (host-as-Component
                    // gives us its scene; ScriptableObject hosts already
                    // rejected above via IsPersistent).
                    if (host is Component hostComp && hostComp.gameObject.scene != sceneGo.scene)
                    {
                        error = $"Cross-scene reference rejected: host scene '{hostComp.gameObject.scene.name}' vs target scene '{sceneGo.scene.name}'.";
                        return false;
                    }
                    obj = sceneGo;
                    return true;
                }

                if (typeof(Component).IsAssignableFrom(sceneExpectedType))
                {
                    // Component field — auto-extract the component. First wins,
                    // matching the sub-asset auto-pick above (LoadAllAssetsAtPath
                    // + first-assignable). Multi-component GOs (e.g. two Cameras)
                    // resolve to GetComponent's first return.
                    if (host is Component hostComp2 && hostComp2.gameObject.scene != sceneGo.scene)
                    {
                        error = $"Cross-scene reference rejected: host scene '{hostComp2.gameObject.scene.name}' vs target scene '{sceneGo.scene.name}'.";
                        return false;
                    }
                    var comp = sceneGo.GetComponent(sceneExpectedType);
                    if (comp == null)
                    {
                        var available = string.Join(", ", sceneGo.GetComponents<Component>().Select(c => c == null ? "<missing>" : c.GetType().Name));
                        error = $"GameObject {value} has no {sceneExpectedTypeName} component. Available: {available}.";
                        return false;
                    }
                    obj = comp;
                    return true;
                }

                error = $"Field expects {sceneExpectedTypeName} but scene refs can only assign GameObject or Component-derived types.";
                return false;
            }

            // Otherwise expect an asset path.
            if (!(value.StartsWith("Assets/", StringComparison.Ordinal) ||
                  value.StartsWith("Packages/", StringComparison.Ordinal)))
            {
                error = $"ObjectReference value must be an asset path (Assets/... or Packages/...), " +
                        $"or 'null'/'none'/'' to clear. Got '{value}'.";
                return false;
            }

            if (string.IsNullOrEmpty(AssetDatabase.AssetPathToGUID(value)))
            {
                // Reuse the registry's asset-not-found signal. We raise it
                // inline rather than letting the caller emit UCI-011 because
                // "asset at path X does not exist" and "asset exists but
                // wrong type" are genuinely different problems for an agent.
                error = $"Asset not found: {value}";
                return false;
            }

            // Resolve the expected type from the field. SerializedProperty.type
            // comes back as "PPtr<$Sprite>" for ObjectReference fields; strip
            // the wrapper to get the bare type name.
            var expectedTypeName = StripPPtrWrapper(prop.type);
            var expectedType = ResolveUnityObjectType(expectedTypeName);

            // LoadAllAssetsAtPath returns [main, sub...]. For a texture with
            // a Sprite sub-asset that is exactly what agents expect — they
            // pass the .png path and the Sprite gets assigned. We pick the
            // FIRST asset assignable to the expected type so "plain" asset
            // types (ScriptableObject, AudioClip, etc.) still work via their
            // main asset.
            var candidates = AssetDatabase.LoadAllAssetsAtPath(value);
            foreach (var candidate in candidates)
            {
                if (candidate == null) continue;
                if (expectedType == null || expectedType.IsAssignableFrom(candidate.GetType()))
                {
                    obj = candidate;
                    return true;
                }
            }

            var found = new List<string>();
            foreach (var c in candidates)
                if (c != null) found.Add(c.GetType().Name);

            error = found.Count == 0
                ? $"Asset at '{value}' contains no assignable objects."
                : $"Asset at '{value}' has no {expectedTypeName} (found: {string.Join(", ", found)}).";
            return false;
        }

        static string StripPPtrWrapper(string s)
        {
            // Unity's objectReferenceTypeString comes back in the form
            // "PPtr<$Sprite>" for most Editor surfaces. Some older paths use
            // "PPtr<Sprite>" without the $. Handle both.
            if (string.IsNullOrEmpty(s)) return s;
            const string prefix = "PPtr<";
            const string suffix = ">";
            if (!s.StartsWith(prefix, StringComparison.Ordinal) || !s.EndsWith(suffix, StringComparison.Ordinal))
                return s;
            var inner = s.Substring(prefix.Length, s.Length - prefix.Length - suffix.Length);
            return inner.StartsWith("$", StringComparison.Ordinal) ? inner.Substring(1) : inner;
        }

        static Type ResolveUnityObjectType(string shortName)
        {
            if (string.IsNullOrEmpty(shortName)) return null;

            // Try UnityEngine.* first (by far the most common — Sprite,
            // Texture, AudioClip, etc.). Fall back to any assembly so
            // project-local ScriptableObject types resolve too.
            foreach (var asm in AppDomain.CurrentDomain.GetAssemblies())
            {
                var t = asm.GetType("UnityEngine." + shortName, throwOnError: false);
                if (t != null && typeof(UnityEngine.Object).IsAssignableFrom(t))
                    return t;
            }
            foreach (var asm in AppDomain.CurrentDomain.GetAssemblies())
            {
                Type[] types;
                try { types = asm.GetTypes(); }
                catch (System.Reflection.ReflectionTypeLoadException) { continue; }
                foreach (var t in types)
                {
                    if (!typeof(UnityEngine.Object).IsAssignableFrom(t)) continue;
                    if (t.Name == shortName || t.FullName == shortName) return t;
                }
            }
            return null;
        }

        static void MarkActiveSceneDirty()
        {
            var scene = EditorSceneManager.GetActiveScene();
            if (scene.IsValid() && scene.isLoaded)
                EditorSceneManager.MarkSceneDirty(scene);
        }
    }
}
