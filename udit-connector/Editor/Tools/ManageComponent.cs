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
    public static partial class ManageComponent
    {
        // SerializedPropertyType cases that `component set` does not yet
        // implement. Each new write-handler removes its entry; the rejection
        // message in TryParseValueForProperty's default branch is computed
        // from this set so each commit's diff is a single-line removal.
        // ExposedReference stays after v0.9.0 (different concept — prefab
        // variant resolution context — likely never set this way from CLI).
        static readonly System.Collections.Generic.HashSet<SerializedPropertyType> s_UnsupportedSet = new()
        {
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
            if (!ParamCoercion.TryParseVector3(value, out var v))
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

        static void MarkActiveSceneDirty()
        {
            var scene = EditorSceneManager.GetActiveScene();
            if (scene.IsValid() && scene.isLoaded)
                EditorSceneManager.MarkSceneDirty(scene);
        }
    }
}
