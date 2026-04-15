using System;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using System.Text;
using System.Threading.Tasks;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;
using UnityEditor.TestTools.TestRunner.Api;
using UnityEngine;
using Object = UnityEngine.Object;

namespace UditConnector.TestRunner
{
    [UditTool(Description = "Run Unity EditMode or PlayMode tests and return results.")]
    public static class RunTests
    {
        internal static readonly string StatusDir = Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.UserProfile), ".udit", "status");

        public class Parameters
        {
            [ToolParameter("Test mode: EditMode or PlayMode", Required = true)]
            public string Mode { get; set; }

            [ToolParameter("Filter by namespace, class, or full test name")]
            public string Filter { get; set; }

            [ToolParameter("Optional JUnit XML output path. Absolute recommended. Via `udit` CLI, relative paths resolve against the CLI cwd; direct HTTP callers fall back to the Unity project root.")]
            public string Output { get; set; }
        }

        // Per-test record captured during the run. We keep the detail client-
        // side so the JSON response stays backward-compatible (just names and
        // "name: message" strings) while JUnit XML has enough to render a
        // proper report.
        internal struct TestRecord
        {
            public string FullName;
            public string ClassName;
            public string TestName;
            public string Status;       // "Passed", "Failed", "Skipped", "Inconclusive"
            public string Message;
            public string StackTrace;
            public double DurationSec;
        }

        public static Task<object> HandleCommand(JObject @params)
        {
            if (@params == null)
                return Task.FromResult<object>(new ErrorResponse("Parameters cannot be null."));

            var p = new ToolParams(@params);

            var modeResult = p.GetRequired("mode");
            if (!modeResult.IsSuccess)
                return Task.FromResult<object>(new ErrorResponse(modeResult.ErrorMessage));

            var modeStr = modeResult.Value.Trim();
            TestMode testMode;
            if (modeStr.Equals("EditMode", StringComparison.OrdinalIgnoreCase))
                testMode = TestMode.EditMode;
            else if (modeStr.Equals("PlayMode", StringComparison.OrdinalIgnoreCase))
                testMode = TestMode.PlayMode;
            else
                return Task.FromResult<object>(new ErrorResponse($"Unknown mode '{modeStr}'. Use EditMode or PlayMode."));

            var filter = p.Get("filter");
            var output = p.Get("output");

            if (testMode == TestMode.EditMode)
                return ExecuteInProcess(testMode, filter, output);

            StartPlayModeRun(filter, output);
            return Task.FromResult<object>(new SuccessResponse("running", new { port = HttpServer.Port }));
        }

        private static Task<object> ExecuteInProcess(TestMode mode, string filter, string output)
        {
            var tcs = new TaskCompletionSource<object>(TaskCreationOptions.RunContinuationsAsynchronously);
            var records = new List<TestRecord>();

            var api = ScriptableObject.CreateInstance<TestRunnerApi>();
            var callbacks = new TestCallbacks(
                onResult: r => CollectResult(r, records),
                onFinished: _ =>
                {
                    if (tcs.Task.IsCompleted) return;
                    Object.DestroyImmediate(api);
                    if (!string.IsNullOrEmpty(output)) TryWriteJUnit(output, records);
                    tcs.TrySetResult(BuildResponse(records, output));
                }
            );

            api.RegisterCallbacks(callbacks);
            api.Execute(new ExecutionSettings(BuildFilter(mode, filter)));
            return tcs.Task;
        }

        private static void StartPlayModeRun(string filter, string output)
        {
            var port = HttpServer.Port;

            try { var f = ResultsFilePath(port); if (File.Exists(f)) File.Delete(f); } catch { }
            TestRunnerState.MarkPending(port, filter, output);

            var records = new List<TestRecord>();

            var api = ScriptableObject.CreateInstance<TestRunnerApi>();
            var callbacks = new TestCallbacks(
                onResult: r => CollectResult(r, records),
                onFinished: _ =>
                {
                    Object.DestroyImmediate(api);
                    TestRunnerState.ClearPending(port);
                    if (!string.IsNullOrEmpty(output)) TryWriteJUnit(output, records);
                    WriteResultsFile(port, records, output);
                }
            );

            api.RegisterCallbacks(callbacks);
            api.Execute(new ExecutionSettings(BuildFilter(TestMode.PlayMode, filter)));
        }

        // --- Shared helpers (used by TestRunnerState after domain reload) ---

        internal static void CollectResult(ITestResultAdaptor result, List<TestRecord> records)
        {
            if (result.Test.IsSuite) return;

            var rec = new TestRecord
            {
                FullName   = result.Test.FullName,
                ClassName  = result.Test.FullName != null && result.Test.Name != null
                             ? StripSuffix(result.Test.FullName, "." + result.Test.Name)
                             : result.Test.FullName,
                TestName   = result.Test.Name,
                Status     = result.TestStatus.ToString(),
                Message    = result.Message ?? string.Empty,
                StackTrace = result.StackTrace ?? string.Empty,
                DurationSec = result.Duration,
            };
            records.Add(rec);
        }

        internal static void WriteResultsFile(int port, List<TestRecord> records, string output)
        {
            var passed  = new List<string>();
            var failed  = new List<string>();
            var skipped = new List<string>();
            Split(records, passed, failed, skipped);

            var data = new
            {
                success = failed.Count == 0,
                message = failed.Count > 0
                    ? $"{failed.Count} test(s) failed."
                    : $"All {passed.Count} test(s) passed.",
                data = new
                {
                    total   = records.Count,
                    passed  = passed.Count,
                    failed  = failed.Count,
                    skipped = skipped.Count,
                    failures = failed,
                    passes   = passed,
                    output_written = string.IsNullOrEmpty(output) ? null : output,
                }
            };

            try
            {
                Directory.CreateDirectory(StatusDir);
                File.WriteAllText(ResultsFilePath(port), JsonConvert.SerializeObject(data));
            }
            catch (Exception ex)
            {
                Debug.LogError($"[UditConnector] Failed to write test results: {ex.Message}");
            }
        }

        internal static string ResultsFilePath(int port) =>
            Path.Combine(StatusDir, $"test-results-{port}.json");

        internal static object BuildResponse(List<TestRecord> records, string output)
        {
            var passed  = new List<string>();
            var failed  = new List<string>();
            var skipped = new List<string>();
            Split(records, passed, failed, skipped);

            var summary = new
            {
                total   = records.Count,
                passed  = passed.Count,
                failed  = failed.Count,
                skipped = skipped.Count,
                failures = failed,
                passes   = passed,
                output_written = string.IsNullOrEmpty(output) ? null : output,
            };
            return failed.Count > 0
                ? (object)new ErrorResponse($"{failed.Count} test(s) failed.", summary)
                : new SuccessResponse($"All {passed.Count} test(s) passed.", summary);
        }

        // Compact existing name+message lists from the richer record form. Kept
        // as a helper so the public response shape does not drift.
        static void Split(List<TestRecord> records, List<string> passed, List<string> failed, List<string> skipped)
        {
            foreach (var r in records)
            {
                switch (r.Status)
                {
                    case "Passed":
                        passed.Add(r.FullName);
                        break;
                    case "Failed":
                        failed.Add(string.IsNullOrEmpty(r.Message) ? r.FullName : $"{r.FullName}: {r.Message}");
                        break;
                    default:
                        skipped.Add(r.FullName);
                        break;
                }
            }
        }

        internal static Filter BuildFilter(TestMode mode, string filterStr)
        {
            var f = new Filter { testMode = mode };
            if (!string.IsNullOrEmpty(filterStr))
            {
                f.testNames  = new[] { filterStr };
                f.groupNames = new[] { filterStr };
            }
            return f;
        }

        // --- JUnit XML output --------------------------------------------

        internal static void TryWriteJUnit(string outputPath, List<TestRecord> records)
        {
            try
            {
                var fullPath = ResolveOutputPath(outputPath);
                var dir = Path.GetDirectoryName(fullPath);
                if (!string.IsNullOrEmpty(dir)) Directory.CreateDirectory(dir);
                File.WriteAllText(fullPath, BuildJUnitXml(records));
            }
            catch (Exception ex)
            {
                // Failing to write the report shouldn't fail the run itself —
                // the primary signal is the response/results-file. Surface via
                // the Editor console so the agent sees it if they read logs.
                Debug.LogError($"[UditConnector] Failed to write JUnit XML to '{outputPath}': {ex.Message}");
            }
        }

        static string ResolveOutputPath(string p)
        {
            if (Path.IsPathRooted(p)) return p;
            // Relative paths anchor at the project root (parent of Assets/).
            var projectRoot = Path.GetDirectoryName(Application.dataPath) ?? Environment.CurrentDirectory;
            return Path.Combine(projectRoot, p);
        }

        static string BuildJUnitXml(List<TestRecord> records)
        {
            int failures = 0;
            int skippedCount = 0;
            double total = 0;
            foreach (var r in records)
            {
                if (r.Status == "Failed") failures++;
                else if (r.Status != "Passed") skippedCount++;
                total += r.DurationSec;
            }

            var sb = new StringBuilder();
            sb.AppendLine("<?xml version=\"1.0\" encoding=\"UTF-8\"?>");
            sb.AppendFormat(CultureInfo.InvariantCulture,
                "<testsuites tests=\"{0}\" failures=\"{1}\" skipped=\"{2}\" time=\"{3:F3}\">",
                records.Count, failures, skippedCount, total);
            sb.AppendLine();

            // Group by class name so each <testsuite> is a real class bucket.
            var bySuite = new Dictionary<string, List<TestRecord>>();
            foreach (var r in records)
            {
                var key = string.IsNullOrEmpty(r.ClassName) ? "(unknown)" : r.ClassName;
                if (!bySuite.TryGetValue(key, out var list))
                {
                    list = new List<TestRecord>();
                    bySuite[key] = list;
                }
                list.Add(r);
            }

            foreach (var kv in bySuite)
            {
                int suiteFailures = 0, suiteSkipped = 0;
                double suiteTime = 0;
                foreach (var r in kv.Value)
                {
                    if (r.Status == "Failed") suiteFailures++;
                    else if (r.Status != "Passed") suiteSkipped++;
                    suiteTime += r.DurationSec;
                }

                sb.AppendFormat(CultureInfo.InvariantCulture,
                    "  <testsuite name=\"{0}\" tests=\"{1}\" failures=\"{2}\" skipped=\"{3}\" time=\"{4:F3}\">",
                    Escape(kv.Key), kv.Value.Count, suiteFailures, suiteSkipped, suiteTime);
                sb.AppendLine();

                foreach (var r in kv.Value)
                {
                    sb.AppendFormat(CultureInfo.InvariantCulture,
                        "    <testcase classname=\"{0}\" name=\"{1}\" time=\"{2:F3}\">",
                        Escape(r.ClassName), Escape(r.TestName), r.DurationSec);
                    sb.AppendLine();

                    if (r.Status == "Failed")
                    {
                        sb.AppendFormat("      <failure message=\"{0}\">{1}</failure>",
                            Escape(FirstLine(r.Message)), Escape(r.Message + "\n" + r.StackTrace));
                        sb.AppendLine();
                    }
                    else if (r.Status != "Passed")
                    {
                        // Everything that is not Passed / Failed lands here —
                        // Inconclusive, Skipped, etc. JUnit's <skipped/> is
                        // the closest semantic match.
                        sb.AppendFormat("      <skipped message=\"{0}\"/>",
                            Escape(string.IsNullOrEmpty(r.Message) ? r.Status : r.Message));
                        sb.AppendLine();
                    }

                    sb.AppendLine("    </testcase>");
                }

                sb.AppendLine("  </testsuite>");
            }

            sb.AppendLine("</testsuites>");
            return sb.ToString();
        }

        static string Escape(string s)
        {
            if (string.IsNullOrEmpty(s)) return string.Empty;
            var sb = new StringBuilder(s.Length);
            foreach (var c in s)
            {
                switch (c)
                {
                    case '&':  sb.Append("&amp;");  break;
                    case '<':  sb.Append("&lt;");   break;
                    case '>':  sb.Append("&gt;");   break;
                    case '"':  sb.Append("&quot;"); break;
                    case '\'': sb.Append("&apos;"); break;
                    default:   sb.Append(c);        break;
                }
            }
            return sb.ToString();
        }

        static string FirstLine(string s)
        {
            if (string.IsNullOrEmpty(s)) return string.Empty;
            var idx = s.IndexOfAny(new[] { '\r', '\n' });
            return idx >= 0 ? s.Substring(0, idx) : s;
        }

        static string StripSuffix(string s, string suffix)
        {
            if (s == null || suffix == null) return s;
            return s.EndsWith(suffix, StringComparison.Ordinal)
                ? s.Substring(0, s.Length - suffix.Length)
                : s;
        }

        internal class TestCallbacks : ICallbacks
        {
            private readonly Action<ITestResultAdaptor> _onResult;
            private readonly Action<ITestResultAdaptor> _onFinished;

            public TestCallbacks(Action<ITestResultAdaptor> onResult, Action<ITestResultAdaptor> onFinished)
            {
                _onResult   = onResult;
                _onFinished = onFinished;
            }

            public void RunStarted(ITestAdaptor testsToRun) { }
            public void RunFinished(ITestResultAdaptor result) => _onFinished(result);
            public void TestStarted(ITestAdaptor test) { }
            public void TestFinished(ITestResultAdaptor result) => _onResult(result);
        }
    }
}
