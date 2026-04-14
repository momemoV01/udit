using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using Newtonsoft.Json.Linq;
using UnityEditor;
using UnityEditor.Build;
using UnityEditor.SceneManagement;
using UnityEngine;
using UnityEngine.Rendering;
using UnityEngine.SceneManagement;

namespace UditConnector.Tools
{
    [UditTool(Description = "Project-level inspection. Actions: info, validate, preflight.")]
    public static class ManageProject
    {
        public class Parameters
        {
            [ToolParameter("Action to perform: info, validate, preflight", Required = true)]
            public string Action { get; set; }

            [ToolParameter("Restrict validate/preflight to Assets/ only, skip Packages/ (default true)")]
            public bool AssetsOnly { get; set; }

            [ToolParameter("Max issues per category for validate/preflight (default 100)")]
            public int Limit { get; set; }
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
                case "info":      return Info();
                case "validate":  return Validate(p);
                case "preflight": return Preflight(p);
                default:
                    return new ErrorResponse(ErrorCodes.InvalidParams,
                        $"Unknown action '{action}'. Available: info, validate, preflight.");
            }
        }

        // --- info ---------------------------------------------------------

        static object Info()
        {
            var scenesInBuild = new List<object>();
            var buildScenes = EditorBuildSettings.scenes;
            for (int i = 0; i < buildScenes.Length; i++)
            {
                var s = buildScenes[i];
                scenesInBuild.Add(new
                {
                    path = s.path,
                    enabled = s.enabled,
                    build_index = i,
                });
            }

            // Read Packages/manifest.json directly. PackageManager.Client.List is
            // async and typically takes 1-3s on cold registries; for a CLI info
            // dump we prefer the declared-versions view from the manifest —
            // accurate enough for "what packages does this project use?" without
            // the async wait. Agents that need the resolved graph can fall back
            // to exec/Client.List themselves.
            var packages = ReadManifestPackages();

            var renderPipelineName = "Built-in";
            if (GraphicsSettings.currentRenderPipeline != null)
                renderPipelineName = GraphicsSettings.currentRenderPipeline.GetType().Name;

            // High-level asset counts via AssetDatabase. One filter per category
            // keeps the query cheap — we're not loading the assets, just
            // counting the GUIDs.
            var allAssetsCount = AssetDatabase.FindAssets("").Length;
            var csFilesCount = AssetDatabase.FindAssets("t:MonoScript", new[] { "Assets" }).Length;
            var sceneCount = AssetDatabase.FindAssets("t:Scene").Length;
            var prefabCount = AssetDatabase.FindAssets("t:Prefab").Length;
            var materialCount = AssetDatabase.FindAssets("t:Material").Length;
            var textureCount = AssetDatabase.FindAssets("t:Texture").Length;

            return new SuccessResponse(
                $"Project: {PlayerSettings.productName} ({Application.unityVersion})",
                new
                {
                    unity_version = Application.unityVersion,
                    project_path = System.IO.Path.GetDirectoryName(Application.dataPath),
                    project_name = System.IO.Path.GetFileName(System.IO.Path.GetDirectoryName(Application.dataPath)),
                    product_name = PlayerSettings.productName,
                    company_name = PlayerSettings.companyName,
                    bundle_version = PlayerSettings.bundleVersion,
                    scripting_backend = PlayerSettings.GetScriptingBackend(NamedBuildTarget.FromBuildTargetGroup(EditorUserBuildSettings.selectedBuildTargetGroup)).ToString(),
                    color_space = PlayerSettings.colorSpace.ToString(),
                    render_pipeline = renderPipelineName,
                    active_build_target = EditorUserBuildSettings.activeBuildTarget.ToString(),
                    scenes_in_build = scenesInBuild,
                    packages,
                    stats = new
                    {
                        total_assets = allAssetsCount,
                        cs_files_in_assets = csFilesCount,
                        scenes = sceneCount,
                        prefabs = prefabCount,
                        materials = materialCount,
                        textures = textureCount,
                    },
                });
        }

        static List<object> ReadManifestPackages()
        {
            var result = new List<object>();
            var projectRoot = System.IO.Path.GetDirectoryName(Application.dataPath);
            if (projectRoot == null) return result;

            var manifestPath = System.IO.Path.Combine(projectRoot, "Packages", "manifest.json");
            if (!File.Exists(manifestPath)) return result;

            string text;
            try { text = File.ReadAllText(manifestPath); }
            catch { return result; }

            JObject manifest;
            try { manifest = JObject.Parse(text); }
            catch { return result; }

            if (manifest["dependencies"] is not JObject deps) return result;

            foreach (var prop in deps.Properties())
            {
                result.Add(new
                {
                    name = prop.Name,
                    version_declared = prop.Value?.ToString() ?? "",
                });
            }
            // Stable alphabetical order so agents can diff between runs.
            result.Sort((a, b) => string.Compare(
                ((dynamic)a).name.ToString(),
                ((dynamic)b).name.ToString(),
                StringComparison.OrdinalIgnoreCase));
            return result;
        }

        // --- validate / preflight -----------------------------------------

        static object Validate(ToolParams p)
        {
            var assetsOnly = p.GetBool("assets_only", true);
            var limit = Math.Max(1, p.GetInt("limit", 100) ?? 100);

