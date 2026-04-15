using System.Linq;
using NUnit.Framework;
using UnityEditor;
using UnityEngine;
using UditConnector.Tools;

namespace UditConnector.Tests
{
    /// <summary>
    /// Ground-truth tests for the four advanced `component set` value
    /// types (AnimationCurve, Gradient, ManagedReference, scene refs).
    ///
    /// Strategy: each test creates a ScriptableObject or in-scene
    /// GameObject with a field of the target type, then asserts on
    /// `prop.{animationCurveValue|gradientValue|managedReferenceValue|
    /// objectReferenceValue}` directly — NOT via udit's
    /// SerializedInspect reader. Prevents setter and reader from
    /// masking each other's bugs.
    ///
    /// This file grows one fixture at a time with each commit in the
    /// v0.9.0 series.
    /// </summary>
    public class ComponentSetAdvancedTests
    {
        // ---------- AnimationCurve ----------

        public class CurveHolder : ScriptableObject
        {
            public AnimationCurve curve = AnimationCurve.Linear(0, 0, 1, 1);
        }

        [Test]
        public void AnimationCurve_ParseMinimalTwoKey_SetsBoth()
        {
            var so = ScriptableObject.CreateInstance<CurveHolder>();
            try
            {
                var sobj = new SerializedObject(so);
                var prop = sobj.FindProperty("curve");

                // Call the internal parser directly — both the parse-and-
                // validate phase and the assignment happen through the same
                // path that ManageComponent.ApplyParsedValue would use.
                var json = @"{""keys"":[{""t"":0,""v"":0},{""t"":1,""v"":1}]}";
                var curve = InvokeTryParseAnimationCurve(json);
                Assert.IsNotNull(curve, "parser returned null");

                prop.animationCurveValue = curve;
                sobj.ApplyModifiedProperties();

                // Re-read independently and assert.
                sobj.Update();
                var resultProp = sobj.FindProperty("curve");
                var keys = resultProp.animationCurveValue.keys;

                Assert.AreEqual(2, keys.Length, "key count");
                Assert.AreEqual(0f, keys[0].time, 1e-6);
                Assert.AreEqual(0f, keys[0].value, 1e-6);
                Assert.AreEqual(1f, keys[1].time, 1e-6);
                Assert.AreEqual(1f, keys[1].value, 1e-6);
                Assert.AreEqual(WrapMode.ClampForever, resultProp.animationCurveValue.preWrapMode);
                Assert.AreEqual(WrapMode.ClampForever, resultProp.animationCurveValue.postWrapMode);
            }
            finally
            {
                Object.DestroyImmediate(so);
            }
        }

        [Test]
        public void AnimationCurve_AppliesTangentsWhenSupplied()
        {
            var so = ScriptableObject.CreateInstance<CurveHolder>();
            try
            {
                var sobj = new SerializedObject(so);
                var prop = sobj.FindProperty("curve");

                var json = @"{""keys"":[{""t"":0,""v"":0,""inT"":0,""outT"":2},{""t"":1,""v"":1,""inT"":2,""outT"":0}]}";
                var curve = InvokeTryParseAnimationCurve(json);
                Assert.IsNotNull(curve);

                prop.animationCurveValue = curve;
                sobj.ApplyModifiedProperties();
                sobj.Update();

                var keys = sobj.FindProperty("curve").animationCurveValue.keys;
                Assert.AreEqual(0f, keys[0].inTangent, 1e-6);
                Assert.AreEqual(2f, keys[0].outTangent, 1e-6);
                Assert.AreEqual(2f, keys[1].inTangent, 1e-6);
                Assert.AreEqual(0f, keys[1].outTangent, 1e-6);
            }
            finally
            {
                Object.DestroyImmediate(so);
            }
        }

        [Test]
        public void AnimationCurve_ParsesWrapModes()
        {
            var so = ScriptableObject.CreateInstance<CurveHolder>();
            try
            {
                var sobj = new SerializedObject(so);
                var prop = sobj.FindProperty("curve");

                var json = @"{""keys"":[{""t"":0,""v"":0},{""t"":1,""v"":1}],""preWrap"":""Loop"",""postWrap"":""PingPong""}";
                var curve = InvokeTryParseAnimationCurve(json);
                Assert.IsNotNull(curve);

                prop.animationCurveValue = curve;
                sobj.ApplyModifiedProperties();
                sobj.Update();

                Assert.AreEqual(WrapMode.Loop, sobj.FindProperty("curve").animationCurveValue.preWrapMode);
                Assert.AreEqual(WrapMode.PingPong, sobj.FindProperty("curve").animationCurveValue.postWrapMode);
            }
            finally
            {
                Object.DestroyImmediate(so);
            }
        }

