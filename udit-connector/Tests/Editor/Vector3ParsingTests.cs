using System.Reflection;
using NUnit.Framework;
using UnityEngine;
using UditConnector.Tools;

namespace UditConnector.Tests
{
    /// <summary>
    /// Each of ManageComponent, ManageGameObject, and ManagePrefab has its
    /// own private TryParseVector3(string, out Vector3). The duplication
    /// is intentional (each tool ships self-contained so dispatch doesn't
    /// cross-import Tools.*) but leaves three copies free to drift.
    ///
    /// These tests pin each copy to the same expected behavior and fail
    /// loudly if one drifts. Refactoring the three into a shared helper
    /// would remove the duplication but is out of scope for this coverage
    /// slice — see Sprint 3 simplify pass for the consolidation candidate.
    ///
    /// Added in Sprint 3 C2 (coverage gap fill).
    /// </summary>
    public class Vector3ParsingTests
    {
        // Same truth table used against every owner type. The input string
        // is the --pos argument as it arrives from the CLI; the expected
        // bool + Vector3 is the contract every TryParseVector3 must honor.
        static readonly (string input, bool ok, Vector3 want)[] Cases =
        {
            ("0,0,0",       true,  new Vector3(0, 0, 0)),
            ("1,2,3",       true,  new Vector3(1, 2, 3)),
            ("-1.5,0,2.5",  true,  new Vector3(-1.5f, 0, 2.5f)),
            (" 1 , 2 , 3 ", true,  new Vector3(1, 2, 3)), // whitespace tolerated
            ("",            false, default),              // empty
            ("1,2",         false, default),              // too few
            ("1,2,3,4",     false, default),              // too many
            ("1,foo,3",     false, default),              // non-numeric
            ("1;2;3",       false, default),              // wrong separator
        };

        [Test]
        public void ManageComponent_TryParseVector3_MatchesTable()
        {
            foreach (var c in Cases)
            {
                var ok = Invoke(typeof(ManageComponent), c.input, out var got);
                Assert.AreEqual(c.ok, ok, "ok for input " + c.input);
                if (ok) Assert.AreEqual(c.want, got, "value for input " + c.input);
            }
        }

        [Test]
        public void ManageGameObject_TryParseVector3_MatchesTable()
        {
            foreach (var c in Cases)
            {
                var ok = Invoke(typeof(ManageGameObject), c.input, out var got);
                Assert.AreEqual(c.ok, ok, "ok for input " + c.input);
                if (ok) Assert.AreEqual(c.want, got, "value for input " + c.input);
            }
        }

        [Test]
        public void ManagePrefab_TryParseVector3_MatchesTable()
        {
            foreach (var c in Cases)
            {
                var ok = Invoke(typeof(ManagePrefab), c.input, out var got);
                Assert.AreEqual(c.ok, ok, "ok for input " + c.input);
                if (ok) Assert.AreEqual(c.want, got, "value for input " + c.input);
            }
        }

        // Cross-type drift guard: every owner must produce identical
        // (ok, value) tuples for the same input. A failure here means
        // one copy grew a feature or a bug that the others don't share —
        // either consolidate or decide which is canonical.
        [Test]
        public void AllThreeImplementationsAgree()
        {
            foreach (var c in Cases)
            {
                var okC = Invoke(typeof(ManageComponent), c.input, out var vC);
                var okG = Invoke(typeof(ManageGameObject), c.input, out var vG);
                var okP = Invoke(typeof(ManagePrefab), c.input, out var vP);

                Assert.AreEqual(okC, okG, "Component vs GameObject disagree on ok for " + c.input);
                Assert.AreEqual(okC, okP, "Component vs Prefab disagree on ok for " + c.input);

                if (okC)
                {
                    Assert.AreEqual(vC, vG, "Component vs GameObject disagree on value for " + c.input);
                    Assert.AreEqual(vC, vP, "Component vs Prefab disagree on value for " + c.input);
                }
            }
        }

        static bool Invoke(System.Type owner, string s, out Vector3 result)
        {
            var m = owner.GetMethod(
                "TryParseVector3",
                BindingFlags.Static | BindingFlags.NonPublic);
            Assert.IsNotNull(m, owner.Name + ".TryParseVector3 not found — renamed?");
            var args = new object[] { s, default(Vector3) };
            var ok = (bool)m.Invoke(null, args);
            result = (Vector3)args[1];
            return ok;
        }
    }
}
