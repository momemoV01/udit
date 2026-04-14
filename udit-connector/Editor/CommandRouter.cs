using System;
using System.Threading;
using System.Threading.Tasks;
using Newtonsoft.Json.Linq;
using UnityEditor;
using UnityEngine;

namespace UditConnector
{
    /// <summary>
    /// Routes incoming command requests to the appropriate tool handler.
    /// All requests are serialized through a single queue to prevent
    /// race conditions when multiple CLI agents access the same Unity instance.
    /// </summary>
    public static class CommandRouter
    {
        // Read-only commands that are safe to run during compile/asset import.
        // Everything else is rejected while Unity is busy to avoid hangs and
        // partial-state crashes (most tools touch APIs that throw mid-reload).
        static readonly System.Collections.Generic.HashSet<string> s_SafeWhileBusy = new()
        {
            "list",
        };

        static readonly SemaphoreSlim s_Lock = new(1, 1);

        public static async Task<object> Dispatch(string command, JObject parameters)
        {
            await s_Lock.WaitAsync();
            try
            {
                return await DispatchInternal(command, parameters);
            }
            finally
            {
                s_Lock.Release();
            }
        }

        static async Task<object> DispatchInternal(string command, JObject parameters)
        {
            // Guard against running while Unity is mid-compile or mid-import.
            if (!s_SafeWhileBusy.Contains(command))
            {
                if (EditorApplication.isCompiling)
                    return new ErrorResponse("Unity is compiling — retry shortly.");
                if (EditorApplication.isUpdating)
                    return new ErrorResponse("Unity is updating (asset import in progress) — retry shortly.");
            }

            if (command == "list")
                return new SuccessResponse("Available tools", ToolDiscovery.GetToolSchemas());

            var handler = ToolDiscovery.FindHandler(command);
            if (handler == null)
                return new ErrorResponse($"Unknown command: {command}");

            try
            {
                var result = handler.Invoke(null, new object[] { parameters ?? new JObject() });

                if (result is Task<object> asyncTask)
                    return await asyncTask;

                if (result is Task task)
                {
                    await task;
                    return new SuccessResponse($"{command} completed");
                }

                return result ?? new SuccessResponse($"{command} completed");
            }
            catch (Exception ex)
            {
                var inner = ex.InnerException ?? ex;
                Debug.LogException(inner);
                return new ErrorResponse($"{command} failed: {inner.Message}");
            }
        }
    }
}
