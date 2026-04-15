using NUnit.Framework;
using UnityEngine;

namespace UditConnector.Tests
{
    /// <summary>
    /// Pins ParamCoercion.TryParseVector3 — the single shared --pos
    /// parser behind ManageGameObject / ManagePrefab / ManageComponent —
    /// to an explicit truth table.
    ///
    /// History: this file used to guard three duplicated TryParseVector3
    /// copies against drift (Sprint 3 C2.5). In Sprint 4 C3 those copies
    /// were collapsed into ParamCoercion.TryParseVector3, so the
    /// cross-type "all three agree" invariant is no longer needed — there
    /// is only one implementation now. The truth table stays as the
    /// behavioral contract.
    /// </summary>
    public class Vector3ParsingTests
    {
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
        public void ParamCoercion_TryParseVector3_MatchesTable()
        {
            foreach (var c in Cases)
            {
                var ok = ParamCoercion.TryParseVector3(c.input, out var got);
                Assert.AreEqual(c.ok, ok, "ok for input " + c.input);
                if (ok) Assert.AreEqual(c.want, got, "value for input " + c.input);
            }
        }
    }
}
