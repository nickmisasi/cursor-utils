package cli

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/nickmisasi/cursor-utils/cursorctl/internal/bridge"
	"github.com/spf13/cobra"
)

func TestAgentCreateFlagMapping(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want map[string]any
	}{
		{
			name: "default local workspace",
			args: []string{"agent", "create", "--model", "composer-2"},
			want: map[string]any{
				"options": map[string]any{
					"apiKey": "test-api-key",
					"model":  map[string]any{"id": "composer-2"},
					"local":  map[string]any{"cwd": []any{"/workspace/project"}},
				},
			},
		},
		{
			name: "local",
			args: []string{
				"agent", "create",
				"--model", "composer-2",
				"--name", "local-agent",
				"--agent-id", "chosen-id",
				"--mode", "plan",
				"--idempotency-key", "create-key",
				"--cwd", "/tmp/primary",
				"--dir", "/tmp/secondary",
				"--setting-source", "project",
				"--sandbox",
				"--auto-review",
				"--mcp-config", `{"docs":{"stdio":{"command":"docs"}}}`,
				"--agents-config", `{"reviewer":{"description":"Review","prompt":"Review this"}}`,
				"--tool", "Read",
				"--disallowed-tool", "Shell",
			},
			want: map[string]any{
				"idempotencyKey": "create-key",
				"options": map[string]any{
					"apiKey":  "test-api-key",
					"model":   map[string]any{"id": "composer-2"},
					"name":    "local-agent",
					"agentId": "chosen-id",
					"mode":    "AGENT_MODE_OPTION_PLAN",
					"local": map[string]any{
						"cwd":            []any{"/tmp/primary"},
						"dirs":           []any{"/tmp/secondary"},
						"settingSources": []any{"SETTING_SOURCE_PROJECT"},
						"sandboxOptions": map[string]any{"enabled": true},
						"autoReview":     true,
					},
					"mcpServers": map[string]any{
						"docs": map[string]any{"stdio": map[string]any{"command": "docs"}},
					},
					"agents": map[string]any{
						"reviewer": map[string]any{
							"description": "Review",
							"prompt":      "Review this",
						},
					},
					"tools":           map[string]any{"names": []any{"Read"}},
					"disallowedTools": []any{"Shell"},
				},
			},
		},
		{
			name: "cloud",
			args: []string{
				"agent", "create",
				"--repo", "https://github.com/acme/repo@main",
				"--pr-url", "https://github.com/acme/repo/pull/1",
				"--env-type", "cloud",
				"--env-name", "production",
				"--auto-create-pr",
				"--skip-reviewer-request",
				"--work-on-current-branch",
				"--env-var", "TOKEN=value=with=equals",
				"--metadata", "owner=cli",
				"--open-as-github-app",
			},
			want: map[string]any{
				"options": map[string]any{
					"apiKey": "test-api-key",
					"cloud": map[string]any{
						"repos": []any{map[string]any{
							"url":         "https://github.com/acme/repo",
							"startingRef": "main",
							"prUrl":       "https://github.com/acme/repo/pull/1",
						}},
						"env": map[string]any{
							"type": "CLOUD_ENVIRONMENT_TYPE_CLOUD",
							"name": "production",
						},
						"autoCreatePr":          true,
						"skipReviewerRequest":   true,
						"workOnCurrentBranch":   true,
						"envVars":               map[string]any{"TOKEN": "value=with=equals"},
						"metadata":              map[string]any{"owner": "cli"},
						"openAsCursorGithubApp": true,
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/sdk.v1.SdkAgentService/CreateAgent" {
					t.Errorf("path = %q", request.URL.Path)
				}
				if err := json.NewDecoder(request.Body).Decode(&got); err != nil {
					t.Errorf("decode request: %v", err)
				}
				io.WriteString(writer, `{"agentId":"agent-1","model":{"id":"composer-2"}}`)
			}))
			defer server.Close()

			root, app, output := phase2TestRoot(server)
			root.SetArgs(test.args)
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("request = %#v\nwant %#v", got, test.want)
			}
			if !strings.Contains(output.String(), `"agentId": "agent-1"`) {
				t.Fatalf("output = %q", output.String())
			}
			_ = app.Close()
		})
	}
}

