using System;
using System.Collections.Generic;
using Newtonsoft.Json.Linq;
using UditConnector.Tools.Common;
using UnityEditor;
using UnityEditor.SceneManagement;
using UnityEngine;

namespace UditConnector.Tools
{
    [UditTool(Description = "Prefab operations. Actions: instantiate, unpack, apply, find_instances.")]
    public static class ManagePrefab
    {
        public class Parameters
        {
            [ToolParameter("Action to perform: instantiate, unpack, apply, find_instances", Required = true)]
            public string Action { get; set; }

            [ToolParameter("Prefab asset path (instantiate, find_instances)")]
            public string Path { get; set; }

            [ToolParameter("Stable ID (go:XXXXXXXX) for unpack / apply")]
            public string Id { get; set; }

            [ToolParameter("Parent stable ID for instantiate (omit for scene root)")]
            public string Parent { get; set; }

            [ToolParameter("Local position for instantiate as 'x,y,z' (default '0,0,0')")]
            public string Pos { get; set; }

            [ToolParameter("Unpack mode: root (default, OutermostRoot) or completely")]
            public string Mode { get; set; }

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
                case "instantiate":    return Instantiate(p);
                case "unpack":         return Unpack(p);
                case "apply":          return Apply(p);
                case "find_instances": return FindInstances(p);
                default:
                    return new ErrorResponse(ErrorCodes.InvalidParams,
                        $"Unknown action '{action}'. Available: instantiate, unpack, apply, find_instances.");
            }
        }

        static object Instantiate(ToolParams p)
        {
            if (EditorApplication.isPlayingOrWillChangePlaymode)
                return new ErrorResponse("Cannot instantiate prefabs while in play mode.");

            var pathResult = p.GetRequired("path", "'path' parameter is required for instantiate.");
            if (!pathResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, pathResult.ErrorMessage);

            var path = pathResult.Value;
            // Split "no asset at this path" from "asset exists but wrong
            // type" — they are different problems for an agent.
            // AssetPathToGUID returns empty for genuinely-missing paths.
            if (string.IsNullOrEmpty(AssetDatabase.AssetPathToGUID(path)))
                return new ErrorResponse(ErrorCodes.AssetNotFound,
                    $"No asset at path: {path}");

            var asset = AssetDatabase.LoadAssetAtPath<GameObject>(path);
            if (asset == null)
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"Asset at '{path}' is not a GameObject (prefab). " +
                    $"Run `udit asset inspect {path}` to see its actual type.");

