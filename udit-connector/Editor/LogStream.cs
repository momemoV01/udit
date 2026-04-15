using System;
using System.Collections.Concurrent;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Net;
using System.Text;
using Newtonsoft.Json;
using UnityEditor;
using UnityEngine;

namespace UditConnector
{
    /// <summary>
    /// SSE streaming layer for <c>udit log tail</c>. Subscribes once to
    /// <see cref="Application.logMessageReceived"/> (main thread) and
    /// fans captured events out to every connected HTTP client. Design
    /// rationale + wire format: plan humble-baking-peacock.md D3/D4/D-extra.
    ///
    /// Ownership model:
    /// <list type="bullet">
    ///   <item>Ring buffer holds the last <see cref="BufferCapacity"/>
    ///     events for <c>--since</c> backfill.</item>
    ///   <item>Active clients live in a <see cref="ConcurrentDictionary{Guid,ClientContext}"/>.
    ///     Each has its own cursor + filter; drain runs on
    ///     <see cref="EditorApplication.update"/>.</item>
    ///   <item>HttpServer owns lifecycle: LogStream does NOT register its
    ///     own AssemblyReload hooks. HttpServer calls
    ///     <see cref="OnBeforeReload"/> / <see cref="OnAfterReload"/>
    ///     explicitly so teardown ordering is deterministic.</item>
    /// </list>
    /// </summary>
    [InitializeOnLoad]
    public static class LogStream
    {
        /// <summary>Ring-buffer depth. Generous enough for 5-10 minutes of
        /// chatty editor output; drops oldest on overflow.</summary>
        public const int BufferCapacity = 2000;

        /// <summary>Cap per-client writes per drain tick to avoid freezing
        /// the Editor during a log storm.</summary>
        public const int MaxEventsPerTick = 500;

        /// <summary>Keep-alive ping cadence — must beat typical proxy
        /// idle timeouts.</summary>
        static readonly TimeSpan KeepAliveInterval = TimeSpan.FromSeconds(30);

        static readonly ConcurrentQueue<BufferedEvent> s_Buffer = new();
        static long s_Sequence; // monotonic; acts as cursor across ring buffer
        static long s_DroppedCount;

        static readonly ConcurrentDictionary<Guid, ClientContext> s_Clients = new();

        // Re-entrance guard. `Application.logMessageReceived` fires on main
        // thread; LogStream drain runs on main thread too. Still, if anything
        // under our control ever logs from inside OnLogMessage / drain, we
        // must not recurse infinitely. Belt-and-suspenders against a future
        // refactor.
        [ThreadStatic] static bool t_InCallback;

        static LogStream()
        {
            Subscribe();
            EditorApplication.update += DrainClients;
        }

        static void Subscribe()
        {
            Application.logMessageReceived -= OnLogMessage; // idempotent
            Application.logMessageReceived += OnLogMessage;
        }

        // ------------------------------------------------------------------
        // Capture
        // ------------------------------------------------------------------

        static void OnLogMessage(string condition, string stackTrace, LogType type)
        {
            if (t_InCallback) return;
            t_InCallback = true;
            try
            {
                var ev = BuildEvent(condition, stackTrace, type);
                s_Buffer.Enqueue(new BufferedEvent
                {
                    Sequence = System.Threading.Interlocked.Increment(ref s_Sequence),
                    Event = ev,
                });
                TrimBuffer();
            }
            catch
            {
                // Never throw out of a logMessageReceived handler — would cascade.
            }
            finally
            {
                t_InCallback = false;
            }
        }

        static LogEvent BuildEvent(string condition, string stackTrace, LogType type)
        {
            var (file, line) = ExtractFileLine(stackTrace);
            return new LogEvent
            {
                TimestampMs = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds(),
                Type = type.ToString(),
                Message = TruncateString(condition ?? string.Empty),
                Stack = string.IsNullOrEmpty(stackTrace) ? null : TruncateString(stackTrace),
                File = file,
                Line = line,
            };
        }