func TestAgentCreateJSONInjectionAndConflict(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		io.WriteString(writer, `{"agentId":"agent-json","model":{"id":"m"}}`)
	}))
	defer server.Close()

	root, _, _ := phase2TestRoot(server)
	root.SetArgs([]string{
		"agent", "create", "--json",
		`{"options":{"name":"from-json","cloud":{"repos":[{"url":"https://example.com/repo"}]}},"idempotencyKey":"raw-key"}`,
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	options := got["options"].(map[string]any)
	if options["apiKey"] != "test-api-key" || options["name"] != "from-json" {
		t.Fatalf("request = %#v", got)
	}
	if got["idempotencyKey"] != "raw-key" {
		t.Fatalf("request = %#v", got)
	}

	root, _, _ = phase2TestRoot(server)
	root.SetArgs([]string{
		"agent", "create", "--json",
		`{"options":{"apiKey":"payload-key","name":"preserved"}}`,
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	options = got["options"].(map[string]any)
	if options["apiKey"] != "payload-key" || options["name"] != "preserved" {
		t.Fatalf("request = %#v", got)
	}

	root, _, _ = phase2TestRoot(server)
	root.SetArgs([]string{"agent", "create", "--json", `{"options":{}}`, "--name", "conflict"})
	err := root.Execute()
	if err == nil || err.Error() != "cannot combine --json with --name" {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestPhase2LeafCommandsHaveJSONFlag(t *testing.T) {
	root, _ := NewRootCommand()
	for _, namespace := range []string{"agent", "run"} {
		command, _, err := root.Find([]string{namespace})
		if err != nil {
			t.Fatal(err)
		}
		for _, child := range command.Commands() {
			if child.Flags().Lookup("json") == nil {
				t.Errorf("%s %s has no --json flag", namespace, child.Name())
			}
		}
	}
}

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
			server := phase2StreamServer(t, func(writer io.Writer) {
				writeTestFrame(t, writer, 0x00, `{}`)
				writeTestFrame(t, writer, 0x00, `{"sdkMessage":{"type":"assistant","message":{"text":"hello"}},"offset":"1"}`)
				writeTestFrame(t, writer, 0x00, `{"result":{"agentId":"agent-1","runId":"run-1","status":"`+test.status+`","result":{"agentId":"agent-1","runId":"run-1","status":"`+test.status+`","result":"hello"}},"offset":"2"}`)
				writeTestFrame(t, writer, 0x00, `{"done":{"agentId":"agent-1","runId":"run-1"}}`)
				writeTestFrame(t, writer, 0x02, `{}`)
			})
			defer server.Close()

			root, _, output := phase2TestRoot(server)
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
	server := phase2StreamServer(t, func(writer io.Writer) {
		writeTestFrame(t, writer, 0x00, `{"sdkMessage":{"type":"system","message":{"subtype":"init","runId":"run-detached"}}}`)
		writeTestFrame(t, writer, 0x02, `{}`)
	})
	defer server.Close()

	root, _, output := phase2TestRoot(server)
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

func TestRunConversationParsesJSONString(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/sdk.v1.SdkAgentService/GetRunConversation" {
			t.Errorf("path = %q", request.URL.Path)
		}
		io.WriteString(writer, `{"conversationJson":"[{\"role\":\"user\",\"parts\":[\"hello\"]}]"}`)
	}))
	defer server.Close()

	root, _, output := phase2TestRoot(server)
	root.SetArgs([]string{"--output", "yaml", "run", "conversation", "run-1"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(output.String(), "role: user") ||
		strings.Contains(output.String(), "conversationJson") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestParseKeyValuesAndRepo(t *testing.T) {
	values, err := parseKeyValues([]string{"A=one", "B=two=three", "EMPTY="})
	if err != nil {
		t.Fatalf("parseKeyValues() error = %v", err)
	}
	if values["A"] != "one" || values["B"] != "two=three" || values["EMPTY"] != "" {
		t.Fatalf("values = %#v", values)
	}
	if _, err := parseKeyValues([]string{"missing"}); err == nil {
		t.Fatal("parseKeyValues() error = nil")
	}

	repo, err := parseRepo("git@github.com:acme/repo.git@feature")
	if err != nil {
		t.Fatalf("parseRepo() error = %v", err)
	}
	if repo["url"] != "git@github.com:acme/repo.git" || repo["startingRef"] != "feature" {
		t.Fatalf("repo = %#v", repo)
	}
	repo, err = parseRepo("https://user@github.com/acme/repo")
	if err != nil {
		t.Fatalf("parseRepo() error = %v", err)
	}
	if repo["url"] != "https://user@github.com/acme/repo" {
		t.Fatalf("repo = %#v", repo)
	}
}

func phase2TestRoot(server *httptest.Server) (*cobra.Command, *App, *bytes.Buffer) {
	root, app := NewRootCommand()
	output := &bytes.Buffer{}
	app.APIKey = "test-api-key"
	app.Workspace = "/workspace/project"
	app.Out = output
	app.Err = io.Discard
	app.clientOverride = bridge.NewClient(server.URL, "secret", server.Client())
	return root, app, output
}

func phase2StreamServer(t *testing.T, send func(io.Writer)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Content-Type") != "application/connect+json" {
			t.Errorf("Content-Type = %q", request.Header.Get("Content-Type"))
		}
		var header [5]byte
		if _, err := io.ReadFull(request.Body, header[:]); err != nil {
			t.Errorf("read request frame: %v", err)
		}
		size := binary.BigEndian.Uint32(header[1:])
		payload := make([]byte, size)
		if _, err := io.ReadFull(request.Body, payload); err != nil {
			t.Errorf("read request payload: %v", err)
		}
		writer.Header().Set("Content-Type", "application/connect+json")
		send(writer)
	}))
}

func writeTestFrame(t *testing.T, writer io.Writer, flags byte, payload string) {
	t.Helper()
	var header [5]byte
	header[0] = flags
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))
	if _, err := writer.Write(header[:]); err != nil {
		t.Fatalf("write frame header: %v", err)
	}
	if _, err := io.WriteString(writer, payload); err != nil {
		t.Fatalf("write frame payload: %v", err)
	}
}
