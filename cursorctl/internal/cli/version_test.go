package cli

import (
	"bytes"
	"encoding/json"
	"runtime"
	"testing"

	"github.com/nickmisasi/cursor-utils/cursorctl/internal/bridge"
)

func TestVersionOutputShape(t *testing.T) {
	original := Version
	Version = "test-version"
	defer func() {
		Version = original
	}()

	root, app := NewRootCommand()
	var output bytes.Buffer
	app.Out = &output
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 ||
		got["version"] != "test-version" ||
		got["bridgeVersion"] != bridge.DefaultVersion ||
		got["goVersion"] != runtime.Version() {
		t.Fatalf("output = %#v", got)
	}
}
