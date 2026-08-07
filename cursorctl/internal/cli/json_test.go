package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadJSONPayloadRaw(t *testing.T) {
	app := &App{In: strings.NewReader("")}
	payload, err := app.ReadJSONPayload(`{"name":"Ada","count":2}`)
	if err != nil {
		t.Fatalf("ReadJSONPayload() error = %v", err)
	}
	if payload["name"] != "Ada" || payload["count"] != json.Number("2") {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestReadJSONPayloadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payload.json")
	if err := os.WriteFile(path, []byte(`{"source":"file"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	app := &App{In: strings.NewReader("")}
	payload, err := app.ReadJSONPayload("@" + path)
	if err != nil {
		t.Fatalf("ReadJSONPayload() error = %v", err)
	}
	if payload["source"] != "file" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestReadJSONPayloadStdin(t *testing.T) {
	app := &App{In: strings.NewReader(`{"source":"stdin"}`)}
	payload, err := app.ReadJSONPayload("-")
	if err != nil {
		t.Fatalf("readJSONPayload() error = %v", err)
	}
	if payload["source"] != "stdin" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestReadJSONPayloadErrors(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "empty file path", value: "@"},
		{name: "invalid JSON", value: "{"},
		{name: "not an object", value: "[]"},
		{name: "null", value: "null"},
		{name: "trailing value", value: `{"ok":true} {}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := &App{In: strings.NewReader("")}
			if _, err := app.ReadJSONPayload(test.value); err == nil {
				t.Fatalf("ReadJSONPayload(%q) error = nil", test.value)
			}
		})
	}
}

func TestDecodeSingleJSONAcceptsAnyValueAndRejectsTrailing(t *testing.T) {
	var value any
	if err := decodeSingleJSON([]byte(`[1,2]`), &value); err != nil {
		t.Fatalf("decodeSingleJSON() error = %v", err)
	}
	items, ok := value.([]any)
	if !ok || len(items) != 2 || items[0] != json.Number("1") {
		t.Fatalf("value = %#v", value)
	}
	if err := decodeSingleJSON([]byte(`true false`), &value); !errors.Is(err, errMultipleJSONValues) {
		t.Fatalf("decodeSingleJSON() error = %v", err)
	}
}
