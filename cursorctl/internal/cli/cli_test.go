package cli

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nickmisasi/cursor-utils/cursorctl/internal/bridge"
	"github.com/spf13/cobra"
)

func newTestRoot(server *httptest.Server) (*cobra.Command, *App, *bytes.Buffer) {
	root, app := NewRootCommand()
	output := &bytes.Buffer{}
	app.APIKey = "test-api-key"
	app.Workspace = "/workspace/project"
	app.Out = output
	app.Err = io.Discard
	app.clientOverride = bridge.NewClient(server.URL, "secret", server.Client())
	return root, app, output
}

func newStreamServer(t *testing.T, send func(io.Writer)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/sdk.v1.SdkAgentService/ResumeAgent":
			io.WriteString(writer, `{"agentId":"resumed"}`)
		case "/sdk.v1.SdkAgentService/Send":
			if request.Header.Get("Content-Type") != "application/connect+json" {
				t.Errorf("Content-Type = %q", request.Header.Get("Content-Type"))
			}
			_ = readStreamRequest(t, request)
			writer.Header().Set("Content-Type", "application/connect+json")
			send(writer)
		default:
			t.Errorf("unexpected path %q", request.URL.Path)
		}
	}))
}

func readStreamRequest(t *testing.T, request *http.Request) map[string]any {
	t.Helper()
	var header [5]byte
	if _, err := io.ReadFull(request.Body, header[:]); err != nil {
		t.Fatalf("read request frame: %v", err)
	}
	size := binary.BigEndian.Uint32(header[1:])
	payload := make([]byte, size)
	if _, err := io.ReadFull(request.Body, payload); err != nil {
		t.Fatalf("read request payload: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("decode stream request: %v", err)
	}
	return result
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
