using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Reflection;
using Newtonsoft.Json.Linq;
using UnityEditor;
using UnityEditor.Build;
using UnityEditor.Build.Reporting;
using UnityEngine;

namespace UditConnector.Tools
{
    [UditTool(Description = "Player builds via BuildPipeline. Actions: player, targets, addressables, cancel.")]
    public static class ManageBuild
    {
        public class Parameters
        {
            [ToolParameter("Action to perform: player, targets, addressables, cancel", Required = true)]
            public string Action { get; set; }

            [ToolParameter("BuildTarget for player: win64 / win32 / mac / linux / android / ios / webgl, or full enum name (StandaloneWindows64, etc.)")]
            public string Target { get; set; }

            [ToolParameter("Output path for player build (absolute recommended; CLI resolves relative against its cwd).")]
            public string Output { get; set; }

            [ToolParameter("Scene paths for player (string array). When omitted, falls back to enabled scenes in Build Settings.")]
            public string[] Scenes { get; set; }

            [ToolParameter("Development build (BuildOptions.Development). Default false.")]
            public bool Development { get; set; }

            [ToolParameter("Addressables profile id or name (optional, for addressables action). Default uses the current active profile.")]
            public string Profile { get; set; }
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
                case "player":       return Player(p);
                case "targets":      return Targets();
                case "addressables": return Addressables(p);
                case "cancel":       return Cancel();
                default:
                    return new ErrorResponse(ErrorCodes.InvalidParams,
                        $"Unknown action '{action}'. Available: player, targets, addressables, cancel.");
            }
        }

        // --- targets ------------------------------------------------------

        // Enumerates every BuildTarget enum value and reports whether the
        // local Editor install supports it (BuildPipeline.IsBuildTargetSupported).
        // Useful for an agent to discover what's actually buildable on the
        // current machine before attempting a `build player --target X`.
        static object Targets()
        {
            var items = new List<object>();
            foreach (BuildTarget t in Enum.GetValues(typeof(BuildTarget)))
            {
                // Some enum values are deprecated/internal placeholders; skip
                // anything that maps to a negative integer (Unity uses those
                // for legacy "Unknown" / "NoTarget" sentinels).
                if ((int)t < 0) continue;
                BuildTargetGroup group;
                try { group = BuildPipeline.GetBuildTargetGroup(t); }
                catch { continue; }

                bool supported;
                try { supported = BuildPipeline.IsBuildTargetSupported(group, t); }
                catch { supported = false; }

                items.Add(new
                {
                    name = t.ToString(),
                    group = group.ToString(),
                    supported,
                });
            }
            items.Sort((a, b) => string.Compare(
                ((dynamic)a).name.ToString(),
                ((dynamic)b).name.ToString(),
                StringComparison.OrdinalIgnoreCase));

            int supportedCount = items.Count(i => (bool)((dynamic)i).supported);

            return new SuccessResponse(
                $"{supportedCount}/{items.Count} BuildTargets supported on this Editor install.",
                new
                {
                    active = EditorUserBuildSettings.activeBuildTarget.ToString(),
                    count = items.Count,
                    supported_count = supportedCount,
                    targets = items,
                });
        }

        // --- player -------------------------------------------------------

        static object Player(ToolParams p)
        {
            var targetStr = p.Get("target", "");
            if (string.IsNullOrEmpty(targetStr))
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    "Player build requires --target (e.g. win64, android).");

