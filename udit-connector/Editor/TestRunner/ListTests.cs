using System;
using System.Collections.Generic;
using System.Threading.Tasks;
using Newtonsoft.Json.Linq;
using UnityEditor.TestTools.TestRunner.Api;
using UnityEngine;
using Object = UnityEngine.Object;

namespace UditConnector.TestRunner
{
    /// <summary>
    /// Enumerates tests without running them. Uses
    /// <see cref="TestRunnerApi.RetrieveTestList"/> which walks every
    /// [Test]/[UnityTest] method in the current assemblies and returns the
    /// tree shape before any execution starts.
    ///
    /// Returns a flat list of leaf tests (IsSuite == false) so agents can
    /// pick specific tests to run by full name via `udit test run --filter`
    /// without the tree navigation overhead.
    /// </summary>
    [UditTool(Description = "List Unity EditMode or PlayMode tests without running them.")]
    public static class ListTests
    {
        public class Parameters
        {
            [ToolParameter("Test mode: EditMode (default) or PlayMode")]
            public string Mode { get; set; }
        }

        public static Task<object> HandleCommand(JObject @params)
        {
            var p = new ToolParams(@params ?? new JObject());
            var modeStr = (p.Get("mode") ?? "EditMode").Trim();

            TestMode testMode;
            if (modeStr.Equals("EditMode", StringComparison.OrdinalIgnoreCase))
                testMode = TestMode.EditMode;
            else if (modeStr.Equals("PlayMode", StringComparison.OrdinalIgnoreCase))
                testMode = TestMode.PlayMode;
            else
                return Task.FromResult<object>(new ErrorResponse(
                    $"Unknown mode '{modeStr}'. Use EditMode or PlayMode."));

            var tcs = new TaskCompletionSource<object>(TaskCreationOptions.RunContinuationsAsynchronously);
            var api = ScriptableObject.CreateInstance<TestRunnerApi>();

            // RetrieveTestList is async-callback based — the API resolves on
            // the main thread so we can safely walk the result inside the
            // callback and close the TaskCompletionSource there.
            api.RetrieveTestList(testMode, root =>
            {
                try
                {
                    var tests = new List<object>();
                    Walk(root, tests);

                    tcs.TrySetResult(new SuccessResponse(
                        $"{tests.Count} test(s) in {modeStr}.",
                        new
                        {
                            mode = testMode.ToString(),
                            total = tests.Count,
                            tests,
                        }));
                }
                catch (Exception ex)
                {
                    tcs.TrySetResult(new ErrorResponse(
                        $"Failed to walk test list: {ex.Message}"));
                }
                finally
                {
                    Object.DestroyImmediate(api);
                }
            });

            return tcs.Task;
        }

        static void Walk(ITestAdaptor node, List<object> results)
        {
            if (node == null) return;

            if (!node.IsSuite)
            {
                // Leaf test case.
                var className = SplitClassAndMethod(node.FullName, node.Name, out var methodName);
                results.Add(new
                {
                    full_name = node.FullName,
                    name = methodName,
                    class_name = className,
                    type_info = node.TypeInfo?.FullName,
                    run_state = node.RunState.ToString(),
                });
                return;
            }

            if (node.Children != null)
                foreach (var child in node.Children)
                    Walk(child, results);
        }

        static string SplitClassAndMethod(string fullName, string name, out string method)
        {
            method = name;
            if (string.IsNullOrEmpty(fullName) || string.IsNullOrEmpty(name))
                return fullName;

            var suffix = "." + name;
            if (fullName.EndsWith(suffix, StringComparison.Ordinal))
                return fullName.Substring(0, fullName.Length - suffix.Length);
            return fullName;
        }
    }
}
