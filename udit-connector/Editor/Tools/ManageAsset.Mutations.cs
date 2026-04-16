using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;
using UnityEditor;
using UnityEngine;

namespace UditConnector.Tools
{
    // Mutation actions for ManageAsset: create, move, delete, label.
    //
    // Note on Undo: AssetDatabase operations (CreateAsset, MoveAsset,
    // DeleteAsset, SetLabels) do NOT participate in Unity's scene Undo
    // system. They write straight to disk and to the AssetDatabase index.
    // Ctrl+Z in the Editor will not reverse these — the safeguards are
    // `--dry-run` for preview, and `delete` defaulting to MoveAssetToTrash
    // so recovery is possible from the OS trash. This is documented in
    // the user-facing help and README.
    public static partial class ManageAsset
    {
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
    }
}
