package bridge

import (
	"strings"
	"testing"
)

func TestParseHandshake(t *testing.T) {
	input := `{
		"schemaVersion": 1,
		"serverVersion": "1.0.27",
		"transport": "tcp",
		"protocol": "connect",
		"url": "http://127.0.0.1:1234",
		"authTokenFile": "/tmp/token",
		"unknown": "ignored"
	}`
	handshake, err := parseHandshake(input)
	if err != nil {
		t.Fatalf("parseHandshake() error = %v", err)
	}
	if handshake.ServerVersion != "1.0.27" ||
		handshake.URL != "http://127.0.0.1:1234" ||
		handshake.AuthTokenFile != "/tmp/token" {
		t.Fatalf("handshake = %#v", handshake)
	}
}

func TestParseHandshakeValidation(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "malformed JSON",
			input: `{`,
			want:  "parse bridge ready handshake",
		},
		{
			name:  "schema",
			input: `{"schemaVersion":2,"transport":"tcp","protocol":"connect","url":"x","authTokenFile":"x"}`,
			want:  "schema version",
		},
		{
			name:  "transport",
			input: `{"schemaVersion":1,"transport":"stdio","protocol":"connect","url":"x","authTokenFile":"x"}`,
			want:  "transport",
		},
		{
			name:  "protocol",
			input: `{"schemaVersion":1,"transport":"tcp","protocol":"grpc","url":"x","authTokenFile":"x"}`,
			want:  "protocol",
		},
		{
			name:  "URL",
			input: `{"schemaVersion":1,"transport":"tcp","protocol":"connect","authTokenFile":"x"}`,
			want:  "missing url",
		},
		{
			name:  "token file",
			input: `{"schemaVersion":1,"transport":"tcp","protocol":"connect","url":"x"}`,
			want:  "missing authTokenFile",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseHandshake(test.input)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("parseHandshake() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestProcessEnvironmentOverridesCursorValues(t *testing.T) {
	t.Setenv("CURSOR_API_KEY", "old")
	t.Setenv("CURSOR_SDK_CLIENT_LANGUAGE", "typescript")
	environment := processEnvironment("new")

	var apiKey, language int
	for _, entry := range environment {
		switch entry {
		case "CURSOR_API_KEY=new":
			apiKey++
		case "CURSOR_SDK_CLIENT_LANGUAGE=go":
			language++
		case "CURSOR_API_KEY=old", "CURSOR_SDK_CLIENT_LANGUAGE=typescript":
			t.Fatalf("stale environment entry %q", entry)
		}
	}
	if apiKey != 1 || language != 1 {
		t.Fatalf("api key entries = %d, language entries = %d", apiKey, language)
	}
}
