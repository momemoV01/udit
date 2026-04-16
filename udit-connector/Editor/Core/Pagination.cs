using System;

namespace UditConnector
{
    /// <summary>
    /// Shared pagination utilities for tools that return paginated result sets.
    /// Used by ManageAsset (find, references) and ManageGameObject (find).
    /// </summary>
    public static class Pagination
    {
        /// <summary>
        /// Parse limit/offset from ToolParams with clamping.
        /// </summary>
        public static (int limit, int offset) Parse(ToolParams p, int defaultLimit, int maxLimit)
        {
            var limit = Clamp(p.GetInt("limit", defaultLimit) ?? defaultLimit, 1, maxLimit);
            var offset = Math.Max(0, p.GetInt("offset", 0) ?? 0);
            return (limit, offset);
        }

        /// <summary>
        /// Whether there are more results beyond the current page.
        /// </summary>
        public static bool HasMore(int offset, int returnedCount, int total)
        {
            return offset + returnedCount < total;
        }

        /// <summary>
        /// Clamp an integer to [lo, hi]. Shared utility replacing per-tool
        /// private Clamp methods.
        /// </summary>
        public static int Clamp(int v, int lo, int hi)
        {
            return v < lo ? lo : (v > hi ? hi : v);
        }
    }
}
