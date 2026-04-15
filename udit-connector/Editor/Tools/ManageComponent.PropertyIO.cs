using System.Collections.Generic;
using System.Globalization;
using UditConnector.Tools.Common;
using UnityEditor;
using UnityEngine;

namespace UditConnector.Tools
{
    // Partial of ManageComponent — value parsing / writing / reading
    // helpers for SerializedProperty surfaces. Split out of
    // ManageComponent.cs in Sprint 4 Track C to keep the action-handler
    // file readable. Same partial class, same private access.
    public static partial class ManageComponent
    {
        static bool TryParseValueForProperty(SerializedProperty prop, string value, out object oldJsonValue, out string error)
        {
            error = null;
            oldJsonValue = null;
            switch (prop.propertyType)
            {
                case SerializedPropertyType.Integer:
                case SerializedPropertyType.LayerMask:
                case SerializedPropertyType.ArraySize:
                case SerializedPropertyType.Character:
                    oldJsonValue = prop.intValue;
                    if (!int.TryParse(value, NumberStyles.Integer, CultureInfo.InvariantCulture, out _))
                    { error = $"Field is Integer, got '{value}'."; return false; }
                    return true;

                case SerializedPropertyType.Boolean:
                    oldJsonValue = prop.boolValue;
                    if (!TryParseBool(value, out _))
                    { error = $"Field is Boolean, got '{value}'. Use true/false/1/0."; return false; }
                    return true;

                case SerializedPropertyType.Float:
                    oldJsonValue = prop.floatValue;
                    if (!float.TryParse(value, NumberStyles.Float, CultureInfo.InvariantCulture, out _))
                    { error = $"Field is Float, got '{value}'."; return false; }
                    return true;

                case SerializedPropertyType.String:
                    oldJsonValue = prop.stringValue;
                    return true;

                case SerializedPropertyType.Vector2:
                    oldJsonValue = new { x = prop.vector2Value.x, y = prop.vector2Value.y };
                    if (!TryParseVector2(value, out _))
                    { error = $"Field is Vector2, expected 'x,y', got '{value}'."; return false; }
                    return true;

                case SerializedPropertyType.Vector3:
                    oldJsonValue = new { x = prop.vector3Value.x, y = prop.vector3Value.y, z = prop.vector3Value.z };
                    if (!ParamCoercion.TryParseVector3(value, out _))
                    { error = $"Field is Vector3, expected 'x,y,z', got '{value}'."; return false; }
                    return true;

                case SerializedPropertyType.Vector4:
                case SerializedPropertyType.Quaternion:
                    oldJsonValue = new { x = prop.vector4Value.x, y = prop.vector4Value.y, z = prop.vector4Value.z, w = prop.vector4Value.w };
                    if (!TryParseVector4(value, out _))
                    { error = $"Field is Vector4/Quaternion, expected 'x,y,z,w', got '{value}'."; return false; }
                    return true;

                case SerializedPropertyType.Color:
                    var c = prop.colorValue;
                    oldJsonValue = new { r = c.r, g = c.g, b = c.b, a = c.a };
                    if (!TryParseColor(value, out _))
                    { error = $"Field is Color, expected 'r,g,b[,a]' or '#RRGGBB[AA]', got '{value}'."; return false; }
                    return true;

                case SerializedPropertyType.Enum:
                    oldJsonValue = new { value = prop.intValue, name = (prop.enumDisplayNames != null && prop.enumValueIndex >= 0 && prop.enumValueIndex < prop.enumDisplayNames.Length) ? prop.enumDisplayNames[prop.enumValueIndex] : null };
                    if (!TryParseEnum(prop, value, out _))
                    { error = $"Field is Enum, value '{value}' is neither a known name nor valid integer. Names: {string.Join(",", prop.enumDisplayNames ?? new string[0])}."; return false; }
                    return true;

                case SerializedPropertyType.ObjectReference:
                    {
                        var existing = prop.objectReferenceValue;
                        oldJsonValue = existing != null
                            ? new
                            {
                                type = existing.GetType().Name,
                                name = existing.name,
                                path = AssetDatabase.GetAssetPath(existing),
                            }
                            : null;
                        // We validate by actually resolving. On the apply path we
                        // call the same resolver again — AssetDatabase caches the
                        // path->guid lookup so the double cost is negligible.
                        if (!TryResolveObjectReference(prop, value, out _, out var resolveError))
                        { error = resolveError; return false; }
                        return true;
                    }

                case SerializedPropertyType.AnimationCurve:
                    {
                        oldJsonValue = SerializedInspect.DescribeAnimationCurve(prop.animationCurveValue);
                        if (!TryParseAnimationCurve(value, out _, out var perr))
                        { error = perr; return false; }
                        return true;
                    }

                case SerializedPropertyType.Gradient:
                    {
                        oldJsonValue = SerializedInspect.DescribeGradient(prop.gradientValue);
                        if (!TryParseGradient(value, out _, out var perr))
                        { error = perr; return false; }
                        return true;
                    }

                case SerializedPropertyType.ManagedReference:
                    {
                        oldJsonValue = SerializedInspect.DescribeManagedReference(prop);
                        if (!TryParseManagedReference(prop, value, out _, out _, out var perr))
                        { error = perr; return false; }
                        return true;
                    }

                default:
                    {
                        var unsupportedNames = string.Join(", ", s_UnsupportedSet);
                        if (s_UnsupportedSet.Contains(prop.propertyType))
                        {
                            error = $"Setting SerializedPropertyType.{prop.propertyType} is not supported yet. " +
                                    $"Pending write support: {unsupportedNames}.";
                        }
                        else
                        {
                            error = $"Setting SerializedPropertyType.{prop.propertyType} is not supported yet. " +
                                    $"Currently writes: Integer, Boolean, Float, String, Vector2/3/4, Quaternion, " +
                                    $"Color, Enum, LayerMask, ObjectReference. " +
                                    $"Pending: {unsupportedNames}.";
                        }
                        return false;
                    }
            }
        }

