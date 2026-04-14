using System;
using System.Collections.Generic;
using System.Text.RegularExpressions;
using Newtonsoft.Json.Linq;
using UditConnector.Tools.Common;
using UnityEditor;
using UnityEngine;

namespace UditConnector.Tools
{
    [UditTool(Description = "Query GameObjects. Actions: find, inspect, path.")]
    public static class ManageGameObject
    {
        const int DefaultLimit = 100;
        const int MaxLimit = 1000;

        public class Parameters
        {
            [ToolParameter("Action to perform: find, inspect, path", Required = true)]
            public string Action { get; set; }

            [ToolParameter("Stable ID (go:XXXXXXXX) — required for inspect, path")]
            public string Id { get; set; }

            [ToolParameter("Name glob pattern for find (e.g. Enemy*). Case-insensitive.")]
            public string Name { get; set; }

            [ToolParameter("Tag filter for find (exact match)")]
            public string Tag { get; set; }

            [ToolParameter("Component type name filter for find (case-insensitive)")]
            public string Component { get; set; }

            [ToolParameter("Include inactive GameObjects in find (default true)")]
            public bool IncludeInactive { get; set; }

            [ToolParameter("Max results per page for find (default 100, max 1000)")]
            public int Limit { get; set; }

            [ToolParameter("Skip first N matches for find (default 0)")]
            public int Offset { get; set; }
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
                case "find":    return Find(p);
                case "inspect": return Inspect(p);
                case "path":    return Path(p);
                default:
                    return new ErrorResponse(ErrorCodes.InvalidParams,
                        $"Unknown action '{action}'. Available: find, inspect, path.");
            }
        }

        static object Find(ToolParams p)
        {
            var name = p.Get("name");
            var tag = p.Get("tag");
            var component = p.Get("component");
            var includeInactive = p.GetBool("include_inactive", true);
            var limit = Clamp(p.GetInt("limit", DefaultLimit) ?? DefaultLimit, 1, MaxLimit);
            var offset = Math.Max(0, p.GetInt("offset", 0) ?? 0);

            Regex nameRegex = null;
            if (!string.IsNullOrEmpty(name))
            {
                // Glob → regex: escape everything, then un-escape `*` to `.*`.
                // Keeps the surface friendly for agents (no regex metacharacters
                // to worry about in GameObject names).
                var pattern = "^" + Regex.Escape(name).Replace("\\*", ".*") + "$";
                nameRegex = new Regex(pattern, RegexOptions.IgnoreCase);
            }

            // FindObjectsByType walks all loaded scenes in one shot and is the
            // non-deprecated replacement for FindObjectsOfType. SortMode.None
            // matters — we sort by instance path ourselves for determinism.
            var transforms = UnityEngine.Object.FindObjectsByType<Transform>(
                includeInactive ? FindObjectsInactive.Include : FindObjectsInactive.Exclude,
                FindObjectsSortMode.None);

            var matches = new List<GameObject>();
            foreach (var t in transforms)
            {
                var go = t.gameObject;
                if (!includeInactive && !go.activeInHierarchy) continue;
                if (nameRegex != null && !nameRegex.IsMatch(go.name)) continue;
                if (!string.IsNullOrEmpty(tag) && !go.CompareTag(tag)) continue;
                if (!string.IsNullOrEmpty(component) && !HasComponent(go, component)) continue;
                matches.Add(go);
            }

            // Deterministic order: hierarchy path ascending. Agents paginate
            // by offset, so the order has to be stable across calls.
            matches.Sort((a, b) => string.Compare(ComputePath(a), ComputePath(b), StringComparison.OrdinalIgnoreCase));

            var total = matches.Count;
            var returned = new List<object>();
            for (int i = offset; i < Math.Min(offset + limit, total); i++)
            {
                var go = matches[i];
                returned.Add(new
                {
                    id = StableIdRegistry.ToStableId(go),
                    name = go.name,
                    active = go.activeInHierarchy,
                    tag = go.tag,
                    layer = LayerMask.LayerToName(go.layer),
                    path = ComputePath(go),
                });
            }

            return new SuccessResponse(
                $"Matched {total} GameObject(s), returning {returned.Count}.",
                new
                {
                    total,
                    offset,
                    limit,
                    returned = returned.Count,
                    has_more = offset + returned.Count < total,
                    matches = returned,
                });
        }

        static object Inspect(ToolParams p)
        {
            var idResult = p.GetRequired("id", "'id' parameter is required for inspect.");
            if (!idResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, idResult.ErrorMessage);

            if (!StableIdRegistry.TryResolve(idResult.Value, out var go))
                return new ErrorResponse(ErrorCodes.GameObjectNotFound,
                    $"GameObject not found: {idResult.Value}. Run `go find` first if the ID is from a previous session.");

            var t = go.transform;
            var parentId = t.parent != null ? StableIdRegistry.ToStableId(t.parent.gameObject) : null;
            var childIds = new List<string>(t.childCount);
            for (int i = 0; i < t.childCount; i++)
                childIds.Add(StableIdRegistry.ToStableId(t.GetChild(i).gameObject));

            var components = go.GetComponents<Component>();
            var compData = new List<object>(components.Length);
            foreach (var c in components)
                compData.Add(SerializedInspect.ComponentToObject(c));

            return new SuccessResponse(
                $"Inspected {go.name}.",
                new
                {
                    id = idResult.Value,
                    name = go.name,
                    active = go.activeInHierarchy,
                    active_self = go.activeSelf,
                    tag = go.tag,
                    layer = LayerMask.LayerToName(go.layer),
                    layer_index = go.layer,
                    scene = go.scene.path,
                    path = ComputePath(go),
                    parent_id = parentId,
                    children_ids = childIds,
                    components = compData,
                });
        }

        static object Path(ToolParams p)
        {
            var idResult = p.GetRequired("id", "'id' parameter is required for path.");
            if (!idResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, idResult.ErrorMessage);

            if (!StableIdRegistry.TryResolve(idResult.Value, out var go))
                return new ErrorResponse(ErrorCodes.GameObjectNotFound,
                    $"GameObject not found: {idResult.Value}.");

            return new SuccessResponse(
                $"Path for {go.name}.",
                new
                {
                    id = idResult.Value,
                    name = go.name,
                    path = ComputePath(go),
                    scene = go.scene.path,
                });
        }

        static bool HasComponent(GameObject go, string typeName)
        {
            foreach (var c in go.GetComponents<Component>())
            {
                if (c == null) continue;
                if (string.Equals(c.GetType().Name, typeName, StringComparison.OrdinalIgnoreCase))
                    return true;
            }
            return false;
        }

        static string ComputePath(GameObject go)
        {
            // Walk up the transform tree and build a slash-joined path. Using
            // a stack rather than recursion keeps stack depth bounded for
            // deep hierarchies (prefab chains 20+ levels deep are common).
            var parts = new Stack<string>();
            var t = go.transform;
            while (t != null)
            {
                parts.Push(t.name);
                t = t.parent;
            }
            return string.Join("/", parts);
        }

        static int Clamp(int v, int lo, int hi) => v < lo ? lo : (v > hi ? hi : v);
    }
}
