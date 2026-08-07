package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestAgentMessagesFlagMapping(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/sdk.v1.SdkAgentService/ListAgentMessages" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if err := json.NewDecoder(request.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		io.WriteString(writer, `{"messages":[]}`)
	}))
	defer server.Close()
	root, _, _ := newTestRoot(server)
	root.SetArgs([]string{
		"agent", "messages", "agent-1",
		"--limit", "20",
		"--offset", "4",
		"--runtime", "local",
		"--cwd", "/tmp/project",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := map[string]any{
		"agentId": "agent-1",
		"options": map[string]any{
			"apiKey":  "test-api-key",
			"limit":   float64(20),
			"offset":  float64(4),
			"runtime": "RUNTIME_LOCAL",
			"cwd":     "/tmp/project",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("request = %#v, want %#v", got, want)
	}
}

func TestAgentUsageFlagMapping(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/sdk.v1.SdkAgentService/GetUsage" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if err := json.NewDecoder(request.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		io.WriteString(writer, `{"usage":{}}`)
	}))
	defer server.Close()
	root, _, _ := newTestRoot(server)
	root.SetArgs([]string{"agent", "usage", "agent-1", "--run-id", "run-2"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := map[string]any{"agentId": "agent-1", "runId": "run-2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("request = %#v, want %#v", got, want)
	}
}
