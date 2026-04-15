using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Threading.Tasks;
using Newtonsoft.Json.Linq;
using UnityEditor;
using UnityEditor.PackageManager;
using UnityEditor.PackageManager.Requests;
using UnityEngine;

namespace UditConnector.Tools
{
    [UditTool(Description = "Unity Package Manager (UPM) operations. Actions: list, add, remove, info, search, resolve.")]
    public static class ManagePackage
    {
        public class Parameters
        {
            [ToolParameter("Action to perform: list, add, remove, info, search, resolve", Required = true)]
            public string Action { get; set; }

            [ToolParameter("Package identifier. For add: name | name@version | git URL. For remove/info: package name.")]
            public string Name { get; set; }

            [ToolParameter("Search query (for search action): substring matched against package name and displayName.")]
            public string Query { get; set; }

            [ToolParameter("For list: when true, query the resolved graph via Client.List (slower, includes transitive). Default false (manifest-only).")]
            public bool Resolved { get; set; }
        }

        public static Task<object> HandleCommand(JObject @params)
        {
            if (@params == null)
                return Task.FromResult<object>(new ErrorResponse(ErrorCodes.InvalidParams, "Parameters cannot be null."));

            var p = new ToolParams(@params);
            var actionResult = p.GetRequired("action");
            if (!actionResult.IsSuccess)
                return Task.FromResult<object>(new ErrorResponse(ErrorCodes.InvalidParams, actionResult.ErrorMessage));

            var action = actionResult.Value.ToLowerInvariant();
            switch (action)
            {
                case "list":    return List(p);
                case "add":     return Add(p);
                case "remove":  return Remove(p);
                case "info":    return Info(p);
                case "search":  return Search(p);
                case "resolve": return Resolve();
                default:
                    return Task.FromResult<object>(new ErrorResponse(ErrorCodes.InvalidParams,
                        $"Unknown action '{action}'. Available: list, add, remove, info, search, resolve."));
            }
        }

        // --- list ---------------------------------------------------------

        // The default `list` reads Packages/manifest.json directly — declared
        // versions only, sub-second response. `--resolved` switches to
        // PackageManager.Client.List which walks the resolved graph
        // (transitive deps, source kind, install path) and takes 1-3s on a
        // cold registry. Manifest-only is the right default for an agent
        // doing repeated lookups; resolved is for "what's actually installed
        // and where did it come from?".
        static Task<object> List(ToolParams p)
        {
            if (p.GetBool("resolved", false))
                return ListResolved();
            return Task.FromResult(ListDeclared());
        }

        static object ListDeclared()
        {
            var packages = ReadManifestPackages();
            return new SuccessResponse(
                $"Declared packages: {packages.Count}",
                new
                {
                    source = "manifest",
                    count = packages.Count,
                    packages,
                });
        }

        static Task<object> ListResolved()
        {
            var req = Client.List(offlineMode: false, includeIndirectDependencies: true);
            return AwaitRequest(req, listReq =>
            {
                var items = new List<object>();
                foreach (var pkg in listReq.Result)
                {
                    items.Add(new
                    {
                        name = pkg.name,
                        version = pkg.version,
                        source = pkg.source.ToString(),         // Registry / Embedded / Local / Git / BuiltIn
                        package_id = pkg.packageId,             // e.g. "com.unity.cinemachine@2.9.7"
                        display_name = pkg.displayName,
                        is_direct = pkg.isDirectDependency,
                    });
                }
                items.Sort((a, b) => string.Compare(
                    ((dynamic)a).name.ToString(),
                    ((dynamic)b).name.ToString(),
                    StringComparison.OrdinalIgnoreCase));

                return new SuccessResponse(
                    $"Resolved packages: {items.Count}",
                    new
                    {
                        source = "client.list",
                        count = items.Count,
                        packages = items,
                    });
            });
        }

        // --- add ----------------------------------------------------------