        static string TruncateString(string s)
        {
            if (s == null) return null;
            var bytes = Encoding.UTF8.GetByteCount(s);
            if (bytes <= LogEvent.MaxFieldBytes) return s;
            // Truncate by characters (approximate) then append suffix. Fine for
            // diagnostics; we don't promise byte-exact cut since UTF-8 is
            // variable-width.
            var ratio = (double)LogEvent.MaxFieldBytes / bytes;
            var take = Math.Max(0, (int)(s.Length * ratio) - LogEvent.TruncationSuffix.Length);
            return s.Substring(0, take) + LogEvent.TruncationSuffix;
        }

        static (string file, int line) ExtractFileLine(string stackTrace)
        {
            if (string.IsNullOrEmpty(stackTrace)) return (null, 0);
            // Unity emits two stack-trace formats:
            //   Editor / managed:   "(at Assets/Scripts/Foo.cs:42)"
            //   IL2CPP / Mono:      "  at Foo.Bar () [0x00024] in Foo.cs:42"
            // Try the "(at path:N)" form first (more common in Editor logs),
            // then fall back to " in path:N ".
            if (TryExtract(stackTrace, "(at ", ")", out var f, out var l)) return (f, l);
            if (TryExtract(stackTrace, " in ", "\n", out f, out l)) return (f, l);
            return (null, 0);
        }

        static bool TryExtract(string stack, string openMarker, string closeMarker,
            out string file, out int line)
        {
            file = null;
            line = 0;
            var idx = stack.IndexOf(openMarker, StringComparison.Ordinal);
            while (idx >= 0)
            {
                var start = idx + openMarker.Length;
                var segEnd = stack.IndexOf(closeMarker, start, StringComparison.Ordinal);
                if (segEnd < 0) segEnd = stack.Length;
                var segLen = segEnd - start;
                if (segLen > 1)
                {
                    // Rightmost ':' separates path from line number. Windows
                    // absolute paths (C:\…) have an earlier ':' too; LastIndexOf
                    // finds the line-number colon correctly.
                    var colon = stack.LastIndexOf(':', segEnd - 1, segLen);
                    if (colon > start &&
                        int.TryParse(stack.Substring(colon + 1, segEnd - colon - 1).TrimEnd(), out var ln))
                    {
                        var path = stack.Substring(start, colon - start).Trim();
                        if (!string.IsNullOrEmpty(path))
                        {
                            file = path;
                            line = ln;
                            return true;
                        }
                    }
                }
                idx = stack.IndexOf(openMarker, segEnd, StringComparison.Ordinal);
            }
            return false;
        }

        static void TrimBuffer()
        {
            while (s_Buffer.Count > BufferCapacity)
            {
                if (s_Buffer.TryDequeue(out _))
                {
                    System.Threading.Interlocked.Increment(ref s_DroppedCount);
                }
                else
                {
                    break;
                }
            }
        }

        // ------------------------------------------------------------------
        // Client attach (called by HttpServer when a GET /logs/stream arrives)
        // ------------------------------------------------------------------

        public static void Attach(HttpListenerContext context)
        {
            // Parse filters from the query string. Reject unknowns with a
            // 400 response (UCI-006 InvalidStreamFilter) — cheaper to surface
            // as HTTP error than to silently drop mismatches.
            var q = context.Request.QueryString;
            if (!TryParseFilter(q, out var filter, out var err))
            {
                WriteError(context.Response, 400, "UCI-006", err);
                context.Response.Close();
                return;
            }

            var resp = context.Response;
            resp.ContentType = "text/event-stream";
            resp.Headers["Cache-Control"] = "no-cache";
            resp.Headers["X-Accel-Buffering"] = "no"; // hint any reverse proxy to not buffer
            resp.SendChunked = true;

            var ctx = new ClientContext
            {
                Id = Guid.NewGuid(),
                Response = resp,
                Filter = filter,
                Cursor = SnapshotCursorFor(filter),
                LastWriteUtc = DateTime.UtcNow,
            };

            s_Clients[ctx.Id] = ctx;
        }

