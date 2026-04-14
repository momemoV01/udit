using System;
using System.Collections.Generic;
using System.Linq;
using System.Text.RegularExpressions;
using Newtonsoft.Json.Linq;
using UditConnector.Tools.Common;
using UnityEditor;
using UnityEngine;

namespace UditConnector.Tools
{
    [UditTool(Description = "Query project assets. Actions: find, inspect, dependencies, references, guid, path.")]
    public static class ManageAsset
    {
        const int DefaultLimit = 100;
        const int MaxLimit = 1000;
        const int ReferencesDefaultLimit = 100;

        public class Parameters
        {
            [ToolParameter("Action to perform: find, inspect, dependencies, references, guid, path", Required = true)]
            public string Action { get; set; }

            [ToolParameter("Asset path relative to project root (required for inspect, dependencies, references, guid)")]
            public string Path { get; set; }

            [ToolParameter("Asset GUID (required for path)")]
            public string Guid { get; set; }

            [ToolParameter("Type filter for find (e.g. Prefab, Texture2D, Material). Maps to AssetDatabase 't:' filter.")]
            public string Type { get; set; }

            [ToolParameter("Label filter for find. Maps to AssetDatabase 'l:' filter.")]
            public string Label { get; set; }

            [ToolParameter("Name glob for find (case-insensitive, '*' wildcard). Applied after AssetDatabase filters.")]
            public string Name { get; set; }

            [ToolParameter("Limit search to these folder paths for find (comma-separated, e.g. Assets/Prefabs,Assets/Scenes)")]
            public string Folder { get; set; }

            [ToolParameter("Max results for find/references (default 100, max 1000)")]
            public int Limit { get; set; }

            [ToolParameter("Skip first N matches for find/references (default 0)")]
            public int Offset { get; set; }

            [ToolParameter("Recursive dependency walk (dependencies only, default false = direct deps only)")]
            public bool Recursive { get; set; }
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
                case "find":         return Find(p);
                case "inspect":      return Inspect(p);
                case "dependencies": return Dependencies(p);
                case "references":   return References(p);
                case "guid":         return Guid(p);
                case "path":         return PathFromGuid(p);
                default:
                    return new ErrorResponse(ErrorCodes.InvalidParams,
                        $"Unknown action '{action}'. Available: find, inspect, dependencies, references, guid, path.");
            }
        }

        static object Find(ToolParams p)
        {
            var type = p.Get("type");
            var label = p.Get("label");
            var name = p.Get("name");
            var folder = p.Get("folder");
            var limit = Clamp(p.GetInt("limit", DefaultLimit) ?? DefaultLimit, 1, MaxLimit);
            var offset = Math.Max(0, p.GetInt("offset", 0) ?? 0);

            // Build AssetDatabase filter string. Empty filter returns everything,
            // which is intentional — without `t:`/`l:` the agent is asking for a
            // whole-project scan with name filtering only.
            var filterParts = new List<string>();
            if (!string.IsNullOrEmpty(type))  filterParts.Add("t:" + type);
            if (!string.IsNullOrEmpty(label)) filterParts.Add("l:" + label);
            var filter = string.Join(" ", filterParts);

            string[] searchFolders = null;
            if (!string.IsNullOrEmpty(folder))
            {
                searchFolders = folder
                    .Split(',')
                    .Select(s => s.Trim())
                    .Where(s => s.Length > 0)
                    .ToArray();
                if (searchFolders.Length == 0) searchFolders = null;
            }

            var guids = searchFolders != null
                ? AssetDatabase.FindAssets(filter, searchFolders)
                : AssetDatabase.FindAssets(filter);

            // Convert to path + main-type entries, then apply the optional name
            // glob. We do glob matching here (not via AssetDatabase) because
            // AssetDatabase treats the free-text term as a substring, not a
            // wildcard pattern.
            Regex nameRegex = null;
            if (!string.IsNullOrEmpty(name))
            {
                var pattern = "^" + Regex.Escape(name).Replace("\\*", ".*") + "$";
                nameRegex = new Regex(pattern, RegexOptions.IgnoreCase);
            }