        // Identifier accepts name / name@version / git URL — Unity's Client.Add
        // parses the form. Domain reload is likely on success (new scripts get
        // imported), so the response may not reach the caller in the worst
        // case. Polling here is a deliberate v1 simplification; if reload
        // truncation becomes a real problem the pending-file pattern from
        // RunTests.PlayMode is the next step.
        static Task<object> Add(ToolParams p)
        {
            var name = (p.Get("name", "") ?? "").Trim();
            if (string.IsNullOrEmpty(name))
                return Task.FromResult<object>(new ErrorResponse(ErrorCodes.InvalidParams,
                    "Add requires a 'name' parameter (registry id, name@version, or git URL)."));

            var req = Client.Add(name);
            return AwaitRequest(req, addReq =>
            {
                var pkg = addReq.Result;
                return new SuccessResponse(
                    $"Added {pkg.name}@{pkg.version}",
                    new
                    {
                        name = pkg.name,
                        version = pkg.version,
                        source = pkg.source.ToString(),
                        package_id = pkg.packageId,
                        display_name = pkg.displayName,
                    });
            });
        }

        // --- remove -------------------------------------------------------

        static Task<object> Remove(ToolParams p)
        {
            var name = (p.Get("name", "") ?? "").Trim();
            if (string.IsNullOrEmpty(name))
                return Task.FromResult<object>(new ErrorResponse(ErrorCodes.InvalidParams,
                    "Remove requires a 'name' parameter (the package name)."));

            var req = Client.Remove(name);
            return AwaitRequest(req, _ => new SuccessResponse(
                $"Removed {name}",
                new { name }));
        }

        // --- info ---------------------------------------------------------

        // Single-package Search — Client.Search hits the registry for one
        // package, returns the latest plus a versions[] history. We surface
        // version, latest, description, registry, and sample versions so an
        // agent can decide what to add without an extra round-trip.
        static Task<object> Info(ToolParams p)
        {
            var name = (p.Get("name", "") ?? "").Trim();
            if (string.IsNullOrEmpty(name))
                return Task.FromResult<object>(new ErrorResponse(ErrorCodes.InvalidParams,
                    "Info requires a 'name' parameter (the package name)."));

            var req = Client.Search(name, offlineMode: false);
            return AwaitRequest(req, searchReq =>
            {
                var results = searchReq.Result;
                if (results == null || results.Length == 0)
                    return new ErrorResponse(ErrorCodes.InvalidParams,
                        $"No package matching '{name}' found in the configured registries.");

                var pkg = results[0];
                return new SuccessResponse(
                    $"{pkg.name} {pkg.version} ({pkg.source})",
                    new
                    {
                        name = pkg.name,
                        version = pkg.version,
                        latest = pkg.versions?.latestCompatible ?? pkg.version,
                        latest_release = pkg.versions?.latest ?? pkg.version,
                        display_name = pkg.displayName,
                        description = pkg.description,
                        category = pkg.category,
                        source = pkg.source.ToString(),
                        registry = pkg.registry?.name,
                        keywords = pkg.keywords,
                        // Last 10 versions — full list can be hundreds for old packages.
                        recent_versions = pkg.versions?.all != null
                            ? pkg.versions.all.Reverse().Take(10).ToArray()
                            : new string[0],
                    });
            });
        }

        // --- search -------------------------------------------------------

        // SearchAll returns the full registry catalog (hundreds of packages).
        // We filter client-side by substring on name + displayName so agents
        // can pass casual queries like "cinemachine" or "shader graph".
        // Capped at 50 results to keep responses bounded.
        static Task<object> Search(ToolParams p)
        {
            var query = (p.Get("query", "") ?? "").Trim();
            if (string.IsNullOrEmpty(query))
                return Task.FromResult<object>(new ErrorResponse(ErrorCodes.InvalidParams,
                    "Search requires a 'query' parameter."));

            var req = Client.SearchAll(offlineMode: false);
            return AwaitRequest(req, searchReq =>
            {
                var qLower = query.ToLowerInvariant();
                var matches = new List<object>();
                foreach (var pkg in searchReq.Result)
                {
                    var name = pkg.name ?? "";
                    var display = pkg.displayName ?? "";
                    if (name.ToLowerInvariant().Contains(qLower)
                        || display.ToLowerInvariant().Contains(qLower))
                    {
                        matches.Add(new
                        {
                            name = pkg.name,
                            version = pkg.version,
                            display_name = pkg.displayName,
                            description = pkg.description,
                            source = pkg.source.ToString(),
                        });
                        if (matches.Count >= 50) break;
                    }
                }

                return new SuccessResponse(
                    $"Found {matches.Count} match(es) for '{query}'.",
                    new
                    {
                        query,
                        count = matches.Count,
                        truncated = matches.Count >= 50,
                        results = matches,
                    });
            });
        }

