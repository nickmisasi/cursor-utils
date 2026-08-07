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

func TestAgentSendStreamOutputAndFailureExit(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		wantExit int
		wantErr  bool
	}{
		{name: "finished", status: "FINISHED"},
		{name: "error", status: "ERROR", wantExit: ExitAgentFailure, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newStreamServer(t, func(writer io.Writer) {
				writeTestFrame(t, writer, 0x00, `{}`)
				writeTestFrame(t, writer, 0x00, `{"sdkMessage":{"type":"assistant","message":{"text":"hello"}},"offset":"1"}`)
				writeTestFrame(t, writer, 0x00, `{"result":{"agentId":"agent-1","runId":"run-1","status":"`+test.status+`","result":{"agentId":"agent-1","runId":"run-1","status":"`+test.status+`","result":"hello"}},"offset":"2"}`)
				writeTestFrame(t, writer, 0x00, `{"done":{"agentId":"agent-1","runId":"run-1"}}`)
				writeTestFrame(t, writer, 0x02, `{}`)
			})
			defer server.Close()

			root, _, output := newTestRoot(server)
			root.SetArgs([]string{"agent", "send", "agent-1", "hello"})
			err := root.Execute()
			if test.wantErr {
				var exitError *ExitError
				if !errors.As(err, &exitError) || exitError.Code != test.wantExit {
					t.Fatalf("Execute() error = %T %v", err, err)
				}
			} else if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			lines := strings.Split(strings.TrimSpace(output.String()), "\n")
			if len(lines) != 3 {
				t.Fatalf("output lines = %d, output = %q", len(lines), output.String())
			}
			var first map[string]any
			var second map[string]any
			if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
				t.Fatal(err)
			}
			if first["event"] != "sdkMessage" || first["offset"] != "1" {
				t.Fatalf("first event = %#v", first)
			}
			if second["event"] != "result" {
				t.Fatalf("result event = %#v", second)
			}
		})
	}
}

func TestAgentSendDetach(t *testing.T) {
	server := newStreamServer(t, func(writer io.Writer) {
		writeTestFrame(t, writer, 0x00, `{"sdkMessage":{"type":"system","message":{"subtype":"init","runId":"run-detached"}}}`)
		writeTestFrame(t, writer, 0x02, `{}`)
	})
	defer server.Close()

	root, _, output := newTestRoot(server)
	root.SetArgs([]string{"agent", "send", "agent-detached", "work", "--detach"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if got["agentId"] != "agent-detached" || got["runId"] != "run-detached" {
		t.Fatalf("output = %#v", got)
	}
}

func TestRunWatchHappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/sdk.v1.SdkAgentService/ObserveRun" {
			t.Errorf("path = %q", request.URL.Path)
		}
		body := readStreamRequest(t, request)
		if body["runId"] != "run-watch" || body["afterOffset"] != "offset-4" {
			t.Errorf("request = %#v", body)
		}
		writer.Header().Set("Content-Type", "application/connect+json")
		writeTestFrame(t, writer, 0x00, `{"sdkMessage":{"type":"assistant","message":{"text":"watched"}},"offset":"5"}`)
		writeTestFrame(t, writer, 0x00, `{"result":{"agentId":"agent-1","runId":"run-watch","status":"FINISHED","result":{"agentId":"agent-1","runId":"run-watch","status":"FINISHED","result":"watched"}},"offset":"6"}`)
		writeTestFrame(t, writer, 0x00, `{"done":{"agentId":"agent-1","runId":"run-watch"}}`)
		writeTestFrame(t, writer, 0x02, `{}`)
	}))
	defer server.Close()

	root, _, output := newTestRoot(server)
	root.SetArgs([]string{"run", "watch", "run-watch", "--after-offset", "offset-4"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(output.String(), `"event":"sdkMessage"`) ||
		!strings.Contains(output.String(), `"event":"result"`) {
		t.Fatalf("output = %q", output.String())
	}
}

func TestStreamRunIDUsesOnlyProtocolLocations(t *testing.T) {
	if got := streamRunID("sdkMessage", map[string]any{
		"type": "system",
		"message": map[string]any{
			"subtype": "init",
			"runId":   "run-init",
		},
	}); got != "run-init" {
		t.Fatalf("streamRunID(init) = %q", got)
	}
	if got := streamRunID("result", map[string]any{"runId": "run-result"}); got != "run-result" {
		t.Fatalf("streamRunID(result) = %q", got)
	}
	if got := streamRunID("sdkMessage", map[string]any{
		"type":    "assistant",
		"message": map[string]any{"runId": "nested"},
	}); got != "" {
		t.Fatalf("streamRunID(assistant) = %q", got)
	}
}