        /// <summary>Determine the starting cursor based on --since backfill rules (plan D3).</summary>
        static long SnapshotCursorFor(StreamFilter filter)
        {
            if (filter.SinceMs <= 0)
            {
                // Live-only; start after the current tail.
                return System.Threading.Interlocked.Read(ref s_Sequence);
            }

            var nowMs = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds();
            var sinceCutoff = nowMs - filter.SinceMs;

            long earliestMatch = long.MaxValue;
            long latestBefore = -1;
            bool anyInBuffer = false;

            foreach (var b in s_Buffer) // snapshot iteration; value type doesn't mutate
            {
                anyInBuffer = true;
                if (b.Event.TimestampMs >= sinceCutoff && b.Sequence < earliestMatch)
                {
                    earliestMatch = b.Sequence;
                }
                else if (b.Event.TimestampMs < sinceCutoff && b.Sequence > latestBefore)
                {
                    latestBefore = b.Sequence;
                }
            }

            if (!anyInBuffer)
            {
                return 0; // empty buffer — case 4
            }

            if (earliestMatch == long.MaxValue)
            {
                // Buffer exists but all entries older than cutoff — nothing to replay.
                return System.Threading.Interlocked.Read(ref s_Sequence);
            }

            // earliestMatch is inclusive; cursor is "last emitted sequence"
            // so we start one below to include it on first drain.
            return earliestMatch - 1;
        }

        // ------------------------------------------------------------------
        // Drain (EditorApplication.update tick)
        // ------------------------------------------------------------------

        static void DrainClients()
        {
            if (s_Clients.IsEmpty) return;

            // Snapshot current buffer once for this tick.
            var snapshot = s_Buffer.ToArray();
            var dropped = System.Threading.Interlocked.Read(ref s_DroppedCount);
            var nowUtc = DateTime.UtcNow;

            var dead = new List<Guid>();

            // Iterate snapshot of keys — safe against concurrent Attach/remove.
            foreach (var id in s_Clients.Keys.ToArray())
            {
                if (!s_Clients.TryGetValue(id, out var client)) continue;

                var wroteAny = false;
                var written = 0;
                var failed = false;

                // Truncation marker — only once, right after attach.
                if (!client.SentTruncationMarker && client.Filter.SinceMs > 0)
                {
                    var (requested, available) = ComputeTruncation(snapshot, client.Filter.SinceMs);
                    if (requested != available)
                    {
                        try
                        {
                            WriteFrame(client.Response, "truncated",
                                $"{{\"t\":{DateTimeOffset.UtcNow.ToUnixTimeMilliseconds()}," +
                                $"\"requested_ms\":{requested},\"available_ms\":{available}}}");
                            wroteAny = true;
                        }
                        catch
                        {
                            failed = true;
                        }
                    }
                    client.SentTruncationMarker = true;
                }

                // Dropped marker — emit when the client's last-seen dropped count
                // lags the global counter.
                if (!failed && dropped > client.SeenDroppedCount)
                {
                    var delta = dropped - client.SeenDroppedCount;
                    try
                    {
                        WriteFrame(client.Response, "dropped",
                            $"{{\"t\":{DateTimeOffset.UtcNow.ToUnixTimeMilliseconds()}," +
                            $"\"count\":{delta}}}");
                        wroteAny = true;
                    }
                    catch
                    {
                        failed = true;
                    }
                    client.SeenDroppedCount = dropped;
                }

                if (!failed)
                {
                    foreach (var buffered in snapshot)
                    {
                        if (buffered.Sequence <= client.Cursor) continue;
                        if (!client.Filter.Accepts(buffered.Event)) { client.Cursor = buffered.Sequence; continue; }
                        if (written >= MaxEventsPerTick) break;

                        var projected = ApplyStackMode(buffered.Event, client.Filter.StackMode);
                        string json;
                        try
                        {
                            json = JsonConvert.SerializeObject(projected);
                        }
                        catch
                        {
                            // Serialization failure — skip this event rather than tearing down the stream.
                            client.Cursor = buffered.Sequence;
                            continue;
                        }

                        try
                        {
                            WriteFrame(client.Response, "log", json);
                            wroteAny = true;
                            written++;
                            client.Cursor = buffered.Sequence;
                        }
                        catch (IOException) { failed = true; break; }
                        catch (HttpListenerException) { failed = true; break; }
                        catch (ObjectDisposedException) { failed = true; break; }
                        catch (InvalidOperationException) { failed = true; break; }
                    }
                }

                // Keep-alive ping if idle for too long.
                if (!failed && !wroteAny && (nowUtc - client.LastWriteUtc) >= KeepAliveInterval)
                {
                    try
                    {
                        WriteKeepAlive(client.Response);
                        wroteAny = true;
                    }
                    catch (IOException) { failed = true; }
                    catch (HttpListenerException) { failed = true; }
                    catch (ObjectDisposedException) { failed = true; }
                    catch (InvalidOperationException) { failed = true; }
                }

                if (wroteAny) client.LastWriteUtc = nowUtc;
                if (failed) dead.Add(id);
            }

            foreach (var id in dead)
            {
                if (s_Clients.TryRemove(id, out var c))
                {
                    try { c.Response.Close(); } catch { }
                }
            }
        }

