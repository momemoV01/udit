using System;
using System.Collections.Generic;
using UnityEditor;
using UnityEngine;

namespace UditConnector.Tools.Common
{
    /// <summary>
    /// Converts Unity <see cref="Component"/> instances into agent-friendly
    /// JSON-shaped objects by walking <see cref="SerializedObject"/>.
    ///
    /// Why SerializedObject and not reflection? Because SerializedObject is
    /// what the Inspector uses, so "visible" properties match what a human
    /// sees when they click the GameObject — the Unity-native surface.
    /// Reflection would also surface private fields and internal editor
    /// state that is not part of the authored asset.
    ///
    /// Transform is special-cased: its serialized form uses m_LocalPosition
    /// etc., but agents almost always want world-space values too, so we
    /// report both local and world coordinates directly rather than just
    /// what SerializedObject exposes.
    ///
    /// Arrays are clipped to MaxArrayElements to keep responses bounded on
    /// scenes with long lists. Agents that need the whole array can later
    /// call `component get go:... FieldName` (coming in a follow-up slice).
    /// </summary>
    public static class SerializedInspect
    {
        const int MaxArrayElements = 20;

        public static object ComponentToObject(Component c)
        {
            if (c == null)
            {
                // A null Component slot signals a missing script reference.
                // Surface it explicitly — agents use this to locate stale
                // prefabs without falling back to console error scraping.
                return new
                {
                    type = "<Missing Script>",
                    enabled = (bool?)null,
                    properties = new Dictionary<string, object>(),
                };
            }

            if (c is Transform t)
                return DescribeTransform(t);

            var props = WalkProperties(c);
            return new
            {
                type = c.GetType().Name,
                enabled = (c is Behaviour b) ? (bool?)b.enabled : null,
                properties = props,
            };
        }

        /// <summary>
        /// Dump any <see cref="UnityEngine.Object"/> (ScriptableObject, Material,
        /// TextAsset, etc.) into the same {type, properties} shape as
        /// <see cref="ComponentToObject"/>. No Behaviour-specific `enabled`
        /// field because non-Component assets do not carry one, and no
        /// Transform special case because this path is for asset data, not
        /// scene hierarchy state. Use ComponentToObject for live Components.
        /// </summary>
        public static object ObjectToJson(UnityEngine.Object obj)
        {
            if (obj == null)
            {
                return new
                {
                    type = "<Missing Asset>",
                    properties = new Dictionary<string, object>(),
                };
            }

            return new
            {
                type = obj.GetType().Name,
                properties = WalkProperties(obj),
            };
        }

        static Dictionary<string, object> WalkProperties(UnityEngine.Object target)
        {
            var props = new Dictionary<string, object>();
            try
            {
                using var so = new SerializedObject(target);
                var iter = so.GetIterator();
                bool enterChildren = true;
                while (iter.NextVisible(enterChildren))
                {
                    enterChildren = false;
                    if (iter.name == "m_Script") continue;
                    props[iter.name] = ReadProperty(iter);
                }
            }
            catch (Exception)
            {
                // Some built-in objects throw during iteration (e.g. certain
                // render-pipeline internals). Returning whatever we got so far
                // plus the type is more useful than propagating an exception
                // that would abort the whole inspect.
            }
            return props;
        }

        static object DescribeTransform(Transform t)
        {
            return new
            {
                type = t is RectTransform ? "RectTransform" : "Transform",
                enabled = (bool?)null,
                properties = new Dictionary<string, object>
                {
                    ["local_position"] = V3(t.localPosition),
                    ["local_rotation_euler"] = V3(t.localEulerAngles),
                    ["local_scale"] = V3(t.localScale),
                    ["position"] = V3(t.position),
                    ["rotation_euler"] = V3(t.eulerAngles),
                    ["sibling_index"] = t.GetSiblingIndex(),
                    ["child_count"] = t.childCount,
                },
            };
        }

