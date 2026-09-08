package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/nickmisasi/cursor-utils/cursorctl/internal/auth"
	"github.com/spf13/cobra"
)

func TestAuthAddListDefaultRemove(t *testing.T) {
	isolateAuthConfig(t)

	root, _, output := newAuthTestRoot(t, strings.NewReader("cursor_work_key_aaaa\n"))
	root.SetArgs([]string{"auth", "add", "work", "--stdin"})
	if err := root.Execute(); err != nil {
		t.Fatalf("auth add work: %v", err)
	}
	var added auth.Summary
	if err := json.Unmarshal(output.Bytes(), &added); err != nil {
		t.Fatal(err)
	}
	if added.Name != "work" || added.Fingerprint != "…aaaa" || !added.Default {
		t.Fatalf("add output = %#v", added)
	}
	if strings.Contains(output.String(), "cursor_work_key_aaaa") {
		t.Fatal("auth add leaked API key")
	}

	root, _, output = newAuthTestRoot(t, strings.NewReader("cursor_personal_bbbb\n"))
	root.SetArgs([]string{"auth", "add", "personal", "--stdin"})
	if err := root.Execute(); err != nil {
		t.Fatalf("auth add personal: %v", err)
	}
	var personal auth.Summary
	if err := json.Unmarshal(output.Bytes(), &personal); err != nil {
		t.Fatal(err)
	}
	if personal.Default {
		t.Fatal("second profile became default without --default")
	}

	root, _, output = newAuthTestRoot(t, strings.NewReader("cursor_ci_key_cccc\n"))
	root.SetArgs([]string{"auth", "add", "ci", "--stdin", "--default"})
	if err := root.Execute(); err != nil {
		t.Fatalf("auth add ci --default: %v", err)
	}
	var ci auth.Summary
	if err := json.Unmarshal(output.Bytes(), &ci); err != nil {
		t.Fatal(err)
	}
	if !ci.Default || ci.Name != "ci" {
		t.Fatalf("add --default output = %#v", ci)
	}

	root, _, output = newAuthTestRoot(t, nil)
	root.SetArgs([]string{"auth", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("auth list: %v", err)
	}
	var listed auth.ListResult
	if err := json.Unmarshal(output.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "cursor_work_key_aaaa") ||
		strings.Contains(output.String(), "cursor_personal_bbbb") ||
		strings.Contains(output.String(), "cursor_ci_key_cccc") {
		t.Fatalf("auth list leaked API key: %s", output.String())
	}
	if listed.DefaultProfile != "ci" || len(listed.Profiles) != 3 {
		t.Fatalf("list = %#v", listed)
	}
	if listed.Profiles[0].Name != "ci" || listed.Profiles[1].Name != "personal" || listed.Profiles[2].Name != "work" {
		t.Fatalf("list order = %#v", listed.Profiles)
	}

	root, _, output = newAuthTestRoot(t, nil)
	root.SetArgs([]string{"auth", "default", "personal"})
	if err := root.Execute(); err != nil {
		t.Fatalf("auth default personal: %v", err)
	}
	var defaulted authDefaultResult
	if err := json.Unmarshal(output.Bytes(), &defaulted); err != nil {
		t.Fatal(err)
	}
	if defaulted.DefaultProfile != "personal" {
		t.Fatalf("default output = %#v", defaulted)
	}

	root, _, output = newAuthTestRoot(t, nil)
	root.SetArgs([]string{"auth", "remove", "personal"})
	if err := root.Execute(); err != nil {
		t.Fatalf("auth remove: %v", err)
	}
	var removed authRemoveResult
	if err := json.Unmarshal(output.Bytes(), &removed); err != nil {
		t.Fatal(err)
	}
	if !removed.Removed || removed.Name != "personal" {
		t.Fatalf("remove output = %#v", removed)
	}

	root, _, output = newAuthTestRoot(t, nil)
	root.SetArgs([]string{"auth", "default"})
	if err := root.Execute(); err != nil {
		t.Fatalf("auth default: %v", err)
	}
	if err := json.Unmarshal(output.Bytes(), &defaulted); err != nil {
		t.Fatal(err)
	}
	if defaulted.DefaultProfile != "" {
		t.Fatalf("default after removing default profile = %#v", defaulted)
	}
}

func TestAuthAddRequiresForceToReplace(t *testing.T) {
	isolateAuthConfig(t)
	root, _, _ := newAuthTestRoot(t, strings.NewReader("cursor_old_key_zzzz\n"))
	root.SetArgs([]string{"auth", "add", "work", "--stdin"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	root, _, _ = newAuthTestRoot(t, strings.NewReader("cursor_new_key_yyyy\n"))
	root.SetArgs([]string{"auth", "add", "work", "--stdin"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("duplicate add = %v", err)
	}
	root, _, output := newAuthTestRoot(t, strings.NewReader("cursor_new_key_yyyy\n"))
	root.SetArgs([]string{"auth", "add", "work", "--stdin", "--force"})
	if err := root.Execute(); err != nil {
		t.Fatalf("force add: %v", err)
	}
	if strings.Contains(output.String(), "cursor_new_key_yyyy") {
		t.Fatal("force add leaked API key")
	}
	profile, err := auth.Lookup("work")
	if err != nil || profile.APIKey != "cursor_new_key_yyyy" {
		t.Fatalf("Lookup() after force = %#v %v", profile, err)
	}
}

func TestAuthAddWithoutStdinRequiresTerminal(t *testing.T) {
	isolateAuthConfig(t)
	root, _, _ := newAuthTestRoot(t, strings.NewReader("cursor_work_key_aaaa\n"))
	root.SetArgs([]string{"auth", "add", "work"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--stdin or a terminal") {
		t.Fatalf("auth add without stdin = %v", err)
	}
}

func TestAuthAddRejectsInvalidNameBeforeReadingKey(t *testing.T) {
	isolateAuthConfig(t)
	root, _, _ := newAuthTestRoot(t, strings.NewReader("cursor_work_key_aaaa\n"))
	root.SetArgs([]string{"auth", "add", "../etc", "--stdin"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid auth profile name") {
		t.Fatalf("auth add invalid name = %v", err)
	}
}

func newAuthTestRoot(t *testing.T, stdin io.Reader) (*cobra.Command, *App, *bytes.Buffer) {
	t.Helper()
	root, app := NewRootCommand()
	output := &bytes.Buffer{}
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	app.In = stdin
	app.Out = output
	app.Err = io.Discard
	app.APIKey = ""
	app.APIKeyEnv = "MISSING_CURSOR_KEY"
	root.SetIn(stdin)
	root.SetOut(output)
	root.SetErr(io.Discard)
	return root, app, output
}

func isolateAuthConfig(t *testing.T) {
	t.Helper()
	t.Setenv(auth.ConfigDirEnv, t.TempDir())
	t.Setenv(auth.ProfileEnv, "")
	t.Setenv("CURSOR_API_KEY", "")
	t.Setenv("MISSING_CURSOR_KEY", "")
	t.Setenv("CUSTOM_CURSOR_KEY", "")
}