        static (long requested, long available) ComputeTruncation(BufferedEvent[] snapshot, long sinceMs)
        {
            if (snapshot.Length == 0) return (sinceMs, 0);
            var nowMs = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds();
            var cutoff = nowMs - sinceMs;
            var oldest = long.MaxValue;
            foreach (var b in snapshot)
            {
                if (b.Event.TimestampMs < oldest) oldest = b.Event.TimestampMs;
            }
            if (oldest <= cutoff) return (sinceMs, sinceMs); // full window covered
            var available = nowMs - oldest;
            return (sinceMs, available);
        }

        static LogEvent ApplyStackMode(LogEvent src, StackMode mode)
        {
            return mode switch
            {
                StackMode.None => new LogEvent
                {
                    TimestampMs = src.TimestampMs,
                    Type = src.Type,
                    Message = src.Message,
                    Stack = null,
                    File = src.File,
                    Line = src.Line,
                },
                StackMode.User => new LogEvent
                {
                    TimestampMs = src.TimestampMs,
                    Type = src.Type,
                    Message = src.Message,
                    Stack = FilterUserStack(src.Stack),
                    File = src.File,
                    Line = src.Line,
                },
                _ => src, // Full
            };
        }

        static string FilterUserStack(string stack)
        {
            if (string.IsNullOrEmpty(stack)) return null;
            var lines = stack.Split('\n');
            var kept = new List<string>(lines.Length);
            foreach (var raw in lines)
            {
                var line = raw;
                if (line.Contains("UnityEngine.Debug")) continue;
                if (line.Contains("UnityEditor.EditorApplication")) continue;
                if (line.Contains("EditorGUIUtility")) continue;
                if (line.Contains("/Library/")) continue;
                kept.Add(line);
            }
            return kept.Count == 0 ? null : string.Join("\n", kept);
        }

        // ------------------------------------------------------------------
        // Reload ordering (called by HttpServer)
        // ------------------------------------------------------------------

        public static void OnBeforeReload()
        {
            // Flush a reload marker to every client, then close responses so
            // clients get EOF rather than a raw TCP reset.
            foreach (var id in s_Clients.Keys.ToArray())
            {
                if (!s_Clients.TryRemove(id, out var c)) continue;
                try
                {
                    WriteFrame(c.Response, "reload",
                        $"{{\"t\":{DateTimeOffset.UtcNow.ToUnixTimeMilliseconds()}}}");
                }
                catch { }
                try { c.Response.Close(); } catch { }
            }
        }

        public static void OnAfterReload()
        {
            // Static ctor of this class already re-subscribed during reload
            // (InitializeOnLoad fires after assembly reload). Nothing further.
            // Method exists as a hook point in case HttpServer needs to signal
            // us beyond the default static-ctor resubscribe.
        }

        // ------------------------------------------------------------------
        // Frame writers
        // ------------------------------------------------------------------

        static void WriteFrame(HttpListenerResponse resp, string eventName, string dataJson)
        {
            // Explicit UTF-8 encoding — SSE frames must be 8-bit clean.
            var payload = $"event: {eventName}\ndata: {dataJson}\n\n";
            var bytes = Encoding.UTF8.GetBytes(payload);
            resp.OutputStream.Write(bytes, 0, bytes.Length);
            resp.OutputStream.Flush();
        }

        static void WriteKeepAlive(HttpListenerResponse resp)
        {
            var bytes = Encoding.UTF8.GetBytes(": ping\n\n");
            resp.OutputStream.Write(bytes, 0, bytes.Length);
            resp.OutputStream.Flush();
        }

