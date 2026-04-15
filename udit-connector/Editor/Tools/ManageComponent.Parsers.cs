using System;
using System.Collections.Generic;
using System.Globalization;
using System.Linq;
using System.Reflection;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;
using UditConnector.Tools.Common;
using UnityEditor;
using UnityEngine;

namespace UditConnector.Tools
{
    // Partial of ManageComponent — value parsers for every type the
    // `component set` action accepts, plus the type-resolution helpers
    // for ManagedReference / ObjectReference. Split out of
    // ManageComponent.cs in Sprint 4 Track C. Same partial class so all
    // members stay private and visible to ManageComponent.cs.
    public static partial class ManageComponent
    {
        static bool TryParseBool(string s, out bool b)
        {
            switch ((s ?? string.Empty).Trim().ToLowerInvariant())
            {
                case "true":
                case "1":
                case "yes":
                case "on":  b = true;  return true;
                case "false":
                case "0":
                case "no":
                case "off": b = false; return true;
                default:    b = false; return false;
            }
        }

        static bool TryParseVector2(string s, out Vector2 v)
        {
            v = default;
            if (string.IsNullOrEmpty(s)) return false;
            var parts = s.Split(',');
            if (parts.Length != 2) return false;
            if (!float.TryParse(parts[0].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var x)) return false;
            if (!float.TryParse(parts[1].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var y)) return false;
            v = new Vector2(x, y); return true;
        }

        static bool TryParseVector4(string s, out Vector4 v)
        {
            v = default;
            if (string.IsNullOrEmpty(s)) return false;
            var parts = s.Split(',');
            if (parts.Length != 4) return false;
            if (!float.TryParse(parts[0].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var x)) return false;
            if (!float.TryParse(parts[1].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var y)) return false;
            if (!float.TryParse(parts[2].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var z)) return false;
            if (!float.TryParse(parts[3].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var w)) return false;
            v = new Vector4(x, y, z, w); return true;
        }

        static bool TryParseColor(string s, out Color c)
        {
            c = default;
            if (string.IsNullOrEmpty(s)) return false;
            s = s.Trim();

            // Hex form '#RRGGBB' or '#RRGGBBAA'.
            if (s.StartsWith("#", StringComparison.Ordinal))
                return ColorUtility.TryParseHtmlString(s, out c);

            // Comma-separated 'r,g,b' or 'r,g,b,a' — floats in 0..1.
            var parts = s.Split(',');
            if (parts.Length < 3 || parts.Length > 4) return false;
            if (!float.TryParse(parts[0].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var r)) return false;
            if (!float.TryParse(parts[1].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var g)) return false;
            if (!float.TryParse(parts[2].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var b)) return false;
            var a = 1f;
            if (parts.Length == 4 && !float.TryParse(parts[3].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out a)) return false;
            c = new Color(r, g, b, a); return true;
        }

        static bool TryParseEnum(SerializedProperty prop, string value, out int enumValueIndex)
        {
            // Accept either an integer value-index or one of the enum's
            // display names (case-insensitive). We normalize to the
            // value-index because prop.enumValueIndex is the only safe way
            // to assign back (works for both dense and sparse enums).
            if (int.TryParse(value, NumberStyles.Integer, CultureInfo.InvariantCulture, out enumValueIndex))
            {
                return prop.enumDisplayNames != null
                    && enumValueIndex >= 0
                    && enumValueIndex < prop.enumDisplayNames.Length;
            }

            if (prop.enumDisplayNames != null)
            {
                for (int i = 0; i < prop.enumDisplayNames.Length; i++)
                {
                    if (string.Equals(prop.enumDisplayNames[i], value, StringComparison.OrdinalIgnoreCase))
                    {
                        enumValueIndex = i;
                        return true;
                    }
                }
            }
            enumValueIndex = 0;
            return false;
        }

