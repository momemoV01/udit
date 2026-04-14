using System.Collections.Generic;
using Newtonsoft.Json.Linq;
using UditConnector.Tools.Common;
using UnityEditor;
using UnityEditor.SceneManagement;
using UnityEngine;
using UnityEngine.SceneManagement;

namespace UditConnector.Tools
{
    [UditTool(Description = "Manage scenes. Actions: list, active, open, save, reload, tree.")]
    public static class ManageScene
    {
        public class Parameters
        {
            [ToolParameter("Action to perform: list, active, open, save, reload, tree", Required = true)]
            public string Action { get; set; }

            [ToolParameter("Scene asset path relative to project root (required for open)")]
            public string Path { get; set; }

            [ToolParameter("Discard unsaved changes when opening or reloading")]
            public bool Force { get; set; }

            [ToolParameter("Max hierarchy depth for tree (0 = roots only, -1 or omitted = unlimited)")]
            public int Depth { get; set; }

            [ToolParameter("Include inactive GameObjects in tree (default true)")]
            public bool IncludeInactive { get; set; }
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
                case "list":   return List();
                case "active": return Active();
                case "open":   return Open(p);
                case "save":   return Save();
                case "reload": return Reload(p);
                case "tree":   return Tree(p);
                default:
                    return new ErrorResponse(ErrorCodes.InvalidParams,
                        $"Unknown action '{action}'. Available: list, active, open, save, reload, tree.");
            }
        }

        static object List()
        {
            // Build a fast path → (enabled, index) lookup from build settings so
            // the per-scene annotation below is O(1) instead of O(buildScenes).
            var buildIndex = new Dictionary<string, (bool enabled, int index)>();
            var buildScenes = EditorBuildSettings.scenes;
            for (int i = 0; i < buildScenes.Length; i++)
                buildIndex[buildScenes[i].path] = (buildScenes[i].enabled, i);

            var guids = AssetDatabase.FindAssets("t:Scene");
            var entries = new List<(string path, string guid)>(guids.Length);
            foreach (var guid in guids)
            {
                var path = AssetDatabase.GUIDToAssetPath(guid);
                if (!string.IsNullOrEmpty(path))
                    entries.Add((path, guid));
            }

            // Sort on the typed tuple so the comparer infers without `dynamic`
            // (anonymous types cannot participate in OrderBy type inference).
            entries.Sort((a, b) => System.StringComparer.OrdinalIgnoreCase.Compare(a.path, b.path));

            var scenes = new List<object>(entries.Count);
            foreach (var (path, guid) in entries)
            {
                buildIndex.TryGetValue(path, out var b);
                scenes.Add(new
                {
                    path,
                    guid,
                    name = System.IO.Path.GetFileNameWithoutExtension(path),
                    in_build = b.enabled,
                    build_index = b.enabled ? (int?)b.index : null,
                });
            }

            return new SuccessResponse($"Found {scenes.Count} scene(s).", new
            {
                count = scenes.Count,
                scenes,
            });
        }

        static object Active()
        {
            var s = EditorSceneManager.GetActiveScene();
            return new SuccessResponse("Active scene.", DescribeScene(s));
        }

        static object Open(ToolParams p)
        {
            var pathResult = p.GetRequired("path", "'path' parameter is required for open.");
            if (!pathResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, pathResult.ErrorMessage);

            if (EditorApplication.isPlayingOrWillChangePlaymode)
                return new ErrorResponse("Cannot open scenes while in play mode.");

            var path = pathResult.Value;
            if (string.IsNullOrEmpty(AssetDatabase.AssetPathToGUID(path)))
                return new ErrorResponse(ErrorCodes.SceneNotFound, $"Scene not found: {path}");

            var force = p.GetBool("force");
            var current = EditorSceneManager.GetActiveScene();
            if (current.isDirty && !force)
            {
                return new ErrorResponse(
                    "Current scene has unsaved changes. Save first (`udit scene save`) or pass `--force` to discard.",
                    new { current = current.path, is_dirty = true });
            }

            var previousPath = string.IsNullOrEmpty(current.path) ? null : current.path;
            var discarded = current.isDirty;

            var opened = EditorSceneManager.OpenScene(path, OpenSceneMode.Single);

            return new SuccessResponse($"Opened {opened.path}.", new
            {
                opened = opened.path,
                previous = previousPath,
                discarded_changes = discarded,
            });
        }

