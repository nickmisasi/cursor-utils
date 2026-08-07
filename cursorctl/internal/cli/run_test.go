package cli

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunConversationParsesJSONString(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/sdk.v1.SdkAgentService/GetRunConversation" {
			t.Errorf("path = %q", request.URL.Path)
		}
		io.WriteString(writer, `{"conversationJson":"[{\"role\":\"user\",\"parts\":[\"hello\"]}]"}`)
	}))
	defer server.Close()

	root, _, output := newTestRoot(server)
	root.SetArgs([]string{"--output", "yaml", "run", "conversation", "run-1"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(output.String(), "role: user") ||
		strings.Contains(output.String(), "conversationJson") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestRunWaitReturnsAgentFailureExit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/sdk.v1.SdkAgentService/WaitLiveRun" {
			t.Errorf("path = %q", request.URL.Path)
		}
		io.WriteString(writer, `{"result":{"runId":"run-error","agentId":"agent-1","status":"RUN_LIFECYCLE_STATUS_ERROR"}}`)
	}))
	defer server.Close()

	root, _, output := newTestRoot(server)
	root.SetArgs([]string{"run", "wait", "run-error"})
	err := root.Execute()
	var exitError *ExitError
	if !errors.As(err, &exitError) || exitError.Code != ExitAgentFailure {
		t.Fatalf("Execute() error = %T %v", err, err)
	}
	if !strings.Contains(output.String(), `"status": "RUN_LIFECYCLE_STATUS_ERROR"`) {
		t.Fatalf("output = %q", output.String())
	}
}

func TestRuntimeFlagUsesCanonicalProtoEnum(t *testing.T) {
	tests := []struct {
		name string
		args []string
		path string
	}{
		{
			name: "agent list",
			args: []string{"agent", "list", "--runtime", "cloud"},
			path: "/sdk.v1.SdkAgentService/ListAgents",
		},
		{
			name: "run list",
			args: []string{"run", "list", "agent-1", "--runtime", "local"},
			path: "/sdk.v1.SdkAgentService/ListRuns",
		},
		{
			name: "run get",
			args: []string{"run", "get", "run-1", "--runtime", "cloud"},
			path: "/sdk.v1.SdkAgentService/GetRun",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var requestBody map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != test.path {
					t.Errorf("path = %q, want %q", request.URL.Path, test.path)
				}
				if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
					t.Errorf("decode request: %v", err)
				}
				io.WriteString(writer, `{}`)
			}))
			defer server.Close()

			root, _, _ := newTestRoot(server)
			root.SetArgs(test.args)
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			options := requestBody["options"].(map[string]any)
			want := "RUNTIME_CLOUD"
			if test.name == "run list" {
				want = "RUNTIME_LOCAL"
			}
			if options["runtime"] != want {
				t.Fatalf("runtime = %#v, want %q", options["runtime"], want)
			}
		})
	}
}
