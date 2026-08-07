package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAgentCreateRegistersAndDeclaresCustomTools(t *testing.T) {
	var createRequest map[string]any
	var callbackResult map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/sdk.v1.SdkBridgeControlService/SetToolCallback":
			var registration struct {
				URL       string `json:"url"`
				AuthToken string `json:"authToken"`
			}
			if err := json.NewDecoder(request.Body).Decode(&registration); err != nil {
				t.Errorf("decode registration: %v", err)
			}
			body := bytes.NewBufferString(
				`{"toolName":"lookup","args":{"query":"docs"},"toolCallId":"call-1","agentId":"agent-1"}`,
			)
			callback, err := http.NewRequest(
				http.MethodPost,
				registration.URL+"/sdk.v1.SdkCustomToolCallbackService/CallCustomTool",
				body,
			)
			if err != nil {
				t.Errorf("create callback request: %v", err)
			} else {
				callback.Header.Set("Authorization", "Bearer "+registration.AuthToken)
				response, callErr := http.DefaultClient.Do(callback)
				if callErr != nil {
					t.Errorf("call custom tool: %v", callErr)
				} else {
					defer response.Body.Close()
					if decodeErr := json.NewDecoder(response.Body).Decode(&callbackResult); decodeErr != nil {
						t.Errorf("decode callback response: %v", decodeErr)
					}
				}
			}
			io.WriteString(writer, `{}`)
		case "/sdk.v1.SdkAgentService/CreateAgent":
			if err := json.NewDecoder(request.Body).Decode(&createRequest); err != nil {
				t.Errorf("decode create request: %v", err)
			}
			io.WriteString(writer, `{"agentId":"agent-1","model":{"id":"m"}}`)
		default:
			t.Errorf("unexpected path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	root, _, _ := newTestRoot(server)
	root.SetArgs([]string{
		"agent", "create",
		"--model", "m",
		"--custom-tool-config",
		`{"lookup":{"description":"Search docs","inputSchema":{"type":"object","required":["query"]},` +
			`"command":"printf old"}}`,
		"--custom-tool", `lookup=printf '{"source":"flag"}'`,
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	result := callbackResult["result"].(map[string]any)
	if result["source"] != "flag" {
		t.Fatalf("callback result = %#v", callbackResult)
	}
	options := createRequest["options"].(map[string]any)
	local := options["local"].(map[string]any)
	tools := local["customTools"].(map[string]any)
	lookup := tools["lookup"].(map[string]any)
	if lookup["description"] != "Search docs" {
		t.Fatalf("tool declaration = %#v", lookup)
	}
	schema := lookup["inputSchema"].(map[string]any)
	if schema["type"] != "object" {
		t.Fatalf("input schema = %#v", schema)
	}
}

func TestCustomToolsRejectCloudOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("bridge must not be called")
	}))
	defer server.Close()
	root, _, _ := newTestRoot(server)
	root.SetArgs([]string{
		"agent", "create",
		"--repo", "https://github.com/acme/repo",
		"--custom-tool", "lookup=printf ok",
	})
	err := root.Execute()
	if err == nil || err.Error() != "custom tools are supported for local agents only" {
		t.Fatalf("Execute() error = %v", err)
	}
}
