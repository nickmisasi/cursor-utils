package cli

import (
	"strings"
	"testing"
)

func TestResolveAPIKey(t *testing.T) {
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
	t.Setenv("MISSING_CURSOR_KEY", "")
	app := &App{APIKeyEnv: "MISSING_CURSOR_KEY"}
	_, err := app.ResolvedAPIKey()
	if err == nil || !strings.Contains(err.Error(), "MISSING_CURSOR_KEY") {
		t.Fatalf("resolveAPIKey() error = %v", err)
	}
}