            var target = ParseTarget(targetStr);
            if (target == null)
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"Unknown target '{targetStr}'. Aliases: win64/win32/mac/linux/android/ios/webgl, or full enum name.");

            var output = p.Get("output", "");
            if (string.IsNullOrEmpty(output))
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    "Player build requires --output (path to write the built player to).");

            // Scene array — explicit list wins; otherwise fall back to the
            // user's Build Settings (enabled-only). This mirrors what the
            // Build Settings dialog does when you click Build.
            string[] scenes = ExtractScenes(p);
            if (scenes.Length == 0)
            {
                scenes = EditorBuildSettings.scenes
                    .Where(s => s.enabled)
                    .Select(s => s.path)
                    .ToArray();
            }
            if (scenes.Length == 0)
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    "No scenes to build. Provide --scenes or enable at least one scene in Build Settings.");

            var options = BuildOptions.None;
            if (p.GetBool("development", false))
                options |= BuildOptions.Development;

            // Make sure the parent directory exists — BuildPipeline writes
            // straight into locationPathName, and on Windows-style file paths
            // it will fail rather than create the directory.
            try
            {
                var parent = Path.GetDirectoryName(output);
                if (!string.IsNullOrEmpty(parent) && !Directory.Exists(parent))
                    Directory.CreateDirectory(parent);
            }
            catch (Exception ex)
            {
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"Failed to prepare output directory: {ex.Message}");
            }

            var buildOptions = new BuildPlayerOptions
            {
                scenes = scenes,
                locationPathName = output,
                target = target.Value,
                targetGroup = BuildPipeline.GetBuildTargetGroup(target.Value),
                options = options,
            };

            // Scripting backend override — temporary switch to IL2CPP for the
            // duration of this build. The previous backend is captured before
            // the flip and restored in finally so Mono-only projects don't
            // inherit a permanent IL2CPP setting. Caveat: if the Editor
            // crashes mid-build the restore never runs and PlayerSettings is
            // left in the IL2CPP state — documented as a best-effort behavior.
            var wantIl2cpp = p.GetBool("il2cpp", false);
            NamedBuildTarget namedTarget = default;
            ScriptingImplementation? previousBackend = null;
            if (wantIl2cpp)
            {
                try
                {
                    namedTarget = NamedBuildTarget.FromBuildTargetGroup(buildOptions.targetGroup);
                    previousBackend = PlayerSettings.GetScriptingBackend(namedTarget);
                    if (previousBackend.Value != ScriptingImplementation.IL2CPP)
                    {
                        PlayerSettings.SetScriptingBackend(namedTarget, ScriptingImplementation.IL2CPP);
                    }
                    else
                    {
                        // Already IL2CPP — no flip needed, skip restore too.
                        previousBackend = null;
                    }
                }
                catch (Exception ex)
                {
                    return new ErrorResponse(ErrorCodes.InvalidParams,
                        $"Could not switch scripting backend to IL2CPP: {ex.Message}");
                }
            }

            BuildReport report;
            try
            {
                try
                {
                    report = BuildPipeline.BuildPlayer(buildOptions);
                }
                catch (Exception ex)
                {
                    return new ErrorResponse(ErrorCodes.InvalidParams,
                        $"BuildPipeline.BuildPlayer threw: {ex.Message}");
                }
            }
            finally
            {
                if (previousBackend.HasValue)
                {
                    try { PlayerSettings.SetScriptingBackend(namedTarget, previousBackend.Value); }
                    catch { /* best-effort; ProjectSettings.asset may be dirty in VCS either way */ }
                }
            }

            var summary = report.summary;
            var data = new
            {
                result = summary.result.ToString(),
                platform = summary.platform.ToString(),
                output_path = summary.outputPath,
                total_size = (long)summary.totalSize,
                total_errors = summary.totalErrors,
                total_warnings = summary.totalWarnings,
                duration_sec = summary.totalTime.TotalSeconds,
                build_started_at = summary.buildStartedAt.ToString("o"),
                build_ended_at = summary.buildEndedAt.ToString("o"),
                steps_count = report.steps?.Length ?? 0,
                scenes_count = scenes.Length,
            };

            // Failed/Cancelled go through ErrorResponse so the agent gets a
            // truthy `success: false` plus the same payload — matches how
            // RunTests reports test-failure (still a valid response, just
            // not a "succeeded" build).
            if (summary.result != BuildResult.Succeeded)
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"Build {summary.result}: {summary.totalErrors} error(s), {summary.totalWarnings} warning(s).",
                    data);

            var sizeMB = summary.totalSize / 1024.0 / 1024.0;
            return new SuccessResponse(
                $"Build Succeeded: {sizeMB:F1}MB in {summary.totalTime.TotalSeconds:F1}s.",
                data);
        }

        static string[] ExtractScenes(ToolParams p)
        {
            // ToolParams may surface scenes as a JArray (when the wire payload
            // sent it as a JSON array) or as a single string (rare). We accept
            // both via GetRaw to inspect the underlying JToken type.
            var raw = p.GetRaw("scenes");
            if (raw == null) return Array.Empty<string>();
            if (raw.Type == JTokenType.Array)
                return raw.Select(t => t.ToString()).Where(s => !string.IsNullOrEmpty(s)).ToArray();
            if (raw.Type == JTokenType.String)
            {
                var s = raw.ToString();
                return string.IsNullOrEmpty(s)
                    ? Array.Empty<string>()
                    : s.Split(',').Select(x => x.Trim()).Where(x => x != "").ToArray();
            }
            return Array.Empty<string>();
        }

        static BuildTarget? ParseTarget(string s)
        {
            s = (s ?? "").Trim();
            if (s.Length == 0) return null;
            switch (s.ToLowerInvariant())
            {
                case "win64":
                case "windows64":
                case "windows":
                case "standalonewindows64":
                    return BuildTarget.StandaloneWindows64;
                case "win32":
                case "windows32":
                case "standalonewindows":
                    return BuildTarget.StandaloneWindows;
                case "mac":
                case "osx":
                case "macos":
                case "standaloneosx":
                    return BuildTarget.StandaloneOSX;
                case "linux":
                case "linux64":
                case "standalonelinux64":
                    return BuildTarget.StandaloneLinux64;
                case "android":
                    return BuildTarget.Android;
                case "ios":
                    return BuildTarget.iOS;
                case "webgl":
                    return BuildTarget.WebGL;
            }
            // Last-resort: try the enum name directly (case-insensitive). This
            // covers PS5 / XboxSeries / Switch and any future BuildTarget values
            // without requiring the alias map to grow forever.
            if (Enum.TryParse<BuildTarget>(s, true, out var t) && Enum.IsDefined(typeof(BuildTarget), t))
                return t;
            return null;
        }

        // --- addressables -------------------------------------------------

        // Reflection-only — udit-connector doesn't depend on
        // com.unity.addressables, so the C# code must compile (and the rest
        // of the connector keep working) on projects without the package.
        // If the package isn't installed, return a clear error instead of
        // a confusing "type not found" stack trace.
        static object Addressables(ToolParams p)
        {
            // Try the canonical entry-point type first; fall back to the
            // older settings-default-object type for older Addressables
            // versions where the API surface lived elsewhere.
            var settingsDefaultType = ResolveType(
                "UnityEditor.AddressableAssets.AddressableAssetSettingsDefaultObject, Unity.Addressables.Editor");
            var contentBuilderType = ResolveType(
                "UnityEditor.AddressableAssets.Settings.AddressableAssetSettings, Unity.Addressables.Editor");

            if (settingsDefaultType == null || contentBuilderType == null)
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    "Addressables package (com.unity.addressables) is not installed in this project. " +
                    "Add it via: udit package add com.unity.addressables");

            // Get the active settings instance via the static Settings property.
            var settingsProp = settingsDefaultType.GetProperty(
                "Settings", BindingFlags.Public | BindingFlags.Static);
            object settings = null;
            try { settings = settingsProp?.GetValue(null); }
            catch (Exception ex)
            {
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"Failed to read Addressables settings: {ex.Message}");
            }
            if (settings == null)
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    "Addressables is installed but no AddressableAssetSettings asset exists. " +
                    "Open Window > Asset Management > Addressables > Groups to create one.");

            // Optional profile switch — best-effort; on any reflection
            // mismatch we just leave the active profile alone and proceed.
            var profileName = p.Get("profile", "");
            string previousProfileId = null;
            string usedProfileId = null;
            if (!string.IsNullOrEmpty(profileName))
            {
                try
                {
                    var profileSettingsProp = contentBuilderType.GetProperty("profileSettings");
                    var profileSettings = profileSettingsProp?.GetValue(settings);
                    var getIdMethod = profileSettings?.GetType().GetMethod("GetProfileId", new[] { typeof(string) });
                    var profileId = getIdMethod?.Invoke(profileSettings, new object[] { profileName }) as string;
                    if (!string.IsNullOrEmpty(profileId))
                    {
                        var activeProfileIdProp = contentBuilderType.GetProperty("activeProfileId");
                        previousProfileId = activeProfileIdProp?.GetValue(settings) as string;
                        activeProfileIdProp?.SetValue(settings, profileId);
                        usedProfileId = profileId;
                    }
                }
                catch
                {
                    // Profile switch failed — not fatal, just note it in the
                    // response so the agent can inspect.
                }
            }

            // Find BuildPlayerContent — there are several overloads across
            // versions; prefer the parameterless one.
            var buildMethod = contentBuilderType.GetMethod(
                "BuildPlayerContent",
                BindingFlags.Public | BindingFlags.Static,
                null,
                Type.EmptyTypes,
                null);

            try
            {
                if (buildMethod != null)
                    buildMethod.Invoke(null, null);
                else
                    return new ErrorResponse(ErrorCodes.InvalidParams,
                        "AddressableAssetSettings.BuildPlayerContent() not found. " +
                        "Addressables API may have changed; please report.");
            }
            catch (TargetInvocationException tex)
            {
                var inner = tex.InnerException?.Message ?? tex.Message;
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"Addressables build failed: {inner}");
            }
            catch (Exception ex)
            {
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"Addressables build failed: {ex.Message}");
            }
            finally
            {
                if (previousProfileId != null)
                {
                    try
                    {
                        var activeProfileIdProp = contentBuilderType.GetProperty("activeProfileId");
                        activeProfileIdProp?.SetValue(settings, previousProfileId);
                    }
                    catch { /* ignore — best-effort restore */ }
                }
            }

            return new SuccessResponse(
                "Addressables BuildPlayerContent completed.",
                new
                {
                    profile_used = usedProfileId,
                    profile_requested = string.IsNullOrEmpty(profileName) ? null : profileName,
                });
        }

        static Type ResolveType(string assemblyQualifiedName)
        {
            try
            {
                var t = Type.GetType(assemblyQualifiedName);
                if (t != null) return t;
            }
            catch { }
            // Fallback — scan loaded assemblies (the assembly-qualified form
            // doesn't always resolve under Unity's domain).
            var typeName = assemblyQualifiedName.Split(',')[0].Trim();
            foreach (var asm in AppDomain.CurrentDomain.GetAssemblies())
            {
                try
                {
                    var t = asm.GetType(typeName);
                    if (t != null) return t;
                }
                catch { }
            }
            return null;
        }

        // --- cancel -------------------------------------------------------

        // BuildPipeline.CancelBuild was a public API in older Unity (5.x /
        // 2017) but is not part of the public surface in Unity 6 — possibly
        // internal, possibly removed. Try reflection so we still call it
        // when available, fall back to a clear unsupported message
        // otherwise. There's no public way to query "is a build active?"
        // either, so on success we just report the call was issued.
        static object Cancel()
        {
            var method = typeof(BuildPipeline).GetMethod(
                "CancelBuild",
                BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.Static,
                null,
                Type.EmptyTypes,
                null);

            if (method == null)
            {
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    "BuildPipeline.CancelBuild is not exposed in this Unity version. " +
                    "Cancel from the Editor UI (build progress dialog) or kill the Unity process.");
            }
            try
            {
                method.Invoke(null, null);
                return new SuccessResponse(
                    "CancelBuild called. No effect if no build is in progress.",
                    new { called = true, via = method.IsPublic ? "public" : "reflection" });
            }
            catch (TargetInvocationException tex)
            {
                var inner = tex.InnerException?.Message ?? tex.Message;
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"CancelBuild failed: {inner}");
            }
            catch (Exception ex)
            {
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"CancelBuild failed: {ex.Message}");
            }
        }
    }
}