        static void WriteError(HttpListenerResponse resp, int status, string code, string message)
        {
            resp.ContentType = "application/json";
            resp.StatusCode = status;
            var body = $"{{\"success\":false,\"error_code\":\"{code}\",\"message\":\"{EscapeJson(message)}\"}}";
            var bytes = Encoding.UTF8.GetBytes(body);
            try
            {
                resp.ContentLength64 = bytes.Length;
                resp.OutputStream.Write(bytes, 0, bytes.Length);
            }
            catch { }
        }

        static string EscapeJson(string s) =>
            (s ?? string.Empty).Replace("\\", "\\\\").Replace("\"", "\\\"");

        // ------------------------------------------------------------------
        // Filter parsing
        // ------------------------------------------------------------------

        static bool TryParseFilter(System.Collections.Specialized.NameValueCollection q,
            out StreamFilter filter, out string error)
        {
            filter = StreamFilter.Default;
            error = null;

            var typesRaw = q["types"];
            if (!string.IsNullOrEmpty(typesRaw))
            {
                var mask = LogTypeMask.None;
                foreach (var t in typesRaw.Split(','))
                {
                    switch (t.Trim().ToLowerInvariant())
                    {
                        case "error": mask |= LogTypeMask.Error; break;
                        case "warning": mask |= LogTypeMask.Warning; break;
                        case "log": mask |= LogTypeMask.Log; break;
                        case "assert": mask |= LogTypeMask.Assert; break;
                        case "exception": mask |= LogTypeMask.Exception; break;
                        default:
                            error = $"Unknown type value '{t}'; accepted: error, warning, log, assert, exception";
                            return false;
                    }
                }
                filter.TypeMask = mask;
            }

            var stackRaw = q["stacktrace"];
            if (!string.IsNullOrEmpty(stackRaw))
            {
                switch (stackRaw.Trim().ToLowerInvariant())
                {
                    case "none": filter.StackMode = StackMode.None; break;
                    case "user": filter.StackMode = StackMode.User; break;
                    case "full": filter.StackMode = StackMode.Full; break;
                    default:
                        error = $"Unknown stacktrace value '{stackRaw}'; accepted: none, user, full";
                        return false;
                }
            }

            var sinceRaw = q["since_ms"];
            if (!string.IsNullOrEmpty(sinceRaw))
            {
                if (!long.TryParse(sinceRaw, out var sinceMs) || sinceMs < 0)
                {
                    error = $"Invalid since_ms value '{sinceRaw}'; must be a non-negative integer";
                    return false;
                }
                filter.SinceMs = sinceMs;
            }

            return true;
        }

        // ------------------------------------------------------------------
        // Internal types
        // ------------------------------------------------------------------

        struct BufferedEvent
        {
            public long Sequence;
            public LogEvent Event;
        }

        class ClientContext
        {
            public Guid Id;
            public HttpListenerResponse Response;
            public StreamFilter Filter;
            public long Cursor;                 // last written Sequence
            public DateTime LastWriteUtc;
            public bool SentTruncationMarker;
            public long SeenDroppedCount;
        }

        [Flags]
        enum LogTypeMask
        {
            None = 0,
            Error = 1 << 0,
            Warning = 1 << 1,
            Log = 1 << 2,
            Assert = 1 << 3,
            Exception = 1 << 4,
            All = Error | Warning | Log | Assert | Exception,
        }

        enum StackMode { None, User, Full }

        struct StreamFilter
        {
            public LogTypeMask TypeMask;
            public StackMode StackMode;
            public long SinceMs;

            public static StreamFilter Default => new StreamFilter
            {
                TypeMask = LogTypeMask.All,
                StackMode = StackMode.User,
                SinceMs = 0,
            };

            public bool Accepts(LogEvent ev)
            {
                return (TypeMask & LogTypeToMask(ev.Type)) != 0;
            }

            static LogTypeMask LogTypeToMask(string t) => t switch
            {
                "Error" => LogTypeMask.Error,
                "Warning" => LogTypeMask.Warning,
                "Log" => LogTypeMask.Log,
                "Assert" => LogTypeMask.Assert,
                "Exception" => LogTypeMask.Exception,
                _ => LogTypeMask.None,
            };
        }
    }
}