        [Test]
        public void AnimationCurve_RejectsUnknownWrapMode()
        {
            // "Clamp" is intentionally not tested here — Unity's WrapMode
            // enum keeps it as a legacy alias for Once, so Enum.TryParse
            // accepts it. Use a genuinely invalid name.
            var json = @"{""keys"":[{""t"":0,""v"":0},{""t"":1,""v"":1}],""preWrap"":""InvalidMode""}";
            var err = InvokeTryParseAnimationCurveError(json);
            Assert.IsNotNull(err, "expected error for invalid WrapMode");
            Assert.That(err, Does.Contain("WrapMode"),
                "error message should mention WrapMode; got: " + err);
        }

        [Test]
        public void AnimationCurve_RejectsMalformedJson()
        {
            var err = InvokeTryParseAnimationCurveError(@"not json");
            Assert.IsNotNull(err);
            Assert.That(err, Does.Contain("invalid JSON").Or.Contain("AnimationCurve"));
        }

        // ---------- Gradient ----------

        public class GradientHolder : ScriptableObject
        {
            public Gradient grad = new Gradient();
        }

        [Test]
        public void Gradient_ParseMinimalBlackWhite_SetsColorAndAlphaKeys()
        {
            var so = ScriptableObject.CreateInstance<GradientHolder>();
            try
            {
                var sobj = new SerializedObject(so);
                var prop = sobj.FindProperty("grad");

                var json = @"{""colorKeys"":[{""t"":0,""color"":""#000000""},{""t"":1,""color"":""#FFFFFF""}],""alphaKeys"":[{""t"":0,""a"":1},{""t"":1,""a"":1}]}";
                var g = InvokeTryParseGradient(json);
                Assert.IsNotNull(g, "parser returned null");

                prop.gradientValue = g;
                sobj.ApplyModifiedProperties();
                sobj.Update();

                var result = sobj.FindProperty("grad").gradientValue;
                Assert.AreEqual(2, result.colorKeys.Length);
                Assert.AreEqual(2, result.alphaKeys.Length);
                Assert.AreEqual(0f, result.colorKeys[0].time, 1e-6);
                Assert.AreEqual(0f, result.colorKeys[0].color.r, 1e-3);
                Assert.AreEqual(1f, result.colorKeys[1].color.r, 1e-3);
                Assert.AreEqual(GradientMode.Blend, result.mode);
            }
            finally { Object.DestroyImmediate(so); }
        }

        [Test]
        public void Gradient_AcceptsFixedMode()
        {
            var so = ScriptableObject.CreateInstance<GradientHolder>();
            try
            {
                var sobj = new SerializedObject(so);
                var prop = sobj.FindProperty("grad");

                var json = @"{""colorKeys"":[{""t"":0,""color"":""#FF0000""},{""t"":1,""color"":""#00FF00""}],""alphaKeys"":[{""t"":0,""a"":1},{""t"":1,""a"":1}],""mode"":""Fixed""}";
                var g = InvokeTryParseGradient(json);
                Assert.IsNotNull(g);
                prop.gradientValue = g;
                sobj.ApplyModifiedProperties();
                sobj.Update();
                Assert.AreEqual(GradientMode.Fixed, sobj.FindProperty("grad").gradientValue.mode);
            }
            finally { Object.DestroyImmediate(so); }
        }

        [Test]
        public void Gradient_RejectsTooFewKeys()
        {
            var json = @"{""colorKeys"":[{""t"":0,""color"":""#000000""}],""alphaKeys"":[{""t"":0,""a"":1},{""t"":1,""a"":1}]}";
            var err = InvokeTryParseGradientError(json);
            Assert.IsNotNull(err);
            Assert.That(err, Does.Contain("colorKeys count"));
        }

        [Test]
        public void Gradient_RejectsTooManyKeys()
        {
            // 9 color keys — over the 8 cap.
            var manyKeys = string.Join(",",
                System.Linq.Enumerable.Range(0, 9).Select(i => $@"{{""t"":{i/8f:F3},""color"":""#FFFFFF""}}"));
            var json = $@"{{""colorKeys"":[{manyKeys}],""alphaKeys"":[{{""t"":0,""a"":1}},{{""t"":1,""a"":1}}]}}";
            var err = InvokeTryParseGradientError(json);
            Assert.IsNotNull(err);
            Assert.That(err, Does.Contain("colorKeys count"));
        }

        [Test]
        public void Gradient_RejectsInvalidColor()
        {
            var json = @"{""colorKeys"":[{""t"":0,""color"":""notacolor""},{""t"":1,""color"":""#FFFFFF""}],""alphaKeys"":[{""t"":0,""a"":1},{""t"":1,""a"":1}]}";
            var err = InvokeTryParseGradientError(json);
            Assert.IsNotNull(err);
            Assert.That(err, Does.Contain("not a valid hex"));
        }