        // --- resolve ------------------------------------------------------

        // Forces UPM to re-resolve manifest.json. Useful after editing the
        // manifest externally or when a previous resolve was interrupted.
        // Client.Resolve is the canonical API in Unity 6; we surface the
        // success without trying to enumerate what changed (PackageManager
        // doesn't expose a diff).
        static Task<object> Resolve()
        {
            // Client.Resolve() is internal in some versions; the public
            // surface is Client.Resolve() in Unity 2020+. If it ever
            // disappears, AssetDatabase.Refresh() is the closest fallback —
            // forces an asset re-import which triggers manifest re-read.
            try
            {
                Client.Resolve();
                return Task.FromResult<object>(new SuccessResponse(
                    "Triggered Package Manager resolve.",
                    new { method = "Client.Resolve" }));
            }
            catch (Exception ex)
            {
                // Fallback path — refresh assets, which prompts UPM to
                // re-evaluate the manifest if it's been edited on disk.
                try
                {
                    AssetDatabase.Refresh();
                    return Task.FromResult<object>(new SuccessResponse(
                        "Client.Resolve unavailable; fell back to AssetDatabase.Refresh().",
                        new { method = "AssetDatabase.Refresh", note = ex.Message }));
                }
                catch (Exception ex2)
                {
                    return Task.FromResult<object>(new ErrorResponse(
                        $"Resolve failed: {ex2.Message}"));
                }
            }
        }

        // --- helpers ------------------------------------------------------

        // Polls a PackageManager Request via EditorApplication.update — the
        // PackageManager API doesn't expose a callback, so this is the
        // canonical pattern for awaiting one as a Task. RunContinuations-
        // Asynchronously matches RunTests.cs's pattern so the HTTP response
        // path stays off the editor tick.
        static Task<object> AwaitRequest<TReq>(TReq req, Func<TReq, object> onSuccess)
            where TReq : Request
        {
            var tcs = new TaskCompletionSource<object>(TaskCreationOptions.RunContinuationsAsynchronously);
            EditorApplication.CallbackFunction tick = null;
            tick = () =>
            {
                if (!req.IsCompleted) return;
                EditorApplication.update -= tick;
                if (req.Status == StatusCode.Success)
                {
                    try { tcs.TrySetResult(onSuccess(req)); }
                    catch (Exception ex) { tcs.TrySetResult(new ErrorResponse($"Response build failed: {ex.Message}")); }
                }
                else
                {
                    var msg = req.Error?.message ?? "unknown error";
                    tcs.TrySetResult(new ErrorResponse(ErrorCodes.InvalidParams,
                        $"Package operation failed: {msg}"));
                }
            };
            EditorApplication.update += tick;
            return tcs.Task;
        }

        static List<object> ReadManifestPackages()
        {
            var result = new List<object>();
            var projectRoot = Path.GetDirectoryName(Application.dataPath);
            if (projectRoot == null) return result;

            var manifestPath = Path.Combine(projectRoot, "Packages", "manifest.json");
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
                var versionDeclared = prop.Value?.ToString() ?? "";
                // Tag git/file references so agents can tell them from
                // registry pins — registry refs are versions, others are URIs.
                var kind = "registry";
                if (versionDeclared.StartsWith("file:", StringComparison.OrdinalIgnoreCase))
                    kind = "file";
                else if (versionDeclared.StartsWith("http", StringComparison.OrdinalIgnoreCase)
                      || versionDeclared.Contains(".git"))
                    kind = "git";

                result.Add(new
                {
                    name = prop.Name,
                    version_declared = versionDeclared,
                    kind,
                });
            }
            // Stable alphabetical order so agents can diff between runs.
            result.Sort((a, b) => string.Compare(
                ((dynamic)a).name.ToString(),
                ((dynamic)b).name.ToString(),
                StringComparison.OrdinalIgnoreCase));
            return result;
        }
    }
}
