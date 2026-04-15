using System;
using System.Collections.Generic;
using System.Text.RegularExpressions;
using Newtonsoft.Json.Linq;
using UditConnector.Tools.Common;
using UnityEditor;
using UnityEditor.SceneManagement;
using UnityEngine;

namespace UditConnector.Tools
{
    [UditTool(Description = "Query and mutate GameObjects. Actions: find, inspect, path, create, destroy, move, rename, setactive.")]
    public static class ManageGameObject
    {
        const int DefaultLimit = 100;
        const int MaxLimit = 1000;

        public class Parameters
        {
            [ToolParameter("Action to perform: find, inspect, path, create, destroy, move, rename, setactive", Required = true)]
            public string Action { get; set; }

            [ToolParameter("Stable ID (go:XXXXXXXX) — required for inspect, path, destroy, move, rename, setactive")]
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

            [ToolParameter("Parent stable ID for create / move (omit for scene root)")]
            public string Parent { get; set; }

            [ToolParameter("Local position for create as 'x,y,z' (default '0,0,0')")]
            public string Pos { get; set; }

            [ToolParameter("New name for rename")]
            public string NewName { get; set; }

            [ToolParameter("Active state for setactive (true/false)")]
            public bool Active { get; set; }

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
                case "find":      return Find(p);
                case "inspect":   return Inspect(p);
                case "path":      return Path(p);
                case "create":    return Create(p);
                case "destroy":   return Destroy(p);
                case "move":      return Move(p);
                case "rename":    return Rename(p);
                case "setactive": return SetActive(p);
                default:
                    return new ErrorResponse(ErrorCodes.InvalidParams,
                        $"Unknown action '{action}'. Available: find, inspect, path, create, destroy, move, rename, setactive.");
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

            // FindObjectsByType walks all loaded scenes in one shot. Unity 6
            // deprecated the SortMode overload; the new single-arg overload
            // returns in unspecified order, which is fine here because we
            // sort by hierarchy path ourselves below for deterministic
            // pagination.
            var transforms = UnityEngine.Object.FindObjectsByType<Transform>(
                includeInactive ? FindObjectsInactive.Include : FindObjectsInactive.Exclude);

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

        // --- Mutations -----------------------------------------------------

        static object Create(ToolParams p)
        {
            if (EditorApplication.isPlayingOrWillChangePlaymode)
                return new ErrorResponse("Cannot create GameObjects while in play mode.");

            var name = p.Get("name", "GameObject");
            if (string.IsNullOrEmpty(name))
                return new ErrorResponse(ErrorCodes.InvalidParams, "'name' must not be empty for create.");

            // Resolve optional parent first so dry-run reports a meaningful target.
            GameObject parent = null;
            var parentId = p.Get("parent");
            if (!string.IsNullOrEmpty(parentId))
            {
                if (!StableIdRegistry.TryResolve(parentId, out parent))
                    return new ErrorResponse(ErrorCodes.GameObjectNotFound,
                        $"Parent GameObject not found: {parentId}.");
            }

            // Parse position. We accept "x,y,z" because that matches the ROADMAP
            // surface and stays inside the existing string-flag plumbing.
            // Errors here point at the offending value rather than failing the
            // whole call with a generic "bad params".
            var posStr = p.Get("pos");
            Vector3 pos = Vector3.zero;
            if (!string.IsNullOrEmpty(posStr))
            {
                if (!ParamCoercion.TryParseVector3(posStr, out pos))
                    return new ErrorResponse(ErrorCodes.InvalidParams,
                        $"--pos must be 'x,y,z' floats, got '{posStr}'.");
            }

            var dryRun = p.GetBool("dry_run");
            var parentDescription = parent != null
                ? $"{parent.name} ({StableIdRegistry.ToStableId(parent)})"
                : "<scene root>";

            if (dryRun)
            {
                return new SuccessResponse(
                    $"[dry-run] Would create '{name}' under {parentDescription}.",
                    new
                    {
                        dry_run = true,
                        would_create = name,
                        parent = parent != null ? StableIdRegistry.ToStableId(parent) : null,
                        local_position = new { x = pos.x, y = pos.y, z = pos.z },
                    });
            }

            // IncrementCurrentGroup forces Unity to start a fresh Undo group
            // for this mutation. Without it, multiple udit commands fired
            // within the same editor tick collapse into one group and a
            // single Undo unwinds them all at once (or, worse, cancels a
            // create+destroy pair into a no-op).
            Undo.IncrementCurrentGroup();
            Undo.SetCurrentGroupName($"udit go create '{name}'");

