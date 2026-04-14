using Newtonsoft.Json;

namespace UditConnector
{
    /// <summary>
    /// Standard udit error codes. Used by ErrorResponse.errorCode so agents can
    /// branch on a stable identifier instead of fragile message-text matching.
    /// Mirror these in docs/ERROR_CODES.md whenever you add or repurpose one.
    /// </summary>
    public static class ErrorCodes
    {
        // 0xx — connectivity / discovery (mostly emitted by the Go CLI)
        public const string NoUnityRunning  = "UCI-001";
        public const string ConnectionRefused = "UCI-002";
        public const string CommandTimeout  = "UCI-003";

        // 01x — request / dispatch
        public const string UnknownCommand  = "UCI-010";
        public const string InvalidParams   = "UCI-011";

        // 02x — Unity busy
        public const string UnityCompiling  = "UCI-020";
        public const string UnityUpdating   = "UCI-021";

        // 03x — exec
        public const string ExecCompileError = "UCI-030";
        public const string ExecRuntimeError = "UCI-031";

        // 04x — asset/scene/GO/component lookup (Phase 2 Observe)
        public const string AssetNotFound      = "UCI-040";
        public const string SceneNotFound      = "UCI-041";
        public const string GameObjectNotFound = "UCI-042";
        public const string ComponentNotFound  = "UCI-043";

        // 99x — generic fallback
        public const string Unknown         = "UCI-999";
    }

    public class SuccessResponse
    {
        public bool success = true;
        public string message;
        public object data;

        public SuccessResponse(string message, object data = null)
        {
            this.message = message;
            this.data = data;
        }
    }

    public class ErrorResponse
    {
        public bool success = false;
        public string message;

        // JSON serializes as "error_code" to match the agent-friendly snake_case
        // convention used elsewhere in the response schema. Null when the error
        // came from a path we haven't classified yet.
        [JsonProperty("error_code", NullValueHandling = NullValueHandling.Ignore)]
        public string errorCode;

        public object data;

        public ErrorResponse(string message, object data = null)
        {
            this.message = message;
            this.data = data;
        }

        public ErrorResponse(string errorCode, string message, object data = null)
        {
            this.errorCode = errorCode;
            this.message = message;
            this.data = data;
        }
    }
}