        /// <summary>
        /// Parse the JSON value string into an AnimationCurve ready for
        /// `prop.animationCurveValue` assignment. Shape:
        ///   { "keys": [ { "t":..,"v":..,"inT":..,"outT":.. }, ... ],
        ///     "preWrap": "ClampForever", "postWrap": "ClampForever" }
        /// Defaults: inT/outT=0 (linear tangents); preWrap/postWrap =
        /// ClampForever (Unity's runtime default).
        /// </summary>
        static bool TryParseAnimationCurve(string json, out AnimationCurve curve, out string error)
        {
            curve = null;
            error = null;
            if (string.IsNullOrWhiteSpace(json))
            {
                error = "AnimationCurve: empty value. Expected JSON { \"keys\": [...] }.";
                return false;
            }
            JObject obj;
            try { obj = JObject.Parse(json); }
            catch (System.Exception ex) { error = $"AnimationCurve: invalid JSON — {ex.Message}"; return false; }

            var keysTok = obj["keys"] as JArray;
            var keyCount = keysTok?.Count ?? 0;
            var keys = new Keyframe[keyCount];
            for (int i = 0; i < keyCount; i++)
            {
                var k = keysTok[i];
                if (k is not JObject)
                {
                    error = $"AnimationCurve: keys[{i}] must be a JSON object like {{\"t\":0,\"v\":0}}.";
                    return false;
                }
                float ParseFloat(string field, float defaultValue)
                {
                    var tok = k[field];
                    if (tok == null) return defaultValue;
                    try { return tok.Value<float>(); }
                    catch { return defaultValue; }
                }
                keys[i] = new Keyframe(
                    ParseFloat("t", 0f),
                    ParseFloat("v", 0f),
                    ParseFloat("inT", 0f),
                    ParseFloat("outT", 0f));
            }
            curve = new AnimationCurve(keys);

            if (obj["preWrap"] != null)
            {
                if (!TryParseWrapMode(obj["preWrap"].ToString(), out var pm, out error)) return false;
                curve.preWrapMode = pm;
            }
            else curve.preWrapMode = WrapMode.ClampForever;

            if (obj["postWrap"] != null)
            {
                if (!TryParseWrapMode(obj["postWrap"].ToString(), out var qm, out error)) return false;
                curve.postWrapMode = qm;
            }
            else curve.postWrapMode = WrapMode.ClampForever;

            return true;
        }

        static bool TryParseWrapMode(string s, out WrapMode mode, out string error)
        {
            mode = WrapMode.ClampForever;
            error = null;
            if (System.Enum.TryParse<WrapMode>(s, true, out var parsed) && System.Enum.IsDefined(typeof(WrapMode), parsed))
            {
                mode = parsed;
                return true;
            }
            error = $"Unknown WrapMode '{s}'. Accepted: Default, Once, Loop, PingPong, ClampForever.";
            return false;
        }