            var go = new GameObject(name);
            // RegisterCreatedObjectUndo means Ctrl+Z in Unity removes the GO
            // again. Without this, agent-created GOs would silently persist
            // through user undo attempts and surprise everyone.
            Undo.RegisterCreatedObjectUndo(go, $"udit go create '{name}'");

            if (parent != null)
                Undo.SetTransformParent(go.transform, parent.transform, "udit go create (parent)");

            go.transform.localPosition = pos;

            MarkActiveSceneDirty();
            var id = StableIdRegistry.ToStableId(go);
            return new SuccessResponse(
                $"Created '{name}' as {id}.",
                new
                {
                    id,
                    name = go.name,
                    path = ComputePath(go),
                    parent = parent != null ? StableIdRegistry.ToStableId(parent) : null,
                    local_position = new { x = go.transform.localPosition.x, y = go.transform.localPosition.y, z = go.transform.localPosition.z },
                });
        }

        static object Destroy(ToolParams p)
        {
            if (EditorApplication.isPlayingOrWillChangePlaymode)
                return new ErrorResponse("Cannot destroy GameObjects while in play mode.");

            var idResult = p.GetRequired("id", "'id' parameter is required for destroy.");
            if (!idResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, idResult.ErrorMessage);

            if (!StableIdRegistry.TryResolve(idResult.Value, out var go))
                return new ErrorResponse(ErrorCodes.GameObjectNotFound,
                    $"GameObject not found: {idResult.Value}.");

            var dryRun = p.GetBool("dry_run");
            var path = ComputePath(go);
            var childCount = go.transform.childCount;
            var componentNames = new List<string>();
            foreach (var c in go.GetComponents<Component>())
            {
                if (c == null) continue;
                componentNames.Add(c.GetType().Name);
            }

            if (dryRun)
            {
                return new SuccessResponse(
                    $"[dry-run] Would destroy '{go.name}' ({childCount} child(ren) cascade).",
                    new
                    {
                        dry_run = true,
                        would_destroy = path,
                        id = idResult.Value,
                        children_affected = childCount,
                        components = componentNames,
                    });
            }

            Undo.IncrementCurrentGroup();
            Undo.SetCurrentGroupName($"udit go destroy '{go.name}'");
            // Undo.DestroyObjectImmediate keeps the GO restorable via Ctrl+Z,
            // unlike plain Object.DestroyImmediate which is permanent.
            Undo.DestroyObjectImmediate(go);
            MarkActiveSceneDirty();

            return new SuccessResponse(
                $"Destroyed '{path}' ({childCount} child(ren)).",
                new
                {
                    destroyed = path,
                    id = idResult.Value,
                    children_affected = childCount,
                });
        }

        static object Move(ToolParams p)
        {
            if (EditorApplication.isPlayingOrWillChangePlaymode)
                return new ErrorResponse("Cannot move GameObjects while in play mode.");

            var idResult = p.GetRequired("id", "'id' parameter is required for move.");
            if (!idResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, idResult.ErrorMessage);

            if (!StableIdRegistry.TryResolve(idResult.Value, out var go))
                return new ErrorResponse(ErrorCodes.GameObjectNotFound,
                    $"GameObject not found: {idResult.Value}.");

            GameObject newParent = null;
            var parentId = p.Get("parent");
            if (!string.IsNullOrEmpty(parentId))
            {
                if (!StableIdRegistry.TryResolve(parentId, out newParent))
                    return new ErrorResponse(ErrorCodes.GameObjectNotFound,
                        $"Parent GameObject not found: {parentId}.");

                // Catch the obvious cycle: making a GO its own ancestor would
                // crash Unity. Walk the candidate's ancestor chain and reject
                // if `go` shows up.
                if (newParent == go || IsAncestor(go.transform, newParent.transform))
                {
                    return new ErrorResponse(ErrorCodes.InvalidParams,
                        $"Cannot reparent '{go.name}' under itself or a descendant.");
                }
            }

            var dryRun = p.GetBool("dry_run");
            var oldParent = go.transform.parent;
            var oldParentId = oldParent != null ? StableIdRegistry.ToStableId(oldParent.gameObject) : null;
            var newParentId = newParent != null ? StableIdRegistry.ToStableId(newParent) : null;

            if (dryRun)
            {
                return new SuccessResponse(
                    $"[dry-run] Would move '{go.name}' from {oldParentId ?? "<root>"} to {newParentId ?? "<root>"}.",
                    new
                    {
                        dry_run = true,
                        id = idResult.Value,
                        from_parent = oldParentId,
                        to_parent = newParentId,
                    });
            }

            Undo.IncrementCurrentGroup();
            Undo.SetCurrentGroupName($"udit go move '{go.name}'");
            Undo.SetTransformParent(go.transform, newParent != null ? newParent.transform : null, "udit go move");
            MarkActiveSceneDirty();

