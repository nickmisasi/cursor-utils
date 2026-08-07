package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
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

			root, app, output := newTestRoot(server)
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

	root, _, _ := newTestRoot(server)
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

	root, _, _ = newTestRoot(server)
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

	root, _, _ = newTestRoot(server)
	root.SetArgs([]string{"agent", "create", "--json", `{"options":{}}`, "--name", "conflict"})
	err := root.Execute()
	if err == nil || err.Error() != "cannot combine --json with --name" {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestAgentPromptClosesAfterStreamErrorAndForwardsJSONIdempotency(t *testing.T) {
	var createRequest map[string]any
	var sendRequest map[string]any
	var closeCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/sdk.v1.SdkAgentService/CreateAgent":
			if err := json.NewDecoder(request.Body).Decode(&createRequest); err != nil {
				t.Errorf("decode CreateAgent request: %v", err)
			}
			io.WriteString(writer, `{"agentId":"agent-prompt","model":{"id":"m"}}`)
		case "/sdk.v1.SdkAgentService/Send":
			sendRequest = readStreamRequest(t, request)
			writer.Header().Set("Content-Type", "application/connect+json")
			writeTestFrame(t, writer, 0x02, `{"error":{"code":"internal","message":"send failed"}}`)
		case "/sdk.v1.SdkAgentService/CloseAgent":
			closeCalls.Add(1)
			io.WriteString(writer, `{}`)
		default:
			t.Errorf("unexpected path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	root, _, _ := newTestRoot(server)
	root.SetArgs([]string{
		"agent", "prompt", "--json",
		`{"options":{"local":{"cwd":["/tmp"]}},"message":{"text":"work"},"sendOptions":{},"idempotencyKey":"prompt-key"}`,
	})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "send failed") {
		t.Fatalf("Execute() error = %v", err)
	}
	if closeCalls.Load() != 1 {
		t.Fatalf("CloseAgent calls = %d, want 1", closeCalls.Load())
	}
	if createRequest["idempotencyKey"] != "prompt-key" ||
		sendRequest["idempotencyKey"] != "prompt-key" {
		t.Fatalf("idempotency keys: create=%#v send=%#v", createRequest, sendRequest)
	}
	options := createRequest["options"].(map[string]any)
	if options["apiKey"] != "test-api-key" {
		t.Fatalf("CreateAgent options = %#v", options)
	}
}
