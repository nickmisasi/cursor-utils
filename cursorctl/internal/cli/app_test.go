package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nickmisasi/cursor-utils/cursorctl/internal/auth"
)

func TestResolveAPIKey(t *testing.T) {
	isolateAuthConfig(t)
	t.Setenv("CUSTOM_CURSOR_KEY", "from-env")
	app := &App{APIKeyEnv: "CUSTOM_CURSOR_KEY"}
	key, err := app.ResolvedAPIKey()
	if err != nil || key != "from-env" {
		t.Fatalf("resolveAPIKey() = %q, %v", key, err)
	}

	app.APIKey = "explicit"
	key, err = app.ResolvedAPIKey()
	if err != nil || key != "explicit" {
		t.Fatalf("resolveAPIKey() explicit = %q, %v", key, err)
	}
}

func TestResolveAPIKeyErrorNamesEnvironmentVariable(t *testing.T) {
	isolateAuthConfig(t)
	t.Setenv("MISSING_CURSOR_KEY", "")
	app := &App{APIKeyEnv: "MISSING_CURSOR_KEY"}
	_, err := app.ResolvedAPIKey()
	if err == nil ||
		!strings.Contains(err.Error(), "MISSING_CURSOR_KEY") ||
		!strings.Contains(err.Error(), "cursorctl auth add") {
		t.Fatalf("resolveAPIKey() error = %v", err)
	}
}

func TestResolveAPIKeyPrefersExplicitThenEnvThenProfileThenDefault(t *testing.T) {
	isolateAuthConfig(t)
	if err := auth.Add("work", "cursor_work_key_aaaa", true, false); err != nil {
		t.Fatal(err)
	}
	if err := auth.Add("personal", "cursor_personal_bbbb", false, false); err != nil {
		t.Fatal(err)
	}

	app := &App{APIKeyEnv: "CUSTOM_CURSOR_KEY"}
	key, err := app.ResolvedAPIKey()
	if err != nil || key != "cursor_work_key_aaaa" {
		t.Fatalf("default profile = %q, %v", key, err)
	}

	app.Profile = "personal"
	key, err = app.ResolvedAPIKey()
	if err != nil || key != "cursor_personal_bbbb" {
		t.Fatalf("named profile = %q, %v", key, err)
	}

	t.Setenv(auth.ProfileEnv, "personal")
	app.Profile = ""
	key, err = app.ResolvedAPIKey()
	if err != nil || key != "cursor_personal_bbbb" {
		t.Fatalf("CURSORCTL_PROFILE = %q, %v", key, err)
	}

	t.Setenv("CUSTOM_CURSOR_KEY", "from-env")
	key, err = app.ResolvedAPIKey()
	if err != nil || key != "from-env" {
		t.Fatalf("env over profile = %q, %v", key, err)
	}

	app.APIKey = "explicit"
	key, err = app.ResolvedAPIKey()
	if err != nil || key != "explicit" {
		t.Fatalf("explicit over env = %q, %v", key, err)
	}
}

func TestResolveAPIKeyUnknownProfile(t *testing.T) {
	isolateAuthConfig(t)
	app := &App{APIKeyEnv: "MISSING_CURSOR_KEY", Profile: "missing"}
	_, err := app.ResolvedAPIKey()
	if err == nil || !strings.Contains(err.Error(), `unknown auth profile "missing"`) {
		t.Fatalf("unknown profile error = %v", err)
	}
}

func TestCatalogInjectsProfileAPIKey(t *testing.T) {
	isolateAuthConfig(t)
	if err := auth.Add("work", "cursor_profile_key_zzzz", true, false); err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		io.WriteString(writer, `{}`)
	}))
	defer server.Close()

	root, app, _ := newTestRoot(server)
	app.APIKey = ""
	app.APIKeyEnv = "MISSING_CURSOR_KEY"
	root.SetArgs([]string{"me", "--json", `{}`})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	options, ok := got["options"].(map[string]any)
	if !ok || options["apiKey"] != "cursor_profile_key_zzzz" {
		t.Fatalf("request = %#v", got)
	}
}

func TestCatalogUsesProfileFlag(t *testing.T) {
	isolateAuthConfig(t)
	if err := auth.Add("work", "cursor_work_key_aaaa", true, false); err != nil {
		t.Fatal(err)
	}
	if err := auth.Add("personal", "cursor_personal_bbbb", false, false); err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		io.WriteString(writer, `{}`)
	}))
	defer server.Close()

	root, app, _ := newTestRoot(server)
	app.APIKey = ""
	app.APIKeyEnv = "MISSING_CURSOR_KEY"
	root.SetArgs([]string{"--profile", "personal", "me", "--json", `{}`})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	options, ok := got["options"].(map[string]any)
	if !ok || options["apiKey"] != "cursor_personal_bbbb" {
		t.Fatalf("request = %#v", got)
	}
}
