using System.Reflection;
using NUnit.Framework;
using UnityEditor;
using UnityEngine;
using UditConnector.Tools;

namespace UditConnector.Tests
{
    /// <summary>
    /// Coverage for the "simple" ManageComponent value parsers — the ones
    /// that take a string and produce a primitive or value-type result
    /// (no SerializedProperty context except for enum).
    ///
    /// Sister file to ComponentSetAdvancedTests.cs which covers the four
    /// heavy types (AnimationCurve, Gradient, ManagedReference, scene refs).
    /// Same strategy: call the private static parser via reflection,
    /// assert on the returned value. Keeps the parsers themselves
    /// honest without dragging the full set-action plumbing into each
    /// test.
    ///
    /// Added in Sprint 3 C2 (coverage gap fill).
    /// </summary>
    public class ComponentSetPrimitiveTests
    {
        // ---------- TryParseBool ----------

        [Test]
        public void Bool_AcceptsTruthyForms()
        {
            foreach (var s in new[] { "true", "TRUE", "1", "yes", "ON", "  true  " })
            {
                Assert.IsTrue(InvokeTryParseBool(s, out var b),
                    "expected parse success for " + s);
                Assert.IsTrue(b, "expected true for " + s);
            }
        }

        [Test]
        public void Bool_AcceptsFalsyForms()
        {
            foreach (var s in new[] { "false", "FALSE", "0", "no", "OFF", "  off  " })
            {
                Assert.IsTrue(InvokeTryParseBool(s, out var b),
                    "expected parse success for " + s);
                Assert.IsFalse(b, "expected false for " + s);
            }
        }

        [Test]
        public void Bool_RejectsGarbage()
        {
            foreach (var s in new[] { "", "maybe", "2", "null", "tru" })
            {
                Assert.IsFalse(InvokeTryParseBool(s, out _),
                    "expected parse failure for " + s);
            }
        }

        // ---------- TryParseVector2 / 3 / 4 ----------

        [Test]
        public void Vector2_ParsesCommaSeparated()
        {
            Assert.IsTrue(InvokeTryParseVector2("1,2", out var v));
            Assert.AreEqual(new Vector2(1, 2), v);

            Assert.IsTrue(InvokeTryParseVector2(" 1.5 , -2.25 ", out v));
            Assert.AreEqual(new Vector2(1.5f, -2.25f), v);
        }

        [Test]
        public void Vector2_RejectsWrongArity()
        {
            Assert.IsFalse(InvokeTryParseVector2("1", out _));       // too few
            Assert.IsFalse(InvokeTryParseVector2("1,2,3", out _));   // too many
            Assert.IsFalse(InvokeTryParseVector2("", out _));
            Assert.IsFalse(InvokeTryParseVector2("1,foo", out _));   // bad float
        }

        [Test]
        public void Vector3_ParsesCommaSeparated()
        {
            Assert.IsTrue(InvokeTryParseVector3("1,2,3", out var v));
            Assert.AreEqual(new Vector3(1, 2, 3), v);

            Assert.IsTrue(InvokeTryParseVector3("0.5,-0.5,0", out v));
            Assert.AreEqual(new Vector3(0.5f, -0.5f, 0), v);
        }

        [Test]
        public void Vector3_RejectsWrongArity()
        {
            Assert.IsFalse(InvokeTryParseVector3("1,2", out _));
            Assert.IsFalse(InvokeTryParseVector3("1,2,3,4", out _));
            Assert.IsFalse(InvokeTryParseVector3("", out _));
        }

        [Test]
        public void Vector4_ParsesCommaSeparated()
        {
            Assert.IsTrue(InvokeTryParseVector4("1,2,3,4", out var v));
            Assert.AreEqual(new Vector4(1, 2, 3, 4), v);
        }

        [Test]
        public void Vector4_RejectsWrongArity()
        {
            Assert.IsFalse(InvokeTryParseVector4("1,2,3", out _));
            Assert.IsFalse(InvokeTryParseVector4("1,2,3,4,5", out _));
        }