            // A GameObject asset that is not a prefab (e.g. a legacy FBX
            // root imported as a GameObject) cannot be instantiated with a
            // prefab connection. Reject cleanly rather than letting
            // InstantiatePrefab error out with an opaque message.
            if (!PrefabUtility.IsPartOfPrefabAsset(asset))
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"Asset at '{path}' is a GameObject but not a prefab. `prefab instantiate` requires a prefab asset.");

            GameObject parent = null;
            var parentId = p.Get("parent");
            if (!string.IsNullOrEmpty(parentId))
            {
                if (!StableIdRegistry.TryResolve(parentId, out parent))
                    return new ErrorResponse(ErrorCodes.GameObjectNotFound,
                        $"Parent GameObject not found: {parentId}.");
            }

            var posStr = p.Get("pos");
            Vector3 pos = Vector3.zero;
            if (!string.IsNullOrEmpty(posStr))
            {
                if (!ParamCoercion.TryParseVector3(posStr, out pos))
                    return new ErrorResponse(ErrorCodes.InvalidParams,
                        $"--pos must be 'x,y,z' floats, got '{posStr}'.");
            }

            var dryRun = p.GetBool("dry_run");
            if (dryRun)
            {
                return new SuccessResponse(
                    $"[dry-run] Would instantiate '{asset.name}' under " +
                    (parent != null ? parent.name : "<scene root>") + $" at {pos}.",
                    new
                    {
                        dry_run = true,
                        would_instantiate = path,
                        name = asset.name,
                        parent = parent != null ? StableIdRegistry.ToStableId(parent) : null,
                        local_position = new { x = pos.x, y = pos.y, z = pos.z },
                    });
            }

            Undo.IncrementCurrentGroup();
            Undo.SetCurrentGroupName($"udit prefab instantiate '{asset.name}'");

            var instance = (GameObject)PrefabUtility.InstantiatePrefab(asset);
            if (instance == null)
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"PrefabUtility.InstantiatePrefab returned null for '{path}'.");

            Undo.RegisterCreatedObjectUndo(instance, $"udit prefab instantiate '{asset.name}'");

            if (parent != null)
                Undo.SetTransformParent(instance.transform, parent.transform, "udit prefab instantiate (parent)");

            // localPosition matches what `go create --pos` does, so creating
            // and instantiating have symmetric coordinate semantics.
            instance.transform.localPosition = pos;

            MarkActiveSceneDirty();
            var id = StableIdRegistry.ToStableId(instance);
            return new SuccessResponse(
                $"Instantiated '{asset.name}' as {id}.",
                new
                {
                    id,
                    name = instance.name,
                    prefab_source = path,
                    parent = parent != null ? StableIdRegistry.ToStableId(parent) : null,
                    local_position = new { x = instance.transform.localPosition.x, y = instance.transform.localPosition.y, z = instance.transform.localPosition.z },
                });
        }

        static object Unpack(ToolParams p)
        {
            if (EditorApplication.isPlayingOrWillChangePlaymode)
                return new ErrorResponse("Cannot unpack prefabs while in play mode.");

            var idResult = p.GetRequired("id", "'id' parameter is required for unpack.");
            if (!idResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, idResult.ErrorMessage);

            if (!StableIdRegistry.TryResolve(idResult.Value, out var go))
                return new ErrorResponse(ErrorCodes.GameObjectNotFound,
                    $"GameObject not found: {idResult.Value}.");

            // Unpack operates on prefab instances; a plain GameObject or a
            // prefab asset itself would fail in PrefabUtility with opaque
            // messages. Catch both variations here.
            if (!PrefabUtility.IsPartOfPrefabInstance(go))
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"GameObject '{go.name}' is not a prefab instance — nothing to unpack.");

            var modeStr = (p.Get("mode") ?? "root").ToLowerInvariant();
            PrefabUnpackMode mode;
            switch (modeStr)
            {
                case "root":
                case "outermost":
                case "outermostroot":
                    mode = PrefabUnpackMode.OutermostRoot;
                    break;
                case "completely":
                case "complete":
                case "all":
                    mode = PrefabUnpackMode.Completely;
                    break;
                default:
                    return new ErrorResponse(ErrorCodes.InvalidParams,
                        $"Unpack mode must be 'root' or 'completely', got '{modeStr}'.");
            }

            // Root determines the outermost prefab — we unpack that, not the
            // sub-component the user pointed at. Surface which GO actually
            // gets touched so the caller can spot nested-prefab surprises.
            var outermost = PrefabUtility.GetOutermostPrefabInstanceRoot(go) ?? go;
            var sourceAsset = PrefabUtility.GetCorrespondingObjectFromSource(outermost);
            var sourcePath = sourceAsset != null ? AssetDatabase.GetAssetPath(sourceAsset) : null;

            var dryRun = p.GetBool("dry_run");
            if (dryRun)
            {
                return new SuccessResponse(
                    $"[dry-run] Would unpack '{outermost.name}' (mode={mode}).",
                    new
                    {
                        dry_run = true,
                        would_unpack = StableIdRegistry.ToStableId(outermost),
                        mode = mode.ToString(),
                        outermost_name = outermost.name,
                        prefab_source = sourcePath,
                    });
            }

            Undo.IncrementCurrentGroup();
            Undo.SetCurrentGroupName($"udit prefab unpack '{outermost.name}'");
            PrefabUtility.UnpackPrefabInstance(outermost, mode, InteractionMode.AutomatedAction);
            MarkActiveSceneDirty();

            return new SuccessResponse(
                $"Unpacked '{outermost.name}' (mode={mode}).",
                new
                {
                    unpacked = StableIdRegistry.ToStableId(outermost),
                    name = outermost.name,
                    mode = mode.ToString(),
                    prefab_source = sourcePath,
                });
        }

        static object Apply(ToolParams p)
        {
            if (EditorApplication.isPlayingOrWillChangePlaymode)
                return new ErrorResponse("Cannot apply prefabs while in play mode.");

            var idResult = p.GetRequired("id", "'id' parameter is required for apply.");
            if (!idResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, idResult.ErrorMessage);

            if (!StableIdRegistry.TryResolve(idResult.Value, out var go))
                return new ErrorResponse(ErrorCodes.GameObjectNotFound,
                    $"GameObject not found: {idResult.Value}.");

            if (!PrefabUtility.IsPartOfPrefabInstance(go))
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"GameObject '{go.name}' is not a prefab instance — nothing to apply.");

            // ApplyPrefabInstance requires the outermost root; Unity will
            // refuse and log if you pass a nested child. Resolve it for the
            // caller so they do not need to walk the prefab chain themselves.
            var outermost = PrefabUtility.GetOutermostPrefabInstanceRoot(go) ?? go;
            var sourceAsset = PrefabUtility.GetCorrespondingObjectFromSource(outermost);
            var sourcePath = sourceAsset != null ? AssetDatabase.GetAssetPath(sourceAsset) : null;

            if (sourcePath == null)
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"Could not resolve the prefab source for '{outermost.name}' — is this a disconnected instance?");

            var dryRun = p.GetBool("dry_run");
            if (dryRun)
            {
                return new SuccessResponse(
                    $"[dry-run] Would apply '{outermost.name}' back to {sourcePath}.",
                    new
                    {
                        dry_run = true,
                        would_apply = StableIdRegistry.ToStableId(outermost),
                        name = outermost.name,
                        prefab_source = sourcePath,
                    });
            }

            Undo.IncrementCurrentGroup();
            Undo.SetCurrentGroupName($"udit prefab apply '{outermost.name}'");
            PrefabUtility.ApplyPrefabInstance(outermost, InteractionMode.AutomatedAction);
            MarkActiveSceneDirty();

            return new SuccessResponse(
                $"Applied '{outermost.name}' to {sourcePath}.",
                new
                {
                    applied = StableIdRegistry.ToStableId(outermost),
                    name = outermost.name,
                    prefab_source = sourcePath,
                });
        }

        static object FindInstances(ToolParams p)
        {
            var pathResult = p.GetRequired("path", "'path' parameter is required for find_instances.");
            if (!pathResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, pathResult.ErrorMessage);

            var path = pathResult.Value;
            if (string.IsNullOrEmpty(AssetDatabase.AssetPathToGUID(path)))
                return new ErrorResponse(ErrorCodes.AssetNotFound,
                    $"No asset at path: {path}");

            var asset = AssetDatabase.LoadAssetAtPath<GameObject>(path);
            if (asset == null)
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"Asset at '{path}' is not a GameObject (prefab).");

            if (!PrefabUtility.IsPartOfPrefabAsset(asset))
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"Asset at '{path}' is not a prefab.");

            // Walk every loaded scene for instances whose source is this
            // prefab. We only count outermost roots — a nested prefab
            // contributes exactly one hit even though its children may also
            // resolve to the same source. Matches what "Find References in
            // Scene" does in the Editor's context menu.
            var matches = new List<GameObject>();
            var transforms = UnityEngine.Object.FindObjectsByType<Transform>(FindObjectsInactive.Include);
            foreach (var t in transforms)
            {
                var go = t.gameObject;
                if (!PrefabUtility.IsPartOfPrefabInstance(go)) continue;
                if (PrefabUtility.GetOutermostPrefabInstanceRoot(go) != go) continue;

                var src = PrefabUtility.GetCorrespondingObjectFromSource(go);
                if (src == asset)
                    matches.Add(go);
            }

            var payload = new List<object>(matches.Count);
            foreach (var m in matches)
            {
                payload.Add(new
                {
                    id = StableIdRegistry.ToStableId(m),
                    name = m.name,
                    scene = m.scene.path,
                    path = ComputePath(m),
                });
            }

            return new SuccessResponse(
                $"Found {matches.Count} instance(s) of '{path}'.",
                new
                {
                    prefab = path,
                    count = matches.Count,
                    instances = payload,
                });
        }

        static string ComputePath(GameObject go)
        {
            var parts = new Stack<string>();
            var t = go.transform;
            while (t != null)
            {
                parts.Push(t.name);
                t = t.parent;
            }
            return string.Join("/", parts);
        }

        static void MarkActiveSceneDirty()
        {
            var scene = EditorSceneManager.GetActiveScene();
            if (scene.IsValid() && scene.isLoaded)
                EditorSceneManager.MarkSceneDirty(scene);
        }
    }
}
