using NUnit.Framework;

namespace UditConnector.Tests
{
    /// <summary>
    /// Verifies the Tests asmdef is wired correctly. If `udit test`
    /// surfaces this fixture, the asmdef + UNITY_INCLUDE_TESTS define +
    /// nunit.framework precompiled reference are all in place. Subsequent
    /// commits add real fixtures (ComponentSetAdvancedTests etc.) on the
    /// same asmdef.
    /// </summary>
    [TestFixture]
    public class SentinelTest
    {
        [Test]
        public void TestInfrastructureIsWired()
        {
            // No assertion needed — just being discovered + executable
            // proves the asmdef + UTF references resolve.
            Assert.Pass();
        }
    }
}