        static object Save()
        {
            // Snapshot dirty paths BEFORE saving so we can report which ones
            // actually needed the write. SaveOpenScenes clears the dirty flag.
            var dirtyBefore = new List<string>();
            for (int i = 0; i < EditorSceneManager.sceneCount; i++)
            {
                var s = EditorSceneManager.GetSceneAt(i);
                if (s.isDirty) dirtyBefore.Add(string.IsNullOrEmpty(s.path) ? s.name : s.path);
            }

            var ok = EditorSceneManager.SaveOpenScenes();
            if (!ok)
                return new ErrorResponse("SaveOpenScenes returned false — one or more scenes could not be saved.");

            return new SuccessResponse(
                dirtyBefore.Count == 0 ? "No dirty scenes to save." : $"Saved {dirtyBefore.Count} scene(s).",
                new { saved = dirtyBefore, count = dirtyBefore.Count });
        }

        static object Reload(ToolParams p)
        {
            if (EditorApplication.isPlayingOrWillChangePlaymode)
                return new ErrorResponse("Cannot reload scenes while in play mode.");

            var active = EditorSceneManager.GetActiveScene();
            if (string.IsNullOrEmpty(active.path))
                return new ErrorResponse("No saved scene to reload (current scene is untitled).");

            var force = p.GetBool("force");
            if (active.isDirty && !force)
            {
                return new ErrorResponse(
                    "Scene has unsaved changes. Pass `--force` to discard and reload.",
                    new { path = active.path, is_dirty = true });
            }

            var path = active.path;
            var discarded = active.isDirty;
            EditorSceneManager.OpenScene(path, OpenSceneMode.Single);

            return new SuccessResponse($"Reloaded {path}.", new
            {
                reloaded = path,
                discarded_changes = discarded,
            });
        }

        static object DescribeScene(Scene s)
        {
            var hasPath = !string.IsNullOrEmpty(s.path);
            return new
            {
                path = hasPath ? s.path : null,
                guid = hasPath ? AssetDatabase.AssetPathToGUID(s.path) : null,
                name = s.name,
                is_dirty = s.isDirty,
                is_loaded = s.isLoaded,
                root_count = s.IsValid() ? s.rootCount : 0,
                build_index = s.buildIndex,
                is_untitled = !hasPath,
            };
        }

        static object Tree(ToolParams p)
        {
            var scene = EditorSceneManager.GetActiveScene();
            if (!scene.IsValid() || !scene.isLoaded)
                return new ErrorResponse("No active scene is loaded.");

            // depth < 0 means unlimited. 0 means roots only (no children).
            // Omitted param lands here as null → default to unlimited.
            var depth = p.GetInt("depth", -1) ?? -1;
            var includeInactive = p.GetBool("include_inactive", true);

            var roots = scene.GetRootGameObjects();
            var nodes = new List<object>(roots.Length);
            var count = 0;
            foreach (var go in roots)
            {
                var node = BuildTreeNode(go, depth, includeInactive, ref count);
                if (node != null) nodes.Add(node);
            }

            return new SuccessResponse(
                $"Scene tree: {count} GameObject(s).",
                new
                {
                    scene = string.IsNullOrEmpty(scene.path) ? null : scene.path,
                    depth,
                    include_inactive = includeInactive,
                    count,
                    roots = nodes,
                });
        }

        static object BuildTreeNode(GameObject go, int depthRemaining, bool includeInactive, ref int count)
        {
            if (go == null) return null;
            // Skip inactive subtrees entirely when the caller asked. activeInHierarchy
            // already folds in parent state, so filtering at each node naturally
            // hides the whole descendant tree of an inactive root.
            if (!includeInactive && !go.activeInHierarchy) return null;

            count++;

            var components = go.GetComponents<Component>();
            var componentNames = new List<string>(components.Length);
            foreach (var c in components)
            {
                // A null Component slot indicates a missing script reference — surface
                // it explicitly so agents can detect and repair stale prefabs.
                componentNames.Add(c == null ? "<Missing Script>" : c.GetType().Name);
            }

            var children = new List<object>();
            if (depthRemaining != 0)
            {
                var t = go.transform;
                var nextDepth = depthRemaining < 0 ? -1 : depthRemaining - 1;
                for (int i = 0; i < t.childCount; i++)
                {
                    var childNode = BuildTreeNode(t.GetChild(i).gameObject, nextDepth, includeInactive, ref count);
                    if (childNode != null) children.Add(childNode);
                }
            }

            return new
            {
                id = StableIdRegistry.ToStableId(go),
                name = go.name,
                active = go.activeInHierarchy,
                components = componentNames,
                children,
            };
        }
    }
}
