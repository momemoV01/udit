using System.Collections.Generic;
using Newtonsoft.Json.Linq;
using UnityEditor;
using UnityEditor.SceneManagement;
using UnityEngine.SceneManagement;

namespace UditConnector.Tools
{
    [UditTool(Description = "Manage scenes. Actions: list, active, open, save, reload.")]
    public static class ManageScene
    {
        public class Parameters
        {
            [ToolParameter("Action to perform: list, active, open, save, reload", Required = true)]
            public string Action { get; set; }

            [ToolParameter("Scene asset path relative to project root (required for open)")]
            public string Path { get; set; }

            [ToolParameter("Discard unsaved changes when opening or reloading")]
            public bool Force { get; set; }
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
                default:
                    return new ErrorResponse(ErrorCodes.InvalidParams,
                        $"Unknown action '{action}'. Available: list, active, open, save, reload.");
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
    }
}