        // ---------- TryParseColor ----------

        [Test]
        public void Color_ParsesHexRGB()
        {
            Assert.IsTrue(InvokeTryParseColor("#FF8800", out var c));
            Assert.AreEqual(1f, c.r, 1e-3);
            Assert.AreEqual(0.533f, c.g, 1e-2);
            Assert.AreEqual(0f, c.b, 1e-3);
            Assert.AreEqual(1f, c.a, 1e-3);
        }

        [Test]
        public void Color_ParsesHexRGBA()
        {
            Assert.IsTrue(InvokeTryParseColor("#00000080", out var c));
            Assert.AreEqual(0f, c.r, 1e-3);
            Assert.AreEqual(0f, c.g, 1e-3);
            Assert.AreEqual(0f, c.b, 1e-3);
            Assert.AreEqual(0.5f, c.a, 1e-2);
        }

        [Test]
        public void Color_ParsesCommaFloats()
        {
            Assert.IsTrue(InvokeTryParseColor("1,0.5,0.25", out var c));
            Assert.AreEqual(1f, c.r);
            Assert.AreEqual(0.5f, c.g);
            Assert.AreEqual(0.25f, c.b);
            Assert.AreEqual(1f, c.a, "alpha defaults to 1 when omitted");

            Assert.IsTrue(InvokeTryParseColor("1,0.5,0.25,0.1", out c));
            Assert.AreEqual(0.1f, c.a);
        }

        [Test]
        public void Color_RejectsMalformed()
        {
            Assert.IsFalse(InvokeTryParseColor("", out _));
            Assert.IsFalse(InvokeTryParseColor("1,2", out _));       // too few
            Assert.IsFalse(InvokeTryParseColor("1,2,3,4,5", out _)); // too many
            Assert.IsFalse(InvokeTryParseColor("1,foo,3", out _));   // bad float
            Assert.IsFalse(InvokeTryParseColor("#GGGGGG", out _));   // bad hex
        }

        // ---------- TryParseEnum ----------
        //
        // TryParseEnum needs a real SerializedProperty so it can read
        // enumDisplayNames. Build a ScriptableObject fixture with a
        // public enum field and feed its SerializedProperty in.

        public enum Flavor { Vanilla, Chocolate, Strawberry }

        public class EnumHolder : ScriptableObject
        {
            public Flavor flavor = Flavor.Vanilla;
        }

        [Test]
        public void Enum_AcceptsDisplayName()
        {
            var so = ScriptableObject.CreateInstance<EnumHolder>();
            try
            {
                var prop = new SerializedObject(so).FindProperty("flavor");
                Assert.IsTrue(InvokeTryParseEnum(prop, "Chocolate", out var idx));
                Assert.AreEqual(1, idx);
            }
            finally { Object.DestroyImmediate(so); }
        }

        [Test]
        public void Enum_AcceptsDisplayNameCaseInsensitive()
        {
            var so = ScriptableObject.CreateInstance<EnumHolder>();
            try
            {
                var prop = new SerializedObject(so).FindProperty("flavor");
                Assert.IsTrue(InvokeTryParseEnum(prop, "strawberry", out var idx));
                Assert.AreEqual(2, idx);
            }
            finally { Object.DestroyImmediate(so); }
        }

        [Test]
        public void Enum_AcceptsIntegerIndex()
        {
            var so = ScriptableObject.CreateInstance<EnumHolder>();
            try
            {
                var prop = new SerializedObject(so).FindProperty("flavor");
                Assert.IsTrue(InvokeTryParseEnum(prop, "0", out var idx));
                Assert.AreEqual(0, idx);
            }
            finally { Object.DestroyImmediate(so); }
        }

