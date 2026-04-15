package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAbsolutizePath_Empty(t *testing.T) {
	if got := absolutizePath(""); got != "" {
		t.Errorf("empty input should pass through, got %q", got)
	}
}

func TestAbsolutizePath_AlreadyAbsolute(t *testing.T) {
	abs, err := filepath.Abs(filepath.Join(t.TempDir(), "x.txt"))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	if got := absolutizePath(abs); got != abs {
		t.Errorf("absolute input should be unchanged: got %q want %q", got, abs)
	}
}

func TestAbsolutizePath_Relative(t *testing.T) {
	got := absolutizePath("foo/bar.xml")
	if !filepath.IsAbs(got) {
		t.Errorf("relative input should become absolute, got %q", got)
	}
	if !strings.HasSuffix(filepath.ToSlash(got), "foo/bar.xml") {
		t.Errorf("result should retain the original tail, got %q", got)
	}
}

func TestAbsolutizePathParam_MissingKey(t *testing.T) {
	params := map[string]interface{}{"mode": "EditMode"}
	absolutizePathParam(params, "output")
	if _, set := params["output"]; set {
		t.Error("missing key should not be added")
	}
}

func TestAbsolutizePathParam_EmptyString(t *testing.T) {
	params := map[string]interface{}{"output": ""}
	absolutizePathParam(params, "output")
	if params["output"] != "" {
		t.Errorf("empty string should stay empty, got %v", params["output"])
	}
}

func TestAbsolutizePathParam_NonString(t *testing.T) {
	// A caller could (wrongly) put a number or map under this key; the
	// helper must leave it alone rather than panicking or rewriting.
	params := map[string]interface{}{"output": 42}
	absolutizePathParam(params, "output")
	if params["output"] != 42 {
		t.Errorf("non-string value should be untouched, got %v", params["output"])
	}
}

func TestAbsolutizePathParam_RelativeString(t *testing.T) {
	params := map[string]interface{}{"output_path": "captures/x.png"}
	absolutizePathParam(params, "output_path")
	out, ok := params["output_path"].(string)
	if !ok {
		t.Fatalf("expected string, got %T", params["output_path"])
	}
	if !filepath.IsAbs(out) {
		t.Errorf("expected absolute path, got %q", out)
	}
	if !strings.HasSuffix(filepath.ToSlash(out), "captures/x.png") {
		t.Errorf("expected tail to match input, got %q", out)
	}
}
