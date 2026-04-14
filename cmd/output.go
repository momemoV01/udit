package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/momemoV01/udit/internal/client"
)

// jsonOutput is the wire format for `--json` mode. It always carries the
// command name + a stable error_code (when applicable) so agents can branch
// without parsing English message text. The Unity meta block reflects what
// the CLI knew at the moment it wrote the response — useful for "what
// project did this run against?" without a separate `udit status` round-trip.
type jsonOutput struct {
	Success   bool            `json:"success"`
	Command   string          `json:"command"`
	Message   string          `json:"message,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	ErrorCode string          `json:"error_code,omitempty"`
	Unity     *unityMeta      `json:"unity,omitempty"`
}

type unityMeta struct {
	Port    int    `json:"port"`
	Project string `json:"project,omitempty"`
	State   string `json:"state,omitempty"`
	Version string `json:"version,omitempty"`
}

func instMeta(inst *client.Instance) *unityMeta {
	if inst == nil {
		return nil
	}
	return &unityMeta{
		Port:    inst.Port,
		Project: inst.ProjectPath,
		State:   inst.State,
		Version: inst.UnityVersion,
	}
}

// classifyGoError maps common Go-side failures to the UCI code registry
// (Phase 1.3) so agents can decide whether to retry. Returns "" when the
// error doesn't match a known pattern (caller may then fall back to
// "UCI-999" or leave the field empty).
//
// Pattern matching is on error text rather than typed errors because the
// surrounding code (net/http, internal/client) returns plain wrapped
// strings; switching every callsite to typed errors is a separate cleanup
// not in this commit's scope.
func classifyGoError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "no Unity instances"),
		strings.Contains(msg, "no instance on port"),
		strings.Contains(msg, "no active instance on port"),
		strings.Contains(msg, "no status for port"),
		strings.Contains(msg, "no Unity instance found for project"):
		return "UCI-001"
	case strings.Contains(msg, "cannot connect to Unity"):
		return "UCI-002"
	case strings.Contains(msg, "Client.Timeout exceeded"),
		strings.Contains(msg, "timed out waiting"),
		strings.Contains(msg, "context deadline exceeded"):
		return "UCI-003"
	}
	return ""
}

// emitJSONResponse writes a Connector CommandResponse as a uniform JSON
// envelope. Success goes to stdout, failure to stderr, exit code is the
// caller's responsibility.
func emitJSONResponse(resp *client.CommandResponse, command string, inst *client.Instance) {
	out := jsonOutput{
		Success:   resp.Success,
		Command:   command,
		Message:   resp.Message,
		Data:      resp.Data,
		ErrorCode: resp.ErrorCode,
		Unity:     instMeta(inst),
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	if resp.Success {
		fmt.Println(string(b))
	} else {
		fmt.Fprintln(os.Stderr, string(b))
	}
}

// emitJSONError writes a CLI-side failure (no Unity response was received)
// as the same envelope. Always to stderr.
func emitJSONError(code, message, command string, inst *client.Instance) {
	out := jsonOutput{
		Success:   false,
		Command:   command,
		Message:   message,
		ErrorCode: code,
		Unity:     instMeta(inst),
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(os.Stderr, string(b))
}

// emitTextError writes a plain "Error: ..." line to stderr (legacy mode).
func emitTextError(message string) {
	fmt.Fprintf(os.Stderr, "Error: %s\n", message)
}

// reportError dispatches to the right emitter based on the global --json flag.
func reportError(err error, command string, inst *client.Instance, useJSON bool) {
	if err == nil {
		return
	}
	if useJSON {
		code := classifyGoError(err)
		emitJSONError(code, err.Error(), command, inst)
	} else {
		emitTextError(err.Error())
	}
}