            var started = DateTime.UtcNow;
            var issues = RunValidation(assetsOnly, limit);
            var scanMs = (long)(DateTime.UtcNow - started).TotalMilliseconds;

            var errors = issues.Count(i => ((dynamic)i).severity == "error");
            var warnings = issues.Count(i => ((dynamic)i).severity == "warning");

            return new SuccessResponse(
                $"Validation: {errors} error(s), {warnings} warning(s), scanned in {scanMs}ms.",
                new
                {
                    ok = errors == 0,
                    errors,
                    warnings,
                    assets_only = assetsOnly,
                    scan_ms = scanMs,
                    issues,
                });
        }

        static object Preflight(ToolParams p)
        {
            var assetsOnly = p.GetBool("assets_only", true);
            var limit = Math.Max(1, p.GetInt("limit", 100) ?? 100);

            var started = DateTime.UtcNow;
            var issues = RunValidation(assetsOnly, limit);

            // Preflight-specific checks on top of validate: build-readiness
            // signals that only matter when the agent is about to ship a
            // build (missing build scenes, empty product name, compile state).
            AddPreflightChecks(issues, limit);

            var scanMs = (long)(DateTime.UtcNow - started).TotalMilliseconds;
            var errors = issues.Count(i => ((dynamic)i).severity == "error");
            var warnings = issues.Count(i => ((dynamic)i).severity == "warning");

            return new SuccessResponse(
                $"Preflight: {errors} error(s), {warnings} warning(s), scanned in {scanMs}ms.",
                new
                {
                    ok = errors == 0,
                    errors,
                    warnings,
                    assets_only = assetsOnly,
                    scan_ms = scanMs,
                    issues,
                });
        }

        static List<object> RunValidation(bool assetsOnly, int limit)
        {
            var issues = new List<object>();

            // Missing-script scan: walk every prefab asset under the selected
            // root and look for null Component slots. MonoBehaviour
            // references that fail to resolve show up as null entries, which
            // is the exact shape SerializedInspect already surfaces as
            // "<Missing Script>".
            var prefabGuids = assetsOnly
                ? AssetDatabase.FindAssets("t:Prefab", new[] { "Assets" })
                : AssetDatabase.FindAssets("t:Prefab");

            foreach (var guid in prefabGuids)
            {
                if (CountIssuesForSeverity(issues, "error") >= limit) break;
                var path = AssetDatabase.GUIDToAssetPath(guid);
                if (string.IsNullOrEmpty(path)) continue;

                GameObject prefab;
                try { prefab = AssetDatabase.LoadAssetAtPath<GameObject>(path); }
                catch { continue; }
                if (prefab == null) continue;

                var missing = CountMissingScripts(prefab);
                if (missing > 0)
                {
                    issues.Add(new
                    {
                        severity = "error",
                        kind = "missing_script",
                        path,
                        missing_count = missing,
                        message = $"Prefab has {missing} missing script reference(s).",
                    });
                }
            }

            // Build-settings sanity: at least one scene should be enabled
            // when the user plans to build, otherwise the player entry point
            // is empty and Unity will silently produce an unplayable build.
            var enabledSceneCount = EditorBuildSettings.scenes.Count(s => s.enabled);
            if (enabledSceneCount == 0)
            {
                issues.Add(new
                {
                    severity = "warning",
                    kind = "build_settings",
                    message = "No scenes in Build Settings are enabled. A player build would have no entry scene.",
                });
            }

            return issues;
        }

        static void AddPreflightChecks(List<object> issues, int limit)
        {
            // Compile state — EditorApplication.isCompiling catches an active
            // compile pass, but the useful signal for preflight is whether
            // the most recent compile succeeded. Unity does not expose a
            // direct API for that; we approximate by reading the ConsoleWindow
            // error count via the internal LogEntries type. Skipping that
            // internal hop here because it is brittle across Unity versions —
            // instead we surface the active compile state and let the agent
            // follow up with `udit console --type error`.
            if (EditorApplication.isCompiling)
            {
                issues.Add(new
                {
                    severity = "warning",
                    kind = "compile_state",
                    message = "Scripts are currently compiling. Preflight results may be incomplete.",
                });
            }

            if (string.IsNullOrEmpty(PlayerSettings.productName))
            {
                issues.Add(new
                {
                    severity = "warning",
                    kind = "player_settings",
                    message = "PlayerSettings.productName is empty. Set it before shipping a build.",
                });
            }
            if (string.IsNullOrEmpty(PlayerSettings.companyName) ||
                PlayerSettings.companyName == "DefaultCompany")
            {
                issues.Add(new
                {
                    severity = "warning",
                    kind = "player_settings",
                    message = $"PlayerSettings.companyName is '{PlayerSettings.companyName}' — change from the default before shipping.",
                });
            }
        }

        static int CountMissingScripts(GameObject go)
        {
            // Walk the prefab's transform tree and count null Component slots
            // on every GameObject. GameObjectUtility has a dedicated API for
            // exactly this in modern Unity.
            int missing = 0;
            missing += GameObjectUtility.GetMonoBehavioursWithMissingScriptCount(go);
            foreach (Transform child in go.transform)
                missing += CountMissingScripts(child.gameObject);
            return missing;
        }

        static int CountIssuesForSeverity(List<object> issues, string severity)
        {
            int n = 0;
            foreach (var i in issues)
                if (((dynamic)i).severity == severity) n++;
            return n;
        }
    }
}