        static void ApplyParsedValue(SerializedProperty prop, string value)
        {
            switch (prop.propertyType)
            {
                case SerializedPropertyType.Integer:
                case SerializedPropertyType.LayerMask:
                case SerializedPropertyType.ArraySize:
                case SerializedPropertyType.Character:
                    prop.intValue = int.Parse(value, NumberStyles.Integer, CultureInfo.InvariantCulture);
                    break;
                case SerializedPropertyType.Boolean:
                    TryParseBool(value, out var b);
                    prop.boolValue = b;
                    break;
                case SerializedPropertyType.Float:
                    prop.floatValue = float.Parse(value, NumberStyles.Float, CultureInfo.InvariantCulture);
                    break;
                case SerializedPropertyType.String:
                    prop.stringValue = value;
                    break;
                case SerializedPropertyType.Vector2:
                    TryParseVector2(value, out var v2);
                    prop.vector2Value = v2;
                    break;
                case SerializedPropertyType.Vector3:
                    ParamCoercion.TryParseVector3(value, out var v3);
                    prop.vector3Value = v3;
                    break;
                case SerializedPropertyType.Vector4:
                    TryParseVector4(value, out var v4);
                    prop.vector4Value = v4;
                    break;
                case SerializedPropertyType.Quaternion:
                    TryParseVector4(value, out var q);
                    prop.quaternionValue = new Quaternion(q.x, q.y, q.z, q.w);
                    break;
                case SerializedPropertyType.Color:
                    TryParseColor(value, out var col);
                    prop.colorValue = col;
                    break;
                case SerializedPropertyType.Enum:
                    TryParseEnum(prop, value, out var e);
                    prop.enumValueIndex = e;
                    break;
                case SerializedPropertyType.ObjectReference:
                    // Resolve again here rather than threading the loaded asset
                    // through the parse->apply split. AssetDatabase caches the
                    // path lookup so the cost is trivial, and keeping the two
                    // paths symmetric avoids a "parse said yes but apply
                    // crashed" disconnect if the asset was swapped between
                    // validation and write.
                    if (TryResolveObjectReference(prop, value, out var obj, out _))
                        prop.objectReferenceValue = obj; // null clears the reference
                    break;
                case SerializedPropertyType.AnimationCurve:
                    if (TryParseAnimationCurve(value, out var curve, out _))
                        prop.animationCurveValue = curve;
                    break;
                case SerializedPropertyType.Gradient:
                    if (TryParseGradient(value, out var gradient, out _))
                        prop.gradientValue = gradient;
                    break;
                case SerializedPropertyType.ManagedReference:
                    if (TryParseManagedReference(prop, value, out var clear, out var mrInstance, out _))
                    {
                        // Undo.RecordObject doesn't snapshot the polymorphic
                        // graph cleanly. RegisterCompleteObjectUndo captures
                        // the full SerializedObject state so Ctrl-Z restores
                        // the previous managedReferenceValue correctly.
                        var target = prop.serializedObject.targetObject;
                        if (target != null)
                            Undo.RegisterCompleteObjectUndo(target, "udit component set (ManagedReference)");
                        prop.managedReferenceValue = clear ? null : mrInstance;
                    }
                    break;
            }
        }

        static object ReadPropertyCurrentValue(Component c, string field)
        {
            using var so = new SerializedObject(c);
            var prop = so.FindProperty(field);
            if (prop == null) return null;
            switch (prop.propertyType)
            {
                case SerializedPropertyType.Integer:
                case SerializedPropertyType.LayerMask:
                case SerializedPropertyType.ArraySize:
                case SerializedPropertyType.Character:
                    return prop.intValue;
                case SerializedPropertyType.Boolean: return prop.boolValue;
                case SerializedPropertyType.Float:   return prop.floatValue;
                case SerializedPropertyType.String:  return prop.stringValue;
                case SerializedPropertyType.Vector2: return new { x = prop.vector2Value.x, y = prop.vector2Value.y };
                case SerializedPropertyType.Vector3: return new { x = prop.vector3Value.x, y = prop.vector3Value.y, z = prop.vector3Value.z };
                case SerializedPropertyType.Vector4:
                case SerializedPropertyType.Quaternion:
                    return new { x = prop.vector4Value.x, y = prop.vector4Value.y, z = prop.vector4Value.z, w = prop.vector4Value.w };
                case SerializedPropertyType.Color:
                    var c2 = prop.colorValue;
                    return new { r = c2.r, g = c2.g, b = c2.b, a = c2.a };
                case SerializedPropertyType.Enum:
                    return new { value = prop.intValue, name = (prop.enumDisplayNames != null && prop.enumValueIndex >= 0 && prop.enumValueIndex < prop.enumDisplayNames.Length) ? prop.enumDisplayNames[prop.enumValueIndex] : null };
                case SerializedPropertyType.ObjectReference:
                    var oref = prop.objectReferenceValue;
                    if (oref == null) return null;
                    return new
                    {
                        type = oref.GetType().Name,
                        name = oref.name,
                        path = AssetDatabase.GetAssetPath(oref),
                    };
                default:
                    return null;
            }
        }

        static string CollectTopLevelFieldNames(SerializedObject so)
        {
            var names = new List<string>();
            var iter = so.GetIterator();
            bool enterChildren = true;
            while (iter.NextVisible(enterChildren))
            {
                enterChildren = false;
                if (iter.name == "m_Script") continue;
                names.Add(iter.name);
            }
            return string.Join(", ", names);
        }
    }
}