        [Test]
        public void Gradient_RejectsInvalidMode()
        {
            var json = @"{""colorKeys"":[{""t"":0,""color"":""#000000""},{""t"":1,""color"":""#FFFFFF""}],""alphaKeys"":[{""t"":0,""a"":1},{""t"":1,""a"":1}],""mode"":""Bogus""}";
            var err = InvokeTryParseGradientError(json);
            Assert.IsNotNull(err);
            Assert.That(err, Does.Contain("GradientMode"));
        }

        static Gradient InvokeTryParseGradient(string json)
        {
            var m = typeof(ManageComponent).GetMethod("TryParseGradient",
                System.Reflection.BindingFlags.Static | System.Reflection.BindingFlags.NonPublic);
            Assert.IsNotNull(m, "TryParseGradient not found — renamed?");
            var args = new object[] { json, null, null };
            var ok = (bool)m.Invoke(null, args);
            return ok ? (Gradient)args[1] : null;
        }

        static string InvokeTryParseGradientError(string json)
        {
            var m = typeof(ManageComponent).GetMethod("TryParseGradient",
                System.Reflection.BindingFlags.Static | System.Reflection.BindingFlags.NonPublic);
            var args = new object[] { json, null, null };
            var ok = (bool)m.Invoke(null, args);
            return ok ? null : (string)args[2];
        }

        // ---------- Scene references ----------

        public class SceneRefHolder : ScriptableObject
        {
            public GameObject targetGo;
            public Camera targetCamera;
        }

        // Live-smoke only: `SceneRef_AssignsGameObjectToGoField`,
        // `SceneRef_AutoExtractsComponentFromGo`, and
        // `SceneRef_RejectsMissingComponent` all need a scene-resident GO
        // whose GlobalObjectId round-trips through
        // GlobalObjectIdentifierToObjectSlow. In EditMode tests the GO
        // has no persistent localFileId until the scene is saved — saving
        // a test scene on every run is heavy machinery for a feature
        // better verified end-to-end. The parse-level branch (unknown
        // stable ID, and the TryResolveObjectReference branch dispatch
        // itself) is still covered below.

        [Test]
        public void SceneRef_RejectsUnknownStableId()
        {
            var so = ScriptableObject.CreateInstance<SceneRefHolder>();
            try
            {
                var sobj = new SerializedObject(so);
                var prop = sobj.FindProperty("targetGo");

                var obj = InvokeTryResolveObjectReference(prop, "go:deadbeef", out var err);
                Assert.IsNull(obj);
                Assert.IsNotNull(err);
                Assert.That(err, Does.Contain("GameObject not found"));
            }
            finally
            {
                Object.DestroyImmediate(so);
            }
        }

        // Reflection-invoker for the private TryResolveObjectReference. Same
        // pattern as the other parsers — tests reach into ManageComponent's
        // internals so the resolver can be exercised without going through
        // the full `udit component set` HTTP round-trip.
        static UnityEngine.Object InvokeTryResolveObjectReference(SerializedProperty prop, string value, out string error)
        {
            var m = typeof(ManageComponent).GetMethod(
                "TryResolveObjectReference",
                System.Reflection.BindingFlags.Static | System.Reflection.BindingFlags.NonPublic);
            Assert.IsNotNull(m, "TryResolveObjectReference not found — renamed?");
            var args = new object[] { prop, value, null, null };
            var ok = (bool)m.Invoke(null, args);
            error = ok ? null : (string)args[3];
            return ok ? (UnityEngine.Object)args[2] : null;
        }

        // ---------- Shims for private parser ----------
        //
        // ManageComponent.TryParseAnimationCurve is `static` but private
        // to the class. Test reaches in via reflection — acceptable
        // coupling for a same-package test assembly that's checking the
        // parser shape directly.

        static AnimationCurve InvokeTryParseAnimationCurve(string json)
        {
            var m = typeof(ManageComponent).GetMethod(
                "TryParseAnimationCurve",
                System.Reflection.BindingFlags.Static | System.Reflection.BindingFlags.NonPublic);
            Assert.IsNotNull(m, "TryParseAnimationCurve not found — renamed?");
            var args = new object[] { json, null, null };
            var ok = (bool)m.Invoke(null, args);
            return ok ? (AnimationCurve)args[1] : null;
        }

        static string InvokeTryParseAnimationCurveError(string json)
        {
            var m = typeof(ManageComponent).GetMethod(
                "TryParseAnimationCurve",
                System.Reflection.BindingFlags.Static | System.Reflection.BindingFlags.NonPublic);
            var args = new object[] { json, null, null };
            var ok = (bool)m.Invoke(null, args);
            return ok ? null : (string)args[2];
        }
    }
}
