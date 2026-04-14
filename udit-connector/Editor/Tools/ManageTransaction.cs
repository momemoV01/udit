using Newtonsoft.Json.Linq;
using UnityEditor;

namespace UditConnector.Tools
{
    /// <summary>
    /// Groups a sequence of mutation commands into a single Unity Undo entry.
    /// Without a transaction, each mutation (go create, component add, etc.)
    /// lives in its own Undo group because every mutation action explicitly
    /// increments the group at its start — that is correct for single commands
    /// but means reversing a multi-step agent change requires N Ctrl+Z's.
    ///
    /// Inside a transaction, mutations still create their own sub-groups as
    /// usual, and commit calls <see cref="Undo.CollapseUndoOperations"/> to
    /// merge them into one named group. Rollback calls
    /// <see cref="Undo.RevertAllDownToGroup"/> to unwind everything back to
    /// the state at begin.
    ///
    /// Only one transaction is active per Unity instance at any time (the
    /// Undo stack itself is global). Attempting to begin a second one
    /// returns UCI-011 with the existing transaction's description — agents
    /// should commit or rollback the first before starting another.
    ///
    /// State lifetime: the active transaction lives in a static field that
    /// is torn down with the domain. Domain reloads (script compile) wipe
    /// the state — `tx status` will report "no active" after a reload, and
    /// the agent should re-run `tx begin` if they intended to keep grouping.
    /// Any mutations that landed before the reload are still on the Undo
    /// stack; they just are not part of any transaction.
    /// </summary>
    [UditTool(Description = "Group mutations into a single Undo entry. Actions: begin, commit, rollback, status.")]
    public static class ManageTransaction
    {
        // Saved Undo group index captured at `begin`. null means no active tx.
        static int? s_ActiveGroup;
        static string s_ActiveName;
        static System.DateTime? s_ActiveStarted;

        public class Parameters
        {
            [ToolParameter("Action to perform: begin, commit, rollback, status", Required = true)]
            public string Action { get; set; }

            [ToolParameter("Descriptive name for the transaction (shown in Edit → Undo menu)")]
            public string Name { get; set; }
        }

        public static object HandleCommand(JObject @params)
        {
            if (@params == null)
                return new ErrorResponse(ErrorCodes.InvalidParams, "Parameters cannot be null.");

            var p = new ToolParams(@params);
            var actionResult = p.GetRequired("action");
            if (!actionResult.IsSuccess)
                return new ErrorResponse(ErrorCodes.InvalidParams, actionResult.ErrorMessage);

            var action = actionResult.Value.ToLowerInvariant();
            switch (action)
            {
                case "begin":    return Begin(p);
                case "commit":   return Commit(p);
                case "rollback": return Rollback();
                case "status":   return Status();
                default:
                    return new ErrorResponse(ErrorCodes.InvalidParams,
                        $"Unknown action '{action}'. Available: begin, commit, rollback, status.");
            }
        }

        static object Begin(ToolParams p)
        {
            if (EditorApplication.isPlayingOrWillChangePlaymode)
                return new ErrorResponse("Cannot begin a transaction while in play mode.");

            if (s_ActiveGroup.HasValue)
            {
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    $"A transaction is already active (name: '{s_ActiveName}', started {FormatAge(s_ActiveStarted)} ago). " +
                    $"Commit or rollback the existing transaction before starting a new one.");
            }

            // Increment so any mutation that follows lands in a fresh group —
            // CollapseUndoOperations then merges from this group upward at
            // commit time. Without the explicit bump, a mutation that happens
            // to land in the *current* Editor-tick group would be swept into
            // the collapse unintentionally.
            Undo.IncrementCurrentGroup();
            var group = Undo.GetCurrentGroup();

            var name = p.Get("name");
            if (string.IsNullOrEmpty(name)) name = "udit transaction";

            s_ActiveGroup = group;
            s_ActiveName = name;
            s_ActiveStarted = System.DateTime.UtcNow;

            // Label the group so mutations that land inside it inherit the
            // name (until they Set their own). The final name shown in the
            // Edit → Undo menu gets overwritten at commit.
            Undo.SetCurrentGroupName(name);

            return new SuccessResponse(
                $"Transaction '{name}' started (group {group}).",
                new
                {
                    active = true,
                    group,
                    name,
                });
        }

        static object Commit(ToolParams p)
        {
            if (!s_ActiveGroup.HasValue)
            {
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    "No active transaction. Call `tx begin` first.");
            }

            var group = s_ActiveGroup.Value;
            var originalName = s_ActiveName;
            var overrideName = p.Get("name");
            var finalName = !string.IsNullOrEmpty(overrideName) ? overrideName : originalName;

            // CollapseUndoOperations merges every sub-group from `group`
            // upward into one group indexed at `group`. A single Ctrl+Z in
            // the Editor will then reverse the whole transaction.
            Undo.CollapseUndoOperations(group);

            // Re-label the collapsed group so the Edit → Undo menu reads
            // "Undo <finalName>" rather than inheriting the last mutation's
            // sub-group name.
            Undo.SetCurrentGroupName(finalName);

            var started = s_ActiveStarted;
            s_ActiveGroup = null;
            s_ActiveName = null;
            s_ActiveStarted = null;

            return new SuccessResponse(
                $"Transaction '{finalName}' committed.",
                new
                {
                    committed = true,
                    group,
                    name = finalName,
                    duration_ms = DurationMsSince(started),
                });
        }

        static object Rollback()
        {
            if (!s_ActiveGroup.HasValue)
            {
                return new ErrorResponse(ErrorCodes.InvalidParams,
                    "No active transaction. Call `tx begin` first.");
            }

            var group = s_ActiveGroup.Value;
            var name = s_ActiveName;

            // RevertAllDownToGroup performs the Undo stack replay back to the
            // state captured at `begin`. Every mutation made since is
            // reversed as if the caller had pressed Ctrl+Z until the pre-tx
            // state was restored.
            Undo.RevertAllDownToGroup(group);

            var started = s_ActiveStarted;
            s_ActiveGroup = null;
            s_ActiveName = null;
            s_ActiveStarted = null;

            return new SuccessResponse(
                $"Transaction '{name}' rolled back.",
                new
                {
                    rolled_back = true,
                    group,
                    name,
                    duration_ms = DurationMsSince(started),
                });
        }

        static object Status()
        {
            if (!s_ActiveGroup.HasValue)
            {
                return new SuccessResponse(
                    "No active transaction.",
                    new
                    {
                        active = false,
                    });
            }

            return new SuccessResponse(
                $"Transaction '{s_ActiveName}' active (group {s_ActiveGroup.Value}).",
                new
                {
                    active = true,
                    group = s_ActiveGroup.Value,
                    name = s_ActiveName,
                    duration_ms = DurationMsSince(s_ActiveStarted),
                });
        }

        static long DurationMsSince(System.DateTime? start)
        {
            if (!start.HasValue) return 0;
            return (long)(System.DateTime.UtcNow - start.Value).TotalMilliseconds;
        }

        static string FormatAge(System.DateTime? start)
        {
            if (!start.HasValue) return "unknown";
            var ms = (System.DateTime.UtcNow - start.Value).TotalMilliseconds;
            if (ms < 1000) return (long)ms + "ms";
            var s = ms / 1000.0;
            if (s < 60) return s.ToString("F1") + "s";
            return ((long)(s / 60)) + "m" + ((long)(s % 60)) + "s";
        }
    }
}