        [Test]
        public void Enum_RejectsOutOfRangeIndex()
        {
            var so = ScriptableObject.CreateInstance<EnumHolder>();
            try
            {
                var prop = new SerializedObject(so).FindProperty("flavor");
                // Flavor has 3 values (indexes 0..2); 5 is out of range.
                Assert.IsFalse(InvokeTryParseEnum(prop, "5", out _));
                Assert.IsFalse(InvokeTryParseEnum(prop, "-1", out _));
            }
            finally { Object.DestroyImmediate(so); }
        }

        [Test]
        public void Enum_RejectsUnknownDisplayName()
        {
            var so = ScriptableObject.CreateInstance<EnumHolder>();
            try
            {
                var prop = new SerializedObject(so).FindProperty("flavor");
                Assert.IsFalse(InvokeTryParseEnum(prop, "Mint", out _));
            }
            finally { Object.DestroyImmediate(so); }
        }

        // ------------------------------------------------------------
        // Reflection helpers. Each wraps one static private TryParse*
        // with a friendlier signature so test bodies stay readable.
        // BindingFlags.Static | NonPublic matches the production code —
        // if a parser's visibility changes, these break loudly via
        // the IsNotNull asserts inside.
        // ------------------------------------------------------------

        static bool InvokeTryParseBool(string s, out bool result)
        {
            var m = typeof(ManageComponent).GetMethod(
                "TryParseBool",
                BindingFlags.Static | BindingFlags.NonPublic);
            Assert.IsNotNull(m, "TryParseBool not found — renamed?");
            var args = new object[] { s, false };
            var ok = (bool)m.Invoke(null, args);
            result = (bool)args[1];
            return ok;
        }

        static bool InvokeTryParseVector2(string s, out Vector2 result)
        {
            var m = typeof(ManageComponent).GetMethod(
                "TryParseVector2",
                BindingFlags.Static | BindingFlags.NonPublic);
            Assert.IsNotNull(m, "TryParseVector2 not found — renamed?");
            var args = new object[] { s, default(Vector2) };
            var ok = (bool)m.Invoke(null, args);
            result = (Vector2)args[1];
            return ok;
        }

        static bool InvokeTryParseVector3(string s, out Vector3 result)
        {
            var m = typeof(ManageComponent).GetMethod(
                "TryParseVector3",
                BindingFlags.Static | BindingFlags.NonPublic);
            Assert.IsNotNull(m, "TryParseVector3 not found — renamed?");
            var args = new object[] { s, default(Vector3) };
            var ok = (bool)m.Invoke(null, args);
            result = (Vector3)args[1];
            return ok;
        }

        static bool InvokeTryParseVector4(string s, out Vector4 result)
        {
            var m = typeof(ManageComponent).GetMethod(
                "TryParseVector4",
                BindingFlags.Static | BindingFlags.NonPublic);
            Assert.IsNotNull(m, "TryParseVector4 not found — renamed?");
            var args = new object[] { s, default(Vector4) };
            var ok = (bool)m.Invoke(null, args);
            result = (Vector4)args[1];
            return ok;
        }

        static bool InvokeTryParseColor(string s, out Color result)
        {
            var m = typeof(ManageComponent).GetMethod(
                "TryParseColor",
                BindingFlags.Static | BindingFlags.NonPublic);
            Assert.IsNotNull(m, "TryParseColor not found — renamed?");
            var args = new object[] { s, default(Color) };
            var ok = (bool)m.Invoke(null, args);
            result = (Color)args[1];
            return ok;
        }

        static bool InvokeTryParseEnum(SerializedProperty prop, string value, out int idx)
        {
            var m = typeof(ManageComponent).GetMethod(
                "TryParseEnum",
                BindingFlags.Static | BindingFlags.NonPublic);
            Assert.IsNotNull(m, "TryParseEnum not found — renamed?");
            var args = new object[] { prop, value, 0 };
            var ok = (bool)m.Invoke(null, args);
            idx = (int)args[2];
            return ok;
        }
    }
}
