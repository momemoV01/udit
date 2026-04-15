using Newtonsoft.Json;

namespace UditConnector
{
    /// <summary>
    /// One log event captured from Unity's <c>Application.logMessageReceived</c>
    /// and ring-buffered for streaming to SSE clients. Serialized to the
    /// <c>data:</c> field of an <c>event: log</c> SSE frame; see Phase 5.2
    /// design doc (plan humble-baking-peacock.md, D4) for the wire format.
    ///
    /// Fields mirror the shape already used by <see cref="ReadConsole"/> where
    /// sensible so agents see the same vocabulary on both the snapshot
    /// (<c>udit console</c>) and streaming (<c>udit log tail</c>) surfaces.
    /// </summary>
    public struct LogEvent
    {
        /// <summary>Unix milliseconds (UTC) when Unity emitted the log.</summary>
        [JsonProperty("t")]
        public long TimestampMs;

        /// <summary>"Error", "Warning", "Log", "Exception", "Assert" (preserves LogType enum name).</summary>
        [JsonProperty("type")]
        public string Type;

        /// <summary>First line of the message. Capped at MaxFieldBytes with a truncation suffix.</summary>
        [JsonProperty("message")]
        public string Message;

        /// <summary>Remaining lines of the message (stack trace). <c>null</c> when empty or filtered out.</summary>
        [JsonProperty("stack", NullValueHandling = NullValueHandling.Ignore)]
        public string Stack;

        /// <summary>Extracted source file when discoverable from the stack trace. <c>null</c> otherwise.</summary>
        [JsonProperty("file", NullValueHandling = NullValueHandling.Ignore)]
        public string File;

        /// <summary>Extracted source line number when discoverable. 0 otherwise.</summary>
        [JsonProperty("line", NullValueHandling = NullValueHandling.Ignore)]
        public int Line;

        /// <summary>
        /// Maximum bytes we'll serialize for either <see cref="Message"/> or
        /// <see cref="Stack"/>. Keeps individual SSE frames bounded so a
        /// pathological Unity stack trace can't balloon past the client's
        /// scanner buffer (plan D5 sizes that at 128KB total frame).
        /// </summary>
        public const int MaxFieldBytes = 16 * 1024;

        /// <summary>Suffix appended when a field is truncated at MaxFieldBytes.</summary>
        public const string TruncationSuffix = "…[truncated]";
    }
}