            var entries = new List<(string path, string guid, string typeName)>();
            foreach (var guid in guids)
            {
                var path = AssetDatabase.GUIDToAssetPath(guid);
                if (string.IsNullOrEmpty(path)) continue;

                var fileName = System.IO.Path.GetFileNameWithoutExtension(path);
                if (nameRegex != null && !nameRegex.IsMatch(fileName)) continue;

                var t = AssetDatabase.GetMainAssetTypeAtPath(path);
                entries.Add((path, guid, t != null ? t.Name : "Unknown"));
            }

            entries.Sort((a, b) => string.Compare(a.path, b.path, StringComparison.OrdinalIgnoreCase));

            var total = entries.Count;
            var returned = new List<object>();
            for (int i = offset; i < Math.Min(offset + limit, total); i++)
            {
                var (path, guid, typeName) = entries[i];
                returned.Add(new
                {
                    path,
                    guid,
                    name = System.IO.Path.GetFileNameWithoutExtension(path),
                    type = typeName,
                });
            }

            return new SuccessResponse(
                $"Matched {total} asset(s), returning {returned.Count}.",
                new
                {
                    filter = string.IsNullOrEmpty(filter) ? null : filter,
                    folders = searchFolders,
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
            var pathResult = p.GetRequired("path", "'path' parameter is required for inspect.");
            if (!pathResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, pathResult.ErrorMessage);

            var path = pathResult.Value;
            var guid = AssetDatabase.AssetPathToGUID(path);
            if (string.IsNullOrEmpty(guid))
                return new ErrorResponse(ErrorCodes.AssetNotFound, $"Asset not found: {path}");

            var main = AssetDatabase.LoadMainAssetAtPath(path);
            if (main == null)
                return new ErrorResponse(ErrorCodes.AssetNotFound, $"Asset not loadable: {path}");

            var labels = AssetDatabase.GetLabels(main);
            var mainType = main.GetType();

            // Per-type detail blocks. We keep these lightweight — one handler per
            // common Unity asset type, enough to give an agent a useful summary
            // without paying the cost of full SerializedObject walks everywhere.
            object details;
            switch (main)
            {
                case Texture2D tex:            details = DescribeTexture(tex); break;
                case Material mat:             details = DescribeMaterial(mat); break;
                case AudioClip clip:           details = DescribeAudioClip(clip); break;
                case GameObject go when PrefabUtility.IsPartOfPrefabAsset(go):
                                               details = DescribePrefab(go); break;
                case ScriptableObject so:      details = SerializedInspect.ObjectToJson(so); break;
                case TextAsset ta:             details = DescribeTextAsset(ta); break;
                default:                       details = null; break;
            }

            return new SuccessResponse(
                $"Asset: {path}",
                new
                {
                    path,
                    guid,
                    name = main.name,
                    type = mainType.Name,
                    full_type = mainType.FullName,
                    labels = labels ?? new string[0],
                    details,
                });
        }

        static object Dependencies(ToolParams p)
        {
            var pathResult = p.GetRequired("path", "'path' parameter is required for dependencies.");
            if (!pathResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, pathResult.ErrorMessage);

            var path = pathResult.Value;
            if (string.IsNullOrEmpty(AssetDatabase.AssetPathToGUID(path)))
                return new ErrorResponse(ErrorCodes.AssetNotFound, $"Asset not found: {path}");

            var recursive = p.GetBool("recursive");
            var deps = AssetDatabase.GetDependencies(path, recursive);

            // Drop self from the list and enrich with type/guid so agents can
            // branch on the kind of reference (Material vs Script etc.) without
            // a follow-up call per path.
            var self = path;
            var list = new List<object>();
            foreach (var d in deps)
            {
                if (d == self) continue;
                var g = AssetDatabase.AssetPathToGUID(d);
                var t = AssetDatabase.GetMainAssetTypeAtPath(d);
                list.Add(new
                {
                    path = d,
                    guid = g,
                    type = t != null ? t.Name : "Unknown",
                });
            }

            return new SuccessResponse(
                $"{list.Count} dependency({(list.Count == 1 ? "" : "ies")}) of {path}.",
                new
                {
                    path,
                    recursive,
                    count = list.Count,
                    dependencies = list,
                });
        }

        static object References(ToolParams p)
        {
            var pathResult = p.GetRequired("path", "'path' parameter is required for references.");
            if (!pathResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, pathResult.ErrorMessage);

            var path = pathResult.Value;
            if (string.IsNullOrEmpty(AssetDatabase.AssetPathToGUID(path)))
                return new ErrorResponse(ErrorCodes.AssetNotFound, $"Asset not found: {path}");

            var limit = Clamp(p.GetInt("limit", ReferencesDefaultLimit) ?? ReferencesDefaultLimit, 1, MaxLimit);
            var offset = Math.Max(0, p.GetInt("offset", 0) ?? 0);

            // Reverse dependency lookup: Unity does not expose an index, so the
            // honest implementation is a full-project scan. We report scan_ms so
            // agents on large projects know this call is expensive. The outer
            // limit / offset pagination keeps the response bounded.
            var started = DateTime.UtcNow;
            var allGuids = AssetDatabase.FindAssets("");
            var matches = new List<string>();
            foreach (var guid in allGuids)
            {
                var candidate = AssetDatabase.GUIDToAssetPath(guid);
                if (string.IsNullOrEmpty(candidate) || candidate == path) continue;

                // Direct dependencies only. Recursive would require loading every
                // asset's full dep tree — usually overkill for "who references X"
                // and easy to misinterpret.
                var deps = AssetDatabase.GetDependencies(candidate, false);
                if (Array.IndexOf(deps, path) >= 0)
                    matches.Add(candidate);
            }

            matches.Sort(StringComparer.OrdinalIgnoreCase);

            var scanMs = (DateTime.UtcNow - started).TotalMilliseconds;
            var total = matches.Count;
            var slice = new List<object>();
            for (int i = offset; i < Math.Min(offset + limit, total); i++)
            {
                var candidate = matches[i];
                var t = AssetDatabase.GetMainAssetTypeAtPath(candidate);
                slice.Add(new
                {
                    path = candidate,
                    guid = AssetDatabase.AssetPathToGUID(candidate),
                    type = t != null ? t.Name : "Unknown",
                });
            }

            return new SuccessResponse(
                $"{total} direct reference(s) to {path} (scanned {allGuids.Length} assets in {(int)scanMs} ms).",
                new
                {
                    path,
                    total,
                    offset,
                    limit,
                    returned = slice.Count,
                    has_more = offset + slice.Count < total,
                    scanned_assets = allGuids.Length,
                    scan_ms = (int)scanMs,
                    references = slice,
                });
        }

        static object Guid(ToolParams p)
        {
            var pathResult = p.GetRequired("path", "'path' parameter is required for guid.");
            if (!pathResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, pathResult.ErrorMessage);

            var path = pathResult.Value;
            var guid = AssetDatabase.AssetPathToGUID(path);
            if (string.IsNullOrEmpty(guid))
                return new ErrorResponse(ErrorCodes.AssetNotFound, $"Asset not found: {path}");

            return new SuccessResponse($"GUID for {path}.", new { path, guid });
        }

        static object PathFromGuid(ToolParams p)
        {
            var guidResult = p.GetRequired("guid", "'guid' parameter is required for path.");
            if (!guidResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, guidResult.ErrorMessage);

            var guid = guidResult.Value;
            var path = AssetDatabase.GUIDToAssetPath(guid);
            if (string.IsNullOrEmpty(path))
                return new ErrorResponse(ErrorCodes.AssetNotFound, $"No asset for GUID: {guid}");

            return new SuccessResponse($"Path for {guid}.", new { guid, path });
        }

        // --- type-specific inspect details ---------------------------------

        static object DescribeTexture(Texture2D tex)
        {
            return new
            {
                width = tex.width,
                height = tex.height,
                format = tex.format.ToString(),
                filter_mode = tex.filterMode.ToString(),
                wrap_mode = tex.wrapMode.ToString(),
                mip_count = tex.mipmapCount,
                is_readable = tex.isReadable,
            };
        }

        static object DescribeMaterial(Material mat)
        {
            var shader = mat.shader;
            // Property enumeration via ShaderUtil gives the authored set (what
            // shows up in the Inspector). We keep it lightweight here — name
            // and type only — because values can be large (textures) and
            // agents that need them can call `asset inspect` again or use
            // SerializedInspect via `exec`.
            var properties = new List<object>();
            if (shader != null)
            {
                var count = ShaderUtil.GetPropertyCount(shader);
                for (int i = 0; i < count; i++)
                {
                    var propName = ShaderUtil.GetPropertyName(shader, i);
                    var propType = ShaderUtil.GetPropertyType(shader, i);
                    object value = null;
                    try
                    {
                        switch (propType)
                        {
                            case ShaderUtil.ShaderPropertyType.Color:  value = Col(mat.GetColor(propName)); break;
                            case ShaderUtil.ShaderPropertyType.Float:
                            case ShaderUtil.ShaderPropertyType.Range:  value = mat.GetFloat(propName); break;
                            case ShaderUtil.ShaderPropertyType.Vector: value = V4(mat.GetVector(propName)); break;
                            case ShaderUtil.ShaderPropertyType.TexEnv:
                                var tex = mat.GetTexture(propName);
                                value = tex != null ? new { type = tex.GetType().Name, name = tex.name } : null;
                                break;
                            case ShaderUtil.ShaderPropertyType.Int:    value = mat.GetInteger(propName); break;
                        }
                    }
                    catch { /* material may not have this property set */ }

                    properties.Add(new
                    {
                        name = propName,
                        type = propType.ToString(),
                        value,
                    });
                }
            }

            return new
            {
                shader = shader != null ? shader.name : null,
                render_queue = mat.renderQueue,
                keywords = mat.shaderKeywords ?? new string[0],
                property_count = properties.Count,
                properties,
            };
        }

        static object DescribeAudioClip(AudioClip clip)
        {
            return new
            {
                length_seconds = clip.length,
                frequency = clip.frequency,
                channels = clip.channels,
                samples = clip.samples,
                load_type = clip.loadType.ToString(),
                preload_audio_data = clip.preloadAudioData,
            };
        }

        static object DescribePrefab(GameObject prefabRoot)
        {
            // Top-level summary only. The full prefab hierarchy is reachable via
            // `scene tree` after opening the prefab, or via `go inspect` once
            // the agent has a stable ID. Keeping this block small means `asset
            // inspect Assets/Prefabs/Big.prefab` does not balloon on complex
            // prefabs.
            var components = prefabRoot.GetComponents<Component>()
                .Where(c => c != null)
                .Select(c => c.GetType().Name)
                .ToList();

            return new
            {
                name = prefabRoot.name,
                tag = prefabRoot.tag,
                layer = LayerMask.LayerToName(prefabRoot.layer),
                root_components = components,
                child_count = prefabRoot.transform.childCount,
            };
        }

        static object DescribeTextAsset(TextAsset ta)
        {
            var text = ta.text ?? string.Empty;
            const int preview = 500;
            return new
            {
                length = text.Length,
                preview = text.Length > preview ? text.Substring(0, preview) : text,
                truncated = text.Length > preview,
            };
        }

        static object V4(Vector4 v) => new { x = v.x, y = v.y, z = v.z, w = v.w };
        static object Col(Color c) => new { r = c.r, g = c.g, b = c.b, a = c.a };

        static int Clamp(int v, int lo, int hi) => v < lo ? lo : (v > hi ? hi : v);
    }
}
