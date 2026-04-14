using System;
using System.Collections.Generic;
using System.Security.Cryptography;
using System.Text;
using UnityEditor;
using UnityEngine;

namespace UditConnector.Tools.Common
{
    /// <summary>
    /// Issues compact, stable identifiers for GameObjects in the form "go:xxxxxxxx".
    ///
    /// Why this exists:
    /// - <see cref="UnityEngine.Object.GetInstanceID"/> is session-scoped — restarting
    ///   the editor regenerates it, so agents cannot chain commands across sessions
    ///   with the same ID.
    /// - <see cref="UnityEditor.GlobalObjectId"/> is persistent but ~80 characters and
    ///   verbose ("GlobalObjectId_V1-2-...-12345-0"), which is unwieldy in CLI
    ///   arguments and JSON responses.
    /// - Hashing GlobalObjectId to 8 hex chars gives agent-friendly IDs that are
    ///   deterministic across restarts (same object → same hash) and compact enough
    ///   to pass between commands without visual noise.
    ///
    /// Collision probability: ~N² / (2·2³²). For 10,000 objects in a scene that is
    /// ~1 in 86 — rare but non-zero. On collision the hash length is extended by
    /// two chars at a time up to 16 (64 bits), which is effectively collision-free
    /// for any realistic workload.
    ///
    /// Lifetime: the registry is in-memory and is torn down on every domain reload
    /// along with all other static state. That is fine — GlobalObjectId issuance is
    /// deterministic, so an agent that re-runs `go find` gets identical IDs back.
    /// </summary>
    public static class StableIdRegistry
    {
        public const string GoPrefix = "go:";
        const int BaseHashChars = 8;
        const int MaxHashChars = 16;

        // id → GlobalObjectId string. We intentionally store the canonical string
        // instead of a live Object reference so domain reloads do not leave us with
        // dangling managed pointers. Resolution parses the string back on demand.
        static readonly Dictionary<string, string> s_IdToGoid = new();

        // Reverse lookup — ensures idempotent ID issuance for the same object.
        static readonly Dictionary<string, string> s_GoidToId = new();

        /// <summary>How many IDs are currently mapped. Intended for diagnostics and tests.</summary>
        public static int Count => s_IdToGoid.Count;

        /// <summary>Cheap format check — does <paramref name="s"/> look like an ID issued by this registry?</summary>
        public static bool IsStableId(string s) =>
            !string.IsNullOrEmpty(s) && s.StartsWith(GoPrefix, StringComparison.Ordinal);

        /// <summary>
        /// Issue or reuse a stable ID for <paramref name="go"/>. Returns null for a
        /// null input so callers can forward untrusted references without null-checking.
        /// </summary>
        public static string ToStableId(GameObject go)
        {
            if (go == null) return null;

            var goid = GlobalObjectId.GetGlobalObjectIdSlow(go);
            var goidStr = goid.ToString();

            if (s_GoidToId.TryGetValue(goidStr, out var existing))
                return existing;

            var id = IssueNewId(goidStr);
            s_IdToGoid[id] = goidStr;
            s_GoidToId[goidStr] = id;
            return id;
        }

        /// <summary>
        /// Resolve a stable ID back to a live GameObject. Returns false if the ID is
        /// unknown to this session (no prior ToStableId call), malformed, or points
        /// to an object that no longer exists (destroyed, unloaded scene, etc.).
        /// </summary>
        public static bool TryResolve(string stableId, out GameObject go)
        {
            go = null;
            if (!IsStableId(stableId)) return false;
            if (!s_IdToGoid.TryGetValue(stableId, out var goidStr)) return false;
            if (!GlobalObjectId.TryParse(goidStr, out var goid)) return false;

            var obj = GlobalObjectId.GlobalObjectIdentifierToObjectSlow(goid);
            go = obj as GameObject;
            return go != null;
        }

        /// <summary>Drop all mappings. Tests use this; production code relies on
        /// Unity's domain-reload tear-down instead.</summary>
        public static void Clear()
        {
            s_IdToGoid.Clear();
            s_GoidToId.Clear();
        }

        static string IssueNewId(string goidStr)
        {
            for (int chars = BaseHashChars; chars <= MaxHashChars; chars += 2)
            {
                var id = GoPrefix + Hash(goidStr, chars);
                if (!s_IdToGoid.ContainsKey(id))
                    return id;
            }

            // Pathologically unlikely — 64 bits is enough for >4 billion objects
            // before the expected-collision count crosses 1. If we ever hit this,
            // fall back to the full 40-char SHA1 and log so we notice.
            Debug.LogWarning(
                $"[UditConnector] Stable ID collision exhausted {MaxHashChars} hex chars for " +
                $"{goidStr}. Falling back to full hash.");
            return GoPrefix + Hash(goidStr, 40);
        }

        static string Hash(string input, int chars)
        {
            if (chars <= 0 || chars > 40)
                throw new ArgumentOutOfRangeException(nameof(chars), "Hash length must be 1..40 (SHA1 is 20 bytes).");

            using var sha = SHA1.Create();
            var bytes = sha.ComputeHash(Encoding.UTF8.GetBytes(input));
            var sb = new StringBuilder(chars);
            int bytesNeeded = (chars + 1) / 2;
            for (int i = 0; i < bytesNeeded && i < bytes.Length; i++)
                sb.Append(bytes[i].ToString("x2"));
            if (sb.Length > chars) sb.Length = chars;
            return sb.ToString();
        }
    }
}