        static object ReadProperty(SerializedProperty p)
        {
            switch (p.propertyType)
            {
                case SerializedPropertyType.Integer:       return p.intValue;
                case SerializedPropertyType.Boolean:       return p.boolValue;
                case SerializedPropertyType.Float:         return p.floatValue;
                case SerializedPropertyType.String:        return p.stringValue;
                case SerializedPropertyType.Color:         return Col(p.colorValue);
                case SerializedPropertyType.Vector2:       return V2(p.vector2Value);
                case SerializedPropertyType.Vector3:       return V3(p.vector3Value);
                case SerializedPropertyType.Vector4:       return V4(p.vector4Value);
                case SerializedPropertyType.Rect:          return new { x = p.rectValue.x, y = p.rectValue.y, width = p.rectValue.width, height = p.rectValue.height };
                case SerializedPropertyType.Quaternion:    return V4(p.quaternionValue);
                case SerializedPropertyType.Bounds:        return new { center = V3(p.boundsValue.center), extents = V3(p.boundsValue.extents) };
                case SerializedPropertyType.ArraySize:     return p.intValue;
                case SerializedPropertyType.LayerMask:     return p.intValue;
                case SerializedPropertyType.AnimationCurve: return DescribeAnimationCurve(p.animationCurveValue);
                case SerializedPropertyType.Gradient:       return "<Gradient>";
                case SerializedPropertyType.Character:     return p.intValue;
                case SerializedPropertyType.Vector2Int:    return new { x = p.vector2IntValue.x, y = p.vector2IntValue.y };
                case SerializedPropertyType.Vector3Int:    return new { x = p.vector3IntValue.x, y = p.vector3IntValue.y, z = p.vector3IntValue.z };
                case SerializedPropertyType.BoundsInt:     return new { position = new { x = p.boundsIntValue.position.x, y = p.boundsIntValue.position.y, z = p.boundsIntValue.position.z }, size = new { x = p.boundsIntValue.size.x, y = p.boundsIntValue.size.y, z = p.boundsIntValue.size.z } };
                case SerializedPropertyType.RectInt:       return new { x = p.rectIntValue.x, y = p.rectIntValue.y, width = p.rectIntValue.width, height = p.rectIntValue.height };

                case SerializedPropertyType.Enum:
                    // enumDisplayNames and enumValueIndex are only valid when
                    // the current integer is in-range; guard against broken
                    // enums (can happen after script changes) to avoid throws.
                    if (p.enumDisplayNames != null &&
                        p.enumValueIndex >= 0 &&
                        p.enumValueIndex < p.enumDisplayNames.Length)
                    {
                        return new
                        {
                            value = p.intValue,
                            name = p.enumDisplayNames[p.enumValueIndex],
                        };
                    }
                    return new { value = p.intValue, name = (string)null };

                case SerializedPropertyType.ObjectReference:
                    return DescribeObjectReference(p.objectReferenceValue);

                case SerializedPropertyType.ExposedReference:
                    return DescribeObjectReference(p.exposedReferenceValue);

                case SerializedPropertyType.ManagedReference:
                    return new { type = p.managedReferenceFullTypename, id = p.managedReferenceId };

                case SerializedPropertyType.Generic:
                    return p.isArray ? ReadArray(p) : ReadGeneric(p);

                default:
                    return null;
            }
        }

        static object ReadArray(SerializedProperty p)
        {
            var size = p.arraySize;
            var take = Math.Min(size, MaxArrayElements);
            var list = new List<object>(take);
            for (int i = 0; i < take; i++)
                list.Add(ReadProperty(p.GetArrayElementAtIndex(i)));

            return new
            {
                count = size,
                elements = list,
                truncated = size > MaxArrayElements,
            };
        }

        static object ReadGeneric(SerializedProperty p)
        {
            // Walk one level of nested fields for structs (Gradient, etc. handled
            // separately above). We duplicate the property to avoid mutating the
            // caller's iteration cursor.
            var copy = p.Copy();
            var end = copy.GetEndProperty();
            var props = new Dictionary<string, object>();
            if (!copy.NextVisible(true)) return props;
            while (!SerializedProperty.EqualContents(copy, end))
            {
                props[copy.name] = ReadProperty(copy);
                if (!copy.NextVisible(false)) break;
            }
            return props;
        }

        static object DescribeObjectReference(UnityEngine.Object o)
        {
            if (o == null) return null;
            var path = AssetDatabase.GetAssetPath(o);
            return new
            {
                type = o.GetType().Name,
                name = o.name,
                path = string.IsNullOrEmpty(path) ? null : path,
                guid = string.IsNullOrEmpty(path) ? null : AssetDatabase.AssetPathToGUID(path),
            };
        }

        /// <summary>
        /// Renders an AnimationCurve in the same JSON shape `component set
        /// &lt;field&gt; curve &lt;value&gt;` accepts, so `get | set` round-trips.
        /// Also used as the `from` field when `component set --dry-run`
        /// reports what the current curve is. Exposed at assembly scope
        /// (internal) so ManageComponent can reuse it.
        /// </summary>
        internal static object DescribeAnimationCurve(AnimationCurve curve)
        {
            if (curve == null) return null;
            var keys = curve.keys ?? new Keyframe[0];
            var keyObjs = new object[keys.Length];
            for (int i = 0; i < keys.Length; i++)
            {
                var k = keys[i];
                keyObjs[i] = new
                {
                    t = k.time,
                    v = k.value,
                    inT = k.inTangent,
                    outT = k.outTangent,
                };
            }
            return new
            {
                keys = keyObjs,
                preWrap = curve.preWrapMode.ToString(),
                postWrap = curve.postWrapMode.ToString(),
            };
        }

        static object V2(Vector2 v) => new { x = v.x, y = v.y };
        static object V3(Vector3 v) => new { x = v.x, y = v.y, z = v.z };
        static object V4(Vector4 v) => new { x = v.x, y = v.y, z = v.z, w = v.w };
        static object V4(Quaternion q) => new { x = q.x, y = q.y, z = q.z, w = q.w };
        static object Col(Color c) => new { r = c.r, g = c.g, b = c.b, a = c.a };
    }
}
