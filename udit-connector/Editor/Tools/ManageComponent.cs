using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;
using UditConnector.Tools.Common;
using UnityEditor;
using UnityEngine;

namespace UditConnector.Tools
{
    [UditTool(Description = "Query component values and schemas. Actions: list, get, schema.")]
    public static class ManageComponent
    {
        public class Parameters
        {
            [ToolParameter("Action to perform: list, get, schema", Required = true)]
            public string Action { get; set; }

            [ToolParameter("Stable ID (go:XXXXXXXX) — required for list, get")]
            public string Id { get; set; }

            [ToolParameter("Component type name — required for get, schema (case-insensitive, unqualified names resolve to UnityEngine.* first)")]
            public string Type { get; set; }

            [ToolParameter("Dotted field path to read from the component (get only). Omit to dump every visible field.")]
            public string Field { get; set; }

            [ToolParameter("Zero-based index when the GameObject has multiple components of the same type (get only, default 0)")]
            public int Index { get; set; }
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
                default:
                    return new ErrorResponse(ErrorCodes.InvalidParams,
                        $"Unknown action '{action}'. Available: list, get, schema.");
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
    }
}
