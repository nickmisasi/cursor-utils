package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBridgeControlCommandsAcceptRawJSONWithoutAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		response string
	}{
		{name: "ping", method: "Ping", response: `{"message":"pong"}`},
		{
			name:     "version",
			method:   "GetVersion",
			response: `{"bridgeVersion":"1.0.27","protocolVersion":"sdk.v1","capabilities":[]}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("MISSING_CURSOR_KEY", "")
			var requestBody map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				wantPath := "/sdk.v1.SdkBridgeControlService/" + test.method
				if request.URL.Path != wantPath {
					t.Errorf("path = %q, want %q", request.URL.Path, wantPath)
				}
				if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
					t.Errorf("decode request: %v", err)
				}
				io.WriteString(writer, test.response)
			}))
			defer server.Close()

			root, app, _ := newTestRoot(server)
			app.APIKey = ""
			app.APIKeyEnv = "MISSING_CURSOR_KEY"
			root.SetArgs([]string{"bridge", test.name, "--json", `{"probe":"raw"}`})
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if requestBody["probe"] != "raw" {
				t.Fatalf("request = %#v", requestBody)
			}
		})
	}
}