            return new SuccessResponse(
                $"Moved '{go.name}' to {newParentId ?? "<root>"}.",
                new
                {
                    id = idResult.Value,
                    name = go.name,
                    from_parent = oldParentId,
                    to_parent = newParentId,
                    path = ComputePath(go),
                });
        }

        static object Rename(ToolParams p)
        {
            if (EditorApplication.isPlayingOrWillChangePlaymode)
                return new ErrorResponse("Cannot rename GameObjects while in play mode.");

            var idResult = p.GetRequired("id", "'id' parameter is required for rename.");
            if (!idResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, idResult.ErrorMessage);

            if (!StableIdRegistry.TryResolve(idResult.Value, out var go))
                return new ErrorResponse(ErrorCodes.GameObjectNotFound,
                    $"GameObject not found: {idResult.Value}.");

            var newName = p.Get("new_name");
            if (string.IsNullOrEmpty(newName))
                return new ErrorResponse(ErrorCodes.InvalidParams, "'new_name' must not be empty for rename.");

            var dryRun = p.GetBool("dry_run");
            var oldName = go.name;

            if (dryRun)
            {
                return new SuccessResponse(
                    $"[dry-run] Would rename '{oldName}' -> '{newName}'.",
                    new
                    {
                        dry_run = true,
                        id = idResult.Value,
                        from = oldName,
                        to = newName,
                    });
            }

            Undo.IncrementCurrentGroup();
            Undo.SetCurrentGroupName($"udit go rename '{oldName}' -> '{newName}'");
            Undo.RecordObject(go, "udit go rename");
            go.name = newName;
            MarkActiveSceneDirty();

            return new SuccessResponse(
                $"Renamed '{oldName}' -> '{newName}'.",
                new
                {
                    id = idResult.Value,
                    from = oldName,
                    to = go.name,
                    path = ComputePath(go),
                });
        }

        static object SetActive(ToolParams p)
        {
            if (EditorApplication.isPlayingOrWillChangePlaymode)
                return new ErrorResponse("Cannot toggle GameObjects while in play mode.");

            var idResult = p.GetRequired("id", "'id' parameter is required for setactive.");
            if (!idResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, idResult.ErrorMessage);

            if (!StableIdRegistry.TryResolve(idResult.Value, out var go))
                return new ErrorResponse(ErrorCodes.GameObjectNotFound,
                    $"GameObject not found: {idResult.Value}.");

            // 'active' is a switch flag from the CLI side. We require it to be
            // present (otherwise `setactive` would silently default to false
            // and toggle off objects without intent).
            if (p.GetRaw("active") == null)
                return new ErrorResponse(ErrorCodes.InvalidParams, "'active' parameter is required for setactive (true|false).");

            var newState = p.GetBool("active");
            var oldState = go.activeSelf;
            var dryRun = p.GetBool("dry_run");

            if (oldState == newState)
            {
                return new SuccessResponse(
                    $"'{go.name}' is already active={oldState}; no change.",
                    new
                    {
                        id = idResult.Value,
                        active_self = oldState,
                        no_change = true,
                    });
            }

            if (dryRun)
            {
                return new SuccessResponse(
                    $"[dry-run] Would set '{go.name}' active={newState} (was {oldState}).",
                    new
                    {
                        dry_run = true,
                        id = idResult.Value,
                        from = oldState,
                        to = newState,
                    });
            }

            Undo.IncrementCurrentGroup();
            Undo.SetCurrentGroupName($"udit go setactive '{go.name}' -> {newState}");
            Undo.RecordObject(go, "udit go setactive");
            go.SetActive(newState);
            MarkActiveSceneDirty();

            return new SuccessResponse(
                $"Set '{go.name}' active={newState}.",
                new
                {
                    id = idResult.Value,
                    from = oldState,
                    to = newState,
                });
        }

        // --- Mutation helpers ---------------------------------------------

        static bool IsAncestor(Transform candidateAncestor, Transform of)
        {
            // Walk `of`'s parent chain and report whether candidateAncestor
            // appears anywhere — used to reject cycle-creating reparents.
            for (var t = of; t != null; t = t.parent)
            {
                if (t == candidateAncestor) return true;
            }
            return false;
        }


        static void MarkActiveSceneDirty()
        {
            // After every mutation we bump the active scene's dirty flag so the
            // user sees the unsaved-changes asterisk and gets the standard
            // save prompt on close. Matches what Inspector edits do.
            var scene = EditorSceneManager.GetActiveScene();
            if (scene.IsValid() && scene.isLoaded)
                EditorSceneManager.MarkSceneDirty(scene);
        }
    }
}
