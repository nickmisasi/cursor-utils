package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAgentCreateDeclaresCustomToolsWithoutCallbackServer(t *testing.T) {
	var createRequest map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/sdk.v1.SdkAgentService/CreateAgent" {
			t.Errorf("unexpected path %q", request.URL.Path)
		}
		if err := json.NewDecoder(request.Body).Decode(&createRequest); err != nil {
			t.Errorf("decode create request: %v", err)
		}
		io.WriteString(writer, `{"agentId":"agent-1","model":{"id":"m"}}`)
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

func TestAgentCreateRawJSONDeclaresCustomTools(t *testing.T) {
	var createRequest map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/sdk.v1.SdkAgentService/CreateAgent" {
			t.Errorf("unexpected path %q", request.URL.Path)
		}
		if err := json.NewDecoder(request.Body).Decode(&createRequest); err != nil {
			t.Errorf("decode create request: %v", err)
		}
		io.WriteString(writer, `{"agentId":"agent-1","model":{"id":"m"}}`)
	}))
	defer server.Close()

	root, _, _ := newTestRoot(server)
	root.SetArgs([]string{
		"agent", "create",
		"--json", `{"options":{"local":{"cwd":["/tmp"]}}}`,
		"--custom-tool", "lookup=printf ok",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	options := createRequest["options"].(map[string]any)
	if options["apiKey"] != "test-api-key" {
		t.Fatalf("create options = %#v", options)
	}
	local := options["local"].(map[string]any)
	if local["customTools"] == nil {
		t.Fatalf("create request = %#v", createRequest)
	}
}

func TestAgentSendStartsExecutorWithoutRewritingRawRequest(t *testing.T) {
	var sendRequest map[string]any
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
		case "/sdk.v1.SdkAgentService/Send":
			sendRequest = readStreamRequest(t, request)
			writeTestFrame(
				t,
				writer,
				0x00,
				`{"result":{"agentId":"agent-1","runId":"run-1","status":"FINISHED",`+
					`"result":{"agentId":"agent-1","runId":"run-1","status":"FINISHED","result":"ok"}}}`,
			)
			writeTestFrame(t, writer, 0x02, `{}`)
		default:
			t.Errorf("unexpected path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	root, _, _ := newTestRoot(server)
	root.SetArgs([]string{
		"agent", "send",
		"--json", `{"agentId":"agent-1","message":{"text":"work"},"options":{"local":{"force":true}}}`,
		"--custom-tool", `lookup=printf '{"source":"executor"}'`,
		"--quiet",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	result := callbackResult["result"].(map[string]any)
	if result["source"] != "executor" {
		t.Fatalf("callback result = %#v", callbackResult)
	}
	options := sendRequest["options"].(map[string]any)
	local := options["local"].(map[string]any)
	if local["force"] != true || local["customTools"] != nil {
		t.Fatalf("send request was rewritten: %#v", sendRequest)
	}
}

func TestAgentPromptDeclaresToolsOnlyOnCreate(t *testing.T) {
	var createRequest map[string]any
	var sendRequest map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/sdk.v1.SdkBridgeControlService/SetToolCallback":
			io.WriteString(writer, `{}`)
		case "/sdk.v1.SdkAgentService/CreateAgent":
			if err := json.NewDecoder(request.Body).Decode(&createRequest); err != nil {
				t.Errorf("decode create request: %v", err)
			}
			io.WriteString(writer, `{"agentId":"agent-1","model":{"id":"m"}}`)
		case "/sdk.v1.SdkAgentService/Send":
			sendRequest = readStreamRequest(t, request)
			writeTestFrame(
				t,
				writer,
				0x00,
				`{"result":{"agentId":"agent-1","runId":"run-1","status":"FINISHED",`+
					`"result":{"agentId":"agent-1","runId":"run-1","status":"FINISHED","result":"ok"}}}`,
			)
			writeTestFrame(t, writer, 0x02, `{}`)
		case "/sdk.v1.SdkAgentService/CloseAgent":
			io.WriteString(writer, `{}`)
		default:
			t.Errorf("unexpected path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	root, _, _ := newTestRoot(server)
	root.SetArgs([]string{
		"agent", "prompt", "work",
		"--custom-tool", "lookup=printf ok",
		"--quiet",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	createOptions := createRequest["options"].(map[string]any)
	createLocal := createOptions["local"].(map[string]any)
	if createLocal["customTools"] == nil {
		t.Fatalf("create request = %#v", createRequest)
	}
	if _, exists := sendRequest["options"]; exists {
		t.Fatalf("send request contains options: %#v", sendRequest)
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