        /// <summary>
        /// Parse a Gradient JSON value. Shape:
        ///   { "colorKeys":[{"t":0,"color":"#RRGGBB[AA]"}],
        ///     "alphaKeys":[{"t":0,"a":1}],
        ///     "mode":"Blend" }
        /// Unity requires 2–8 keys per array; violations produce a clear error.
        /// </summary>
        static bool TryParseGradient(string json, out Gradient gradient, out string error)
        {
            gradient = null;
            error = null;
            if (string.IsNullOrWhiteSpace(json))
            {
                error = "Gradient: empty value. Expected JSON { \"colorKeys\": [...], \"alphaKeys\": [...] }.";
                return false;
            }
            JObject obj;
            try { obj = JObject.Parse(json); }
            catch (System.Exception ex) { error = $"Gradient: invalid JSON — {ex.Message}"; return false; }

            var ckTok = obj["colorKeys"] as JArray;
            var akTok = obj["alphaKeys"] as JArray;
            if (ckTok == null || akTok == null)
            {
                error = "Gradient: both colorKeys and alphaKeys arrays are required.";
                return false;
            }
            if (ckTok.Count < 2 || ckTok.Count > 8)
            {
                error = $"Gradient: colorKeys count must be 2..8, got {ckTok.Count}.";
                return false;
            }
            if (akTok.Count < 2 || akTok.Count > 8)
            {
                error = $"Gradient: alphaKeys count must be 2..8, got {akTok.Count}.";
                return false;
            }

            var colorKeys = new GradientColorKey[ckTok.Count];
            for (int i = 0; i < ckTok.Count; i++)
            {
                var k = ckTok[i];
                if (k is not JObject)
                {
                    error = $"Gradient: colorKeys[{i}] must be a JSON object like {{\"t\":0,\"color\":\"#000000\"}}.";
                    return false;
                }
                float t = k["t"] != null ? k["t"].Value<float>() : 0f;
                var colorRaw = k["color"]?.ToString() ?? "#FFFFFF";
                if (!ColorUtility.TryParseHtmlString(colorRaw, out var col))
                {
                    error = $"Gradient: colorKeys[{i}].color '{colorRaw}' is not a valid hex or named color.";
                    return false;
                }
                colorKeys[i] = new GradientColorKey(col, t);
            }

            var alphaKeys = new GradientAlphaKey[akTok.Count];
            for (int i = 0; i < akTok.Count; i++)
            {
                var k = akTok[i];
                if (k is not JObject)
                {
                    error = $"Gradient: alphaKeys[{i}] must be a JSON object like {{\"t\":0,\"a\":1}}.";
                    return false;
                }
                float t = k["t"] != null ? k["t"].Value<float>() : 0f;
                float a = k["a"] != null ? k["a"].Value<float>() : 1f;
                alphaKeys[i] = new GradientAlphaKey(a, t);
            }

            gradient = new Gradient();
            gradient.SetKeys(colorKeys, alphaKeys);

            if (obj["mode"] != null)
            {
                if (!TryParseGradientMode(obj["mode"].ToString(), out var gm, out error)) return false;
                gradient.mode = gm;
            }
            // colorSpace deferred — Unity's default (Gamma) is correct for most cases.

            return true;
        }

        static bool TryParseGradientMode(string s, out GradientMode mode, out string error)
        {
            mode = GradientMode.Blend;
            error = null;
            if (System.Enum.TryParse<GradientMode>(s, true, out var parsed) && System.Enum.IsDefined(typeof(GradientMode), parsed))
            {
                mode = parsed;
                return true;
            }
            error = $"Unknown GradientMode '{s}'. Accepted: Blend, Fixed, PerceptualBlend.";
            return false;
        }

        /// <summary>
        /// Parse a ManagedReference value. Shape:
        ///   {"$type":"Namespace.Concrete, Assembly-CSharp", ...fields}
        ///   {"$type":"Namespace.Concrete", ...}      ← short-name fallback
        ///   null | "null" | "none" | ""              ← clear
        ///
        /// Returns two output signals:
        ///   - clear=true → caller writes prop.managedReferenceValue = null.
        ///   - clear=false + instance non-null → caller writes that instance.
        /// Both cases return true from the function (parse succeeded).
        /// </summary>
        static bool TryParseManagedReference(SerializedProperty prop, string value,
            out bool clear, out object instance, out string error)
        {
            clear = false;
            instance = null;
            error = null;

            if (string.IsNullOrEmpty(value) ||
                string.Equals(value, "null", System.StringComparison.OrdinalIgnoreCase) ||
                string.Equals(value, "none", System.StringComparison.OrdinalIgnoreCase))
            {
                clear = true;
                return true;
            }

            // Parse the outer envelope with Newtonsoft so we can extract $type
            // and strip it from the body. The residual body will be passed to
            // JsonUtility.FromJsonOverwrite — which uses Unity's native
            // [SerializeField] rules and matches how the instance would be
            // serialized to disk.
            JObject envelope;
            try { envelope = JObject.Parse(value); }
            catch (System.Exception ex) { error = $"ManagedReference: invalid JSON — {ex.Message}"; return false; }

            var typeTok = envelope["$type"];
            if (typeTok == null || typeTok.Type != JTokenType.String)
            {
                error = "ManagedReference: JSON must contain a `$type` string (AQN like 'MyNs.Foo, Assembly-CSharp' or a short 'MyNs.Foo').";
                return false;
            }

            // Field base-type. Unity's managedReferenceFieldTypename is
            // space-separated "Assembly TypeFullName" (known quirk, NOT AQN).
            var baseName = prop.managedReferenceFieldTypename;
            System.Type baseType = null;
            if (!string.IsNullOrEmpty(baseName))
            {
                var parts = baseName.Split(' ');
                if (parts.Length >= 2)
                {
                    baseType = System.Type.GetType($"{parts[1]}, {parts[0]}", throwOnError: false);
                }
            }

            if (!TryResolveManagedType(typeTok.ToString(), baseType, out var concreteType, out var typeError))
            { error = typeError; return false; }

            // Construct without invoking any ctor — lets agents target types
            // that don't have a parameterless constructor (Unity's
            // serialization ignores ctors anyway).
            object newInstance;
            try { newInstance = System.Runtime.Serialization.FormatterServices.GetUninitializedObject(concreteType); }
            catch (System.Exception ex)
            {
                error = $"ManagedReference: cannot instantiate {concreteType.FullName}: {ex.Message}";
                return false;
            }

            // Strip $type and hand the residual JSON to JsonUtility. The only
            // fields that matter are Unity's [SerializeField] + public-field
            // set; JsonUtility handles the rest identically to Unity's own
            // disk-read path. Nested [SerializeReference] is a known
            // limitation — won't recurse.
            envelope.Remove("$type");
            var fieldsJson = envelope.ToString();
            try { JsonUtility.FromJsonOverwrite(fieldsJson, newInstance); }
            catch (System.Exception ex)
            {
                error = $"ManagedReference: FromJsonOverwrite failed for {concreteType.FullName}: {ex.Message}";
                return false;
            }

            instance = newInstance;
            return true;
        }

