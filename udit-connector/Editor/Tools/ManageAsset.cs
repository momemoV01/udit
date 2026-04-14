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
    [UditTool(Description = "Query and mutate project assets. Actions: find, inspect, dependencies, references, guid, path, create, move, delete, label.")]
    public static class ManageAsset
    {
        const int DefaultLimit = 100;
        const int MaxLimit = 1000;
        const int ReferencesDefaultLimit = 100;

        public class Parameters
        {
            [ToolParameter("Action to perform: find, inspect, dependencies, references, guid, path, create, move, delete, label", Required = true)]
            public string Action { get; set; }

            [ToolParameter("Asset path relative to project root (inspect/dependencies/references/guid/create/move/delete/label)")]
            public string Path { get; set; }

            [ToolParameter("Asset GUID (required for path)")]
            public string Guid { get; set; }

            [ToolParameter("Type filter for find OR type name for create (e.g. MyGame.GameConfig, Folder)")]
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

            [ToolParameter("Destination path for move")]
            public string Dst { get; set; }

            [ToolParameter("Permanently delete (default: move to trash, recoverable)")]
            public bool Permanent { get; set; }

            [ToolParameter("Label sub-op: add, remove, list, set, clear")]
            public string LabelOp { get; set; }

            [ToolParameter("Labels (comma-separated) for label add/remove/set")]
            public string Labels { get; set; }

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
                case "find":         return Find(p);
                case "inspect":      return Inspect(p);
                case "dependencies": return Dependencies(p);
                case "references":   return References(p);
                case "guid":         return Guid(p);
                case "path":         return PathFromGuid(p);
                case "create":       return Create(p);
                case "move":         return Move(p);
                case "delete":       return Delete(p);
                case "label":        return Label(p);
                default:
                    return new ErrorResponse(ErrorCodes.InvalidParams,
                        $"Unknown action '{action}'. Available: find, inspect, dependencies, references, guid, path, create, move, delete, label.");
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

        // --- Mutations -----------------------------------------------------
        //
        // Note on Undo: AssetDatabase operations (CreateAsset, MoveAsset,
        // DeleteAsset, SetLabels) do NOT participate in Unity's scene Undo
        // system. They write straight to disk and to the AssetDatabase index.
        // Ctrl+Z in the Editor will not reverse these — the safeguards are
        // `--dry-run` for preview, and `delete` defaulting to MoveAssetToTrash
        // so recovery is possible from the OS trash. This is documented in
        // the user-facing help and README.

        static object Create(ToolParams p)
        {
            var typeResult = p.GetRequired("type", "'type' parameter is required for create (e.g. --type MyGame.GameConfig, --type Folder).");
            if (!typeResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, typeResult.ErrorMessage);

            var pathResult = p.GetRequired("path", "'path' parameter is required for create.");
            if (!pathResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, pathResult.ErrorMessage);

            var typeName = typeResult.Value;
            var requestedPath = pathResult.Value;
            var dryRun = p.GetBool("dry_run");

            // Folder: special cased because it flows through
            // AssetDatabase.CreateFolder, which takes (parent, name) rather
            // than a full path the way CreateAsset does.
            if (string.Equals(typeName, "Folder", StringComparison.OrdinalIgnoreCase))
            {
                return CreateFolder(requestedPath, dryRun);
            }

            // Resolve the type. We accept full-qualified names preferentially
            // so "MyGame.GameConfig" disambiguates against UnityEngine types.
            var type = ResolveScriptableObjectType(typeName);
            if (type == null)
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"Type '{typeName}' not found in loaded assemblies, or is not a ScriptableObject subclass. " +
                    $"This version of `asset create` supports ScriptableObject-derived types and 'Folder'.");

            // Resolve the target file path. If the caller passed a folder
            // (trailing '/' or the path resolves to a folder), default the
            // filename to "<TypeName>.asset".
            var resolvedPath = ResolveAssetFilePath(requestedPath, type.Name);
            if (resolvedPath.error != null)
                return resolvedPath.error;

            if (!string.IsNullOrEmpty(AssetDatabase.AssetPathToGUID(resolvedPath.path)))
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"An asset already exists at '{resolvedPath.path}'. Move or delete it first.");

            if (dryRun)
            {
                return new SuccessResponse(
                    $"[dry-run] Would create {type.Name} at {resolvedPath.path}.",
                    new
                    {
                        dry_run = true,
                        would_create = resolvedPath.path,
                        type = type.Name,
                        full_type = type.FullName,
                    });
            }

            ScriptableObject instance;
            try
            {
                instance = ScriptableObject.CreateInstance(type);
            }
            catch (Exception ex)
            {
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"ScriptableObject.CreateInstance({type.Name}) failed: {ex.Message}");
            }
            if (instance == null)
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"ScriptableObject.CreateInstance({type.Name}) returned null.");

            try
            {
                AssetDatabase.CreateAsset(instance, resolvedPath.path);
                AssetDatabase.SaveAssets();
            }
            catch (Exception ex)
            {
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"AssetDatabase.CreateAsset('{resolvedPath.path}') failed: {ex.Message}");
            }

            var guid = AssetDatabase.AssetPathToGUID(resolvedPath.path);
            return new SuccessResponse(
                $"Created {type.Name} at {resolvedPath.path}.",
                new
                {
                    path = resolvedPath.path,
                    guid,
                    type = type.Name,
                    full_type = type.FullName,
                });
        }

        static object CreateFolder(string requestedPath, bool dryRun)
        {
            // AssetDatabase.CreateFolder expects (parent, childName). Split
            // the requested path ourselves so callers can type the full
            // "Assets/Data/Enemies" in one go.
            var trimmed = requestedPath.TrimEnd('/');
            if (string.IsNullOrEmpty(trimmed))
                return new ErrorResponse(ErrorCodes.InvalidParams, "Folder path cannot be empty.");

            var lastSlash = trimmed.LastIndexOf('/');
            if (lastSlash < 0)
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"Folder path must contain a parent folder (e.g. 'Assets/MyFolder'). Got '{requestedPath}'.");

            var parent = trimmed.Substring(0, lastSlash);
            var child = trimmed.Substring(lastSlash + 1);

            if (!AssetDatabase.IsValidFolder(parent))
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"Parent folder does not exist: '{parent}'.");

            var fullPath = parent + "/" + child;
            if (AssetDatabase.IsValidFolder(fullPath))
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"Folder already exists: '{fullPath}'.");

            if (dryRun)
            {
                return new SuccessResponse(
                    $"[dry-run] Would create folder {fullPath}.",
                    new
                    {
                        dry_run = true,
                        would_create = fullPath,
                        type = "Folder",
                    });
            }

            var guid = AssetDatabase.CreateFolder(parent, child);
            if (string.IsNullOrEmpty(guid))
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"AssetDatabase.CreateFolder('{parent}', '{child}') returned empty GUID.");

            return new SuccessResponse(
                $"Created folder {fullPath}.",
                new
                {
                    path = fullPath,
                    guid,
                    type = "Folder",
                });
        }

        static object Move(ToolParams p)
        {
            var pathResult = p.GetRequired("path", "'path' (source) is required for move.");
            if (!pathResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, pathResult.ErrorMessage);

            var dstResult = p.GetRequired("dst", "'dst' (destination) is required for move.");
            if (!dstResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, dstResult.ErrorMessage);

            var from = pathResult.Value;
            var to = dstResult.Value;

            if (string.IsNullOrEmpty(AssetDatabase.AssetPathToGUID(from)))
                return new ErrorResponse(ErrorCodes.AssetNotFound,
                    $"Source asset not found: {from}");

            if (!string.IsNullOrEmpty(AssetDatabase.AssetPathToGUID(to)))
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"An asset already exists at destination: {to}");

            var dryRun = p.GetBool("dry_run");
            var guid = AssetDatabase.AssetPathToGUID(from);

            if (dryRun)
            {
                return new SuccessResponse(
                    $"[dry-run] Would move {from} -> {to}.",
                    new
                    {
                        dry_run = true,
                        from,
                        to,
                        guid,
                    });
            }

            // ValidateMoveAsset returns an empty string on success, a human
            // message on failure (e.g. "File not found", "Destination path
            // not within project"). Calling it up front gives agents a
            // precise error instead of the generic "MoveAsset returned false".
            var validation = AssetDatabase.ValidateMoveAsset(from, to);
            if (!string.IsNullOrEmpty(validation))
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"Cannot move: {validation}");

            var err = AssetDatabase.MoveAsset(from, to);
            if (!string.IsNullOrEmpty(err))
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"AssetDatabase.MoveAsset failed: {err}");

            return new SuccessResponse(
                $"Moved {from} -> {to}.",
                new
                {
                    from,
                    to,
                    guid, // GUID is preserved across move — references stay valid
                });
        }

        static object Delete(ToolParams p)
        {
            var pathResult = p.GetRequired("path", "'path' is required for delete.");
            if (!pathResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, pathResult.ErrorMessage);

            var path = pathResult.Value;
            if (string.IsNullOrEmpty(AssetDatabase.AssetPathToGUID(path)))
                return new ErrorResponse(ErrorCodes.AssetNotFound,
                    $"Asset not found: {path}");

            var permanent = p.GetBool("permanent");
            var dryRun = p.GetBool("dry_run");

            // Count direct references so the caller sees the blast radius
            // before committing to a permanent delete. Skip this on a dry-run
            // of MoveToTrash because the operation is recoverable anyway and
            // scanning can be slow on large projects.
            int? referencedBy = null;
            if (permanent)
            {
                var scanned = 0;
                var count = 0;
                foreach (var guid in AssetDatabase.FindAssets(""))
                {
                    scanned++;
                    var other = AssetDatabase.GUIDToAssetPath(guid);
                    if (string.IsNullOrEmpty(other) || other == path) continue;
                    var deps = AssetDatabase.GetDependencies(other, false);
                    if (Array.IndexOf(deps, path) >= 0) count++;
                }
                referencedBy = count;
            }

            if (dryRun)
            {
                return new SuccessResponse(
                    $"[dry-run] Would {(permanent ? "permanently delete" : "move to trash")} {path}.",
                    new
                    {
                        dry_run = true,
                        would_delete = path,
                        permanent,
                        referenced_by = referencedBy,
                    });
            }

            bool ok;
            try
            {
                ok = permanent
                    ? AssetDatabase.DeleteAsset(path)
                    : AssetDatabase.MoveAssetToTrash(path);
            }
            catch (Exception ex)
            {
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"AssetDatabase.{(permanent ? "DeleteAsset" : "MoveAssetToTrash")}('{path}') threw: {ex.Message}");
            }

            if (!ok)
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"AssetDatabase.{(permanent ? "DeleteAsset" : "MoveAssetToTrash")}('{path}') returned false.");

            return new SuccessResponse(
                permanent ? $"Deleted {path} (permanent)." : $"Moved {path} to trash.",
                new
                {
                    deleted = path,
                    permanent,
                    referenced_by = referencedBy,
                });
        }

        static object Label(ToolParams p)
        {
            var opResult = p.GetRequired("label_op", "'label_op' is required for label (add, remove, list, set, clear).");
            if (!opResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, opResult.ErrorMessage);

            var pathResult = p.GetRequired("path", "'path' is required for label.");
            if (!pathResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, pathResult.ErrorMessage);

            var path = pathResult.Value;
            if (string.IsNullOrEmpty(AssetDatabase.AssetPathToGUID(path)))
                return new ErrorResponse(ErrorCodes.AssetNotFound,
                    $"Asset not found: {path}");

            var obj = AssetDatabase.LoadMainAssetAtPath(path);
            if (obj == null)
                return new ErrorResponse(ErrorCodes.AssetNotFound,
                    $"Asset not loadable: {path}");

            var op = opResult.Value.ToLowerInvariant();
            var current = AssetDatabase.GetLabels(obj) ?? new string[0];
            var currentSet = new HashSet<string>(current, StringComparer.Ordinal);

            // list is a pure read — no need for a dry-run path.
            if (op == "list")
            {
                return new SuccessResponse(
                    $"{current.Length} label(s) on {path}.",
                    new { path, count = current.Length, labels = current });
            }

            var labelsArg = p.Get("labels") ?? string.Empty;
            var incoming = labelsArg
                .Split(',')
                .Select(s => s.Trim())
                .Where(s => s.Length > 0)
                .ToArray();

            string[] desired;
            string description;
            switch (op)
            {
                case "add":
                    if (incoming.Length == 0)
                        return new ErrorResponse(ErrorCodes.InvalidParams,
                            "label add requires at least one label.");
                    foreach (var l in incoming) currentSet.Add(l);
                    desired = currentSet.ToArray();
                    description = $"Add {string.Join(", ", incoming)} to {path}";
                    break;

                case "remove":
                    if (incoming.Length == 0)
                        return new ErrorResponse(ErrorCodes.InvalidParams,
                            "label remove requires at least one label.");
                    foreach (var l in incoming) currentSet.Remove(l);
                    desired = currentSet.ToArray();
                    description = $"Remove {string.Join(", ", incoming)} from {path}";
                    break;

                case "set":
                    // Replace whole set — explicit destructive op, agent
                    // should know exactly what they are committing.
                    desired = incoming;
                    description = $"Set labels on {path} to [{string.Join(", ", incoming)}]";
                    break;

                case "clear":
                    desired = new string[0];
                    description = $"Clear all labels on {path}";
                    break;

                default:
                    return new ErrorResponse(ErrorCodes.InvalidParams,
                        $"Unknown label op '{op}'. Available: add, remove, list, set, clear.");
            }

            if (p.GetBool("dry_run"))
            {
                return new SuccessResponse(
                    $"[dry-run] Would {description}.",
                    new
                    {
                        dry_run = true,
                        path,
                        op,
                        before = current,
                        after = desired,
                    });
            }

            AssetDatabase.SetLabels(obj, desired);
            AssetDatabase.SaveAssets();

            return new SuccessResponse(
                $"{description}.",
                new
                {
                    path,
                    op,
                    before = current,
                    after = desired,
                });
        }

        // --- Mutation helpers ---------------------------------------------

        struct ResolvedPath
        {
            public string path;
            public ErrorResponse error;
        }

        static ResolvedPath ResolveAssetFilePath(string requested, string fallbackName)
        {
            // If the path ends with '/' or resolves to an existing folder,
            // auto-append "{fallbackName}.asset". Otherwise treat the path
            // as an explicit filename.
            var result = new ResolvedPath();
            var p = requested;

            if (p.EndsWith("/", StringComparison.Ordinal))
            {
                var folder = p.TrimEnd('/');
                if (!AssetDatabase.IsValidFolder(folder))
                {
                    result.error = new ErrorResponse(ErrorCodes.InvalidParams,
                        $"Target folder does not exist: '{folder}'.");
                    return result;
                }
                result.path = folder + "/" + fallbackName + ".asset";
                return result;
            }

            if (AssetDatabase.IsValidFolder(p))
            {
                result.path = p + "/" + fallbackName + ".asset";
                return result;
            }

            // Explicit filename. Require parent folder exists so we fail
            // cleanly rather than letting CreateAsset throw.
            var parentIdx = p.LastIndexOf('/');
            if (parentIdx > 0)
            {
                var parent = p.Substring(0, parentIdx);
                if (!AssetDatabase.IsValidFolder(parent))
                {
                    result.error = new ErrorResponse(ErrorCodes.InvalidParams,
                        $"Parent folder does not exist: '{parent}'. Create it with `udit asset create --type Folder --path {parent}` first.");
                    return result;
                }
            }
            result.path = p;
            return result;
        }

        static Type ResolveScriptableObjectType(string name)
        {
            if (string.IsNullOrEmpty(name)) return null;

            // Try exact FullName match first. Project types should be
            // addressed by full name to disambiguate against UnityEngine.
            foreach (var asm in AppDomain.CurrentDomain.GetAssemblies())
            {
                var t = asm.GetType(name, throwOnError: false, ignoreCase: false);
                if (t != null && typeof(ScriptableObject).IsAssignableFrom(t) && !t.IsAbstract)
                    return t;
            }

            // Short-name fallback. This lets "GameConfig" resolve when only
            // one ScriptableObject with that name exists.
            Type best = null;
            foreach (var asm in AppDomain.CurrentDomain.GetAssemblies())
            {
                Type[] types;
                try { types = asm.GetTypes(); }
                catch (System.Reflection.ReflectionTypeLoadException) { continue; }

                foreach (var t in types)
                {
                    if (!typeof(ScriptableObject).IsAssignableFrom(t)) continue;
                    if (t.IsAbstract) continue;
                    if (!string.Equals(t.Name, name, StringComparison.OrdinalIgnoreCase)) continue;
                    if (best == null) best = t;
                }
            }
            return best;
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
                // Unity 2021.2+ prefers the Shader instance methods over
                // ShaderUtil.*. The enum moved from ShaderUtil.
                // ShaderPropertyType to UnityEngine.Rendering.
                // ShaderPropertyType and the Texture case was renamed from
                // TexEnv -> Texture.
                var count = shader.GetPropertyCount();
                for (int i = 0; i < count; i++)
                {
                    var propName = shader.GetPropertyName(i);
                    var propType = shader.GetPropertyType(i);
                    object value = null;
                    try
                    {
                        switch (propType)
                        {
                            case UnityEngine.Rendering.ShaderPropertyType.Color:   value = Col(mat.GetColor(propName)); break;
                            case UnityEngine.Rendering.ShaderPropertyType.Float:
                            case UnityEngine.Rendering.ShaderPropertyType.Range:   value = mat.GetFloat(propName); break;
                            case UnityEngine.Rendering.ShaderPropertyType.Vector:  value = V4(mat.GetVector(propName)); break;
                            case UnityEngine.Rendering.ShaderPropertyType.Texture:
                                var tex = mat.GetTexture(propName);
                                value = tex != null ? new { type = tex.GetType().Name, name = tex.name } : null;
                                break;
                            case UnityEngine.Rendering.ShaderPropertyType.Int:     value = mat.GetInteger(propName); break;
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
