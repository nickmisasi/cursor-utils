package toolserver

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerRejectsInvalidAuth(t *testing.T) {
	server := httptest.NewServer(newHandler("secret", map[string]Tool{}))
	defer server.Close()
	request, err := http.NewRequest(http.MethodPost, server.URL+callbackPath, bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestHandlerExecutesAndWrapsTools(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    map[string]any
	}{
		{
			name: "JSON object and environment",
			command: `printf '{"tool":"%s","call":"%s","agent":"%s","args":' ` +
				`"$CURSORCTL_TOOL_NAME" "$CURSORCTL_TOOL_CALL_ID" "$CURSORCTL_AGENT_ID"; ` +
				`cat; printf '}'`,
			want: map[string]any{
				"tool":  "lookup",
				"call":  "call-1",
				"agent": "agent-1",
				"args":  map[string]any{"query": "docs"},
			},
		},
		{
			name:    "non-object stdout",
			command: `printf 'plain text'`,
			want:    map[string]any{"output": "plain text"},
		},
		{
			name:    "command failure",
			command: `printf 'tool broke\n' >&2; exit 7`,
			want:    map[string]any{"error": "tool broke"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := newHandler("secret", map[string]Tool{"lookup": {Command: test.command}})
			server := httptest.NewServer(handler)
			defer server.Close()
			body := bytes.NewBufferString(
				`{"toolName":"lookup","args":{"query":"docs"},"toolCallId":"call-1","agentId":"agent-1"}`,
			)
			request, err := http.NewRequest(http.MethodPost, server.URL+callbackPath, body)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Authorization", "Bearer secret")
			request.TransferEncoding = []string{"chunked"}
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			data, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Result map[string]any `json:"result"`
			}
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("decode response %q: %v", data, err)
			}
			if !equalJSON(got.Result, test.want) {
				t.Fatalf("result = %#v, want %#v", got.Result, test.want)
			}
		})
	}
}

func equalJSON(left, right any) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return bytes.Equal(leftJSON, rightJSON)
}
