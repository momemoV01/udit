using System.Globalization;
using Newtonsoft.Json.Linq;
using UnityEngine;

namespace UditConnector
{
    public static class ParamCoercion
    {
        public static bool CoerceBool(JToken token, bool defaultValue)
        {
            return CoerceBoolNullable(token) ?? defaultValue;
        }

        public static bool? CoerceBoolNullable(JToken token)
        {
            if (token == null || token.Type == JTokenType.Null)
                return null;
            try
            {
                if (token.Type == JTokenType.Boolean)
                    return token.Value<bool>();
                var s = token.ToString().Trim().ToLowerInvariant();
                if (s.Length == 0) return null;
                if (bool.TryParse(s, out var b)) return b;
                if (s == "1" || s == "yes" || s == "on") return true;
                if (s == "0" || s == "no" || s == "off") return false;
            }
            catch { }
            return null;
        }

        /// <summary>
        /// Parse a "x,y,z" string into a Vector3 using invariant-culture
        /// floats. Whitespace around each component is trimmed. Any other
        /// shape (empty, wrong arity, non-numeric token) returns false and
        /// leaves the out parameter at default.
        ///
        /// Shared by ManageGameObject / ManagePrefab / ManageComponent so
        /// the --pos argument parses identically across tools. Drift is
        /// caught by Vector3ParsingTests.
        /// </summary>
        public static bool TryParseVector3(string s, out Vector3 v)
        {
            v = default;
            if (string.IsNullOrEmpty(s)) return false;
            var parts = s.Split(',');
            if (parts.Length != 3) return false;
            if (!float.TryParse(parts[0].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var x)) return false;
            if (!float.TryParse(parts[1].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var y)) return false;
            if (!float.TryParse(parts[2].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var z)) return false;
            v = new Vector3(x, y, z);
            return true;
        }
    }
}