        /// <summary>
        /// Resolve a ManagedReference `$type` string into a runtime Type.
        /// Algorithm (plan D2):
        ///   1. Try Type.GetType(value) — handles AQN including generics.
        ///   2. On failure + value looks like AQN (has ", "), try the
        ///      TypeCache to catch [MovedFrom] rewrites Unity applies.
        ///   3. Bare short name → TypeCache.GetTypesDerivedFrom(baseType)
        ///      with FullName match.
        /// Ambiguity for short names → list candidates in the error.
        /// </summary>
        static bool TryResolveManagedType(string typeName, System.Type baseType,
            out System.Type type, out string error)
        {
            type = null;
            error = null;
            if (string.IsNullOrWhiteSpace(typeName))
            {
                error = "ManagedReference: $type is empty.";
                return false;
            }

            // Step 1: direct Type.GetType covers AQN + generics.
            var direct = System.Type.GetType(typeName, throwOnError: false);
            if (direct != null)
            {
                if (baseType != null && !baseType.IsAssignableFrom(direct))
                {
                    error = $"ManagedReference: type {direct.FullName} is not assignable to field type {baseType.FullName}.";
                    return false;
                }
                type = direct;
                return true;
            }

            // Step 2+3: scan TypeCache for FullName matches.
            var candidates = new List<System.Type>();
            if (baseType != null)
            {
                foreach (var t in UnityEditor.TypeCache.GetTypesDerivedFrom(baseType))
                {
                    if (t.IsAbstract || t.IsGenericTypeDefinition) continue;
                    if (t.FullName == typeName || t.AssemblyQualifiedName == typeName) candidates.Add(t);
                }
            }
            else
            {
                foreach (var asm in System.AppDomain.CurrentDomain.GetAssemblies())
                {
                    try
                    {
                        var t = asm.GetType(typeName, throwOnError: false);
                        if (t != null && !t.IsAbstract && !t.IsGenericTypeDefinition) candidates.Add(t);
                    }
                    catch { /* ignore asm load quirks */ }
                }
            }

            if (candidates.Count == 1)
            {
                type = candidates[0];
                return true;
            }
            if (candidates.Count == 0)
            {
                error = baseType != null
                    ? $"ManagedReference: no type named '{typeName}' assignable to {baseType.FullName} in loaded assemblies."
                    : $"ManagedReference: no type named '{typeName}' in loaded assemblies.";
                return false;
            }
            var list = string.Join(" | ", candidates.Select(c => c.AssemblyQualifiedName));
            error = $"ManagedReference: '{typeName}' is ambiguous — {candidates.Count} matches. Use a fully-qualified name. Candidates: {list}";
            return false;
        }

