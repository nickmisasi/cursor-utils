package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCatalogCommandsInjectAPIKey(t *testing.T) {
	tests := []struct {
		command string
		method  string
	}{
		{command: "me", method: "Me"},
		{command: "models", method: "ListModels"},
		{command: "repos", method: "ListRepositories"},
	}
	for _, test := range tests {
		t.Run(test.command, func(t *testing.T) {
			var got map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				wantPath := "/sdk.v1.SdkCursorService/" + test.method
				if request.URL.Path != wantPath {
					t.Errorf("path = %q, want %q", request.URL.Path, wantPath)
				}
				if err := json.NewDecoder(request.Body).Decode(&got); err != nil {
					t.Errorf("decode request: %v", err)
				}
				io.WriteString(writer, `{}`)
			}))
			defer server.Close()
			root, _, _ := newTestRoot(server)
			root.SetArgs([]string{test.command, "--json", `{}`})
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			options, ok := got["options"].(map[string]any)
			if !ok || options["apiKey"] != "test-api-key" {
				t.Fatalf("request = %#v", got)
			}
		})
	}
}
