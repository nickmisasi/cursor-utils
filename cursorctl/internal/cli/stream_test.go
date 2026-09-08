package cli

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
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

func TestAgentSendResumesBeforeSend(t *testing.T) {
	tests := []struct {
		name    string
		agentID string
		want    map[string]any
	}{
		{
			name:    "cloud",
			agentID: "bc-follow-up",
			want: map[string]any{
				"agentId": "bc-follow-up",
				"options": map[string]any{
					"apiKey": "test-api-key",
					"cloud":  map[string]any{},
				},
			},
		},
		{
			name:    "local",
			agentID: "agent-local",
			want: map[string]any{
				"agentId": "agent-local",
				"options": map[string]any{
					"apiKey": "test-api-key",
					"local":  map[string]any{"cwd": []any{"/workspace/project"}},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var resumeRequest map[string]any
			var sendCalls int
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/sdk.v1.SdkAgentService/ResumeAgent":
					if err := json.NewDecoder(request.Body).Decode(&resumeRequest); err != nil {
						t.Errorf("decode ResumeAgent: %v", err)
					}
					io.WriteString(writer, `{"agentId":"`+test.agentID+`"}`)
				case "/sdk.v1.SdkAgentService/Send":
					sendCalls++
					_ = readStreamRequest(t, request)
					writer.Header().Set("Content-Type", "application/connect+json")
					writeTestFrame(t, writer, 0x00, `{"result":{"agentId":"`+test.agentID+`","runId":"run-1","status":"FINISHED","result":{"agentId":"`+test.agentID+`","runId":"run-1","status":"FINISHED","result":"ok"}}}`)
					writeTestFrame(t, writer, 0x02, `{}`)
				default:
					t.Errorf("unexpected path %q", request.URL.Path)
				}
			}))
			defer server.Close()

			root, _, _ := newTestRoot(server)
			root.SetArgs([]string{"agent", "send", test.agentID, "hello", "--quiet"})
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if sendCalls != 1 {
				t.Fatalf("Send calls = %d", sendCalls)
			}
			if !reflect.DeepEqual(resumeRequest, test.want) {
				t.Fatalf("ResumeAgent request = %#v\nwant %#v", resumeRequest, test.want)
			}
		})
	}
}