        static bool TryResolveObjectReference(SerializedProperty prop, string value, out UnityEngine.Object obj, out string error)
        {
            obj = null;
            error = null;

            // Clearing the reference: treat "null", "none", and "" as clear.
            // We accept three spellings because agents occasionally pick the
            // one that matches their vocabulary (JSON null, Unity "None").
            if (string.IsNullOrEmpty(value) ||
                string.Equals(value, "null", StringComparison.OrdinalIgnoreCase) ||
                string.Equals(value, "none", StringComparison.OrdinalIgnoreCase))
            {
                return true;
            }

            // Scene-object references (go:XXXXXXXX). Historically rejected
            // (v0.4.1 conservative cut) on the claim that scene refs need a
            // distinct SceneObjectReference payload. Modern Unity accepts
            // `prop.objectReferenceValue = sceneGameObject` directly when
            // the host and target share a scene — supported in v0.9.0
            // with cross-scene + persistent-host guards matching Inspector
            // default behavior (EditorSceneManager.preventCrossSceneReferences).
            if (value.StartsWith("go:", StringComparison.Ordinal))
            {
                if (!StableIdRegistry.TryResolve(value, out var sceneGo))
                {
                    error = $"GameObject not found for stable ID {value}. Run `udit go find` / `udit scene tree` to get a current id.";
                    return false;
                }

                var host = prop.serializedObject.targetObject;

                // Prefab assets, ScriptableObject assets, and in-memory
                // prefab-edit-mode hosts cannot legally hold a scene
                // reference — Unity will strip the write on reload. Reject
                // loudly rather than writing dead data.
                if (EditorUtility.IsPersistent(host))
                {
                    error = $"Cannot assign scene GameObject {value} to a persistent (prefab / asset) host. " +
                            $"Scene refs only make sense on scene-resident hosts.";
                    return false;
                }

                // Expected type extraction. `prop.type` returns the string
                // form used throughout udit already — plan's
                // `objectReferenceTypeString` suggestion turned out to not
                // be a real property on SerializedProperty; stick with the
                // working `prop.type` that the asset path above also uses.
                var sceneExpectedTypeName = StripPPtrWrapper(prop.type);
                var sceneExpectedType = ResolveUnityObjectType(sceneExpectedTypeName);

                if (sceneExpectedType == typeof(GameObject) || sceneExpectedType == null)
                {
                    // GameObject field — same-scene check (host-as-Component
                    // gives us its scene; ScriptableObject hosts already
                    // rejected above via IsPersistent).
                    if (host is Component hostComp && hostComp.gameObject.scene != sceneGo.scene)
                    {
                        error = $"Cross-scene reference rejected: host scene '{hostComp.gameObject.scene.name}' vs target scene '{sceneGo.scene.name}'.";
                        return false;
                    }
                    obj = sceneGo;
                    return true;
                }

                if (typeof(Component).IsAssignableFrom(sceneExpectedType))
                {
                    // Component field — auto-extract the component. First wins,
                    // matching the sub-asset auto-pick above (LoadAllAssetsAtPath
                    // + first-assignable). Multi-component GOs (e.g. two Cameras)
                    // resolve to GetComponent's first return.
                    if (host is Component hostComp2 && hostComp2.gameObject.scene != sceneGo.scene)
                    {
                        error = $"Cross-scene reference rejected: host scene '{hostComp2.gameObject.scene.name}' vs target scene '{sceneGo.scene.name}'.";
                        return false;
                    }
                    var comp = sceneGo.GetComponent(sceneExpectedType);
                    if (comp == null)
                    {
                        var available = string.Join(", ", sceneGo.GetComponents<Component>().Select(c => c == null ? "<missing>" : c.GetType().Name));
                        error = $"GameObject {value} has no {sceneExpectedTypeName} component. Available: {available}.";
                        return false;
                    }
                    obj = comp;
                    return true;
                }

                error = $"Field expects {sceneExpectedTypeName} but scene refs can only assign GameObject or Component-derived types.";
                return false;
            }

            // Otherwise expect an asset path.
            if (!(value.StartsWith("Assets/", StringComparison.Ordinal) ||
                  value.StartsWith("Packages/", StringComparison.Ordinal)))
            {
                error = $"ObjectReference value must be an asset path (Assets/... or Packages/...), " +
                        $"or 'null'/'none'/'' to clear. Got '{value}'.";
                return false;
            }

            if (string.IsNullOrEmpty(AssetDatabase.AssetPathToGUID(value)))
            {
                // Reuse the registry's asset-not-found signal. We raise it
                // inline rather than letting the caller emit UCI-011 because
                // "asset at path X does not exist" and "asset exists but
                // wrong type" are genuinely different problems for an agent.
                error = $"Asset not found: {value}";
                return false;
            }

            // Resolve the expected type from the field. SerializedProperty.type
            // comes back as "PPtr<$Sprite>" for ObjectReference fields; strip
            // the wrapper to get the bare type name.
            var expectedTypeName = StripPPtrWrapper(prop.type);
            var expectedType = ResolveUnityObjectType(expectedTypeName);

            // LoadAllAssetsAtPath returns [main, sub...]. For a texture with
            // a Sprite sub-asset that is exactly what agents expect — they
            // pass the .png path and the Sprite gets assigned. We pick the
            // FIRST asset assignable to the expected type so "plain" asset
            // types (ScriptableObject, AudioClip, etc.) still work via their
            // main asset.
            var candidates = AssetDatabase.LoadAllAssetsAtPath(value);
            foreach (var candidate in candidates)
            {
                if (candidate == null) continue;
                if (expectedType == null || expectedType.IsAssignableFrom(candidate.GetType()))
                {
                    obj = candidate;
                    return true;
                }
            }

            var found = new List<string>();
            foreach (var c in candidates)
                if (c != null) found.Add(c.GetType().Name);

            error = found.Count == 0
                ? $"Asset at '{value}' contains no assignable objects."
                : $"Asset at '{value}' has no {expectedTypeName} (found: {string.Join(", ", found)}).";
            return false;
        }

        static string StripPPtrWrapper(string s)
        {
            // Unity's objectReferenceTypeString comes back in the form
            // "PPtr<$Sprite>" for most Editor surfaces. Some older paths use
            // "PPtr<Sprite>" without the $. Handle both.
            if (string.IsNullOrEmpty(s)) return s;
            const string prefix = "PPtr<";
            const string suffix = ">";
            if (!s.StartsWith(prefix, StringComparison.Ordinal) || !s.EndsWith(suffix, StringComparison.Ordinal))
                return s;
            var inner = s.Substring(prefix.Length, s.Length - prefix.Length - suffix.Length);
            return inner.StartsWith("$", StringComparison.Ordinal) ? inner.Substring(1) : inner;
        }

        static Type ResolveUnityObjectType(string shortName)
        {
            if (string.IsNullOrEmpty(shortName)) return null;

            // Try UnityEngine.* first (by far the most common — Sprite,
            // Texture, AudioClip, etc.). Fall back to any assembly so
            // project-local ScriptableObject types resolve too.
            foreach (var asm in AppDomain.CurrentDomain.GetAssemblies())
            {
                var t = asm.GetType("UnityEngine." + shortName, throwOnError: false);
                if (t != null && typeof(UnityEngine.Object).IsAssignableFrom(t))
                    return t;
            }
            foreach (var asm in AppDomain.CurrentDomain.GetAssemblies())
            {
                Type[] types;
                try { types = asm.GetTypes(); }
                catch (System.Reflection.ReflectionTypeLoadException) { continue; }
                foreach (var t in types)
                {
                    if (!typeof(UnityEngine.Object).IsAssignableFrom(t)) continue;
                    if (t.Name == shortName || t.FullName == shortName) return t;
                }
            }
            return null;
        }
    }
}
