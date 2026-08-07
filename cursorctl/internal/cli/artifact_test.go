package cli

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestArtifactDownloadDecodesStreamToStdout(t *testing.T) {
	var request map[string]any
	server := newArtifactServer(t, &request, func(writer io.Writer) {
		writeTestFrame(t, writer, 0x00, `{"data":"`+base64.StdEncoding.EncodeToString([]byte("hello "))+`"}`)
		writeTestFrame(t, writer, 0x00, `{"data":"`+base64.StdEncoding.EncodeToString([]byte("world"))+`"}`)
		writeTestFrame(t, writer, 0x02, `{}`)
	})
	defer server.Close()

	root, _, output := newTestRoot(server)
	root.SetArgs([]string{"artifact", "download", "agent-1", "reports/result.txt"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if output.String() != "hello world" {
		t.Fatalf("output = %q", output.String())
	}
	if request["agentId"] != "agent-1" || request["path"] != "reports/result.txt" {
		t.Fatalf("request = %#v", request)
	}
}

func TestArtifactDownloadFileAndPartialCleanup(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := newArtifactServer(t, nil, func(writer io.Writer) {
			writeTestFrame(t, writer, 0x00, `{"data":"ZmlsZSBieXRlcw=="}`)
			writeTestFrame(t, writer, 0x02, `{}`)
		})
		defer server.Close()
		path := filepath.Join(t.TempDir(), "artifact.bin")
		root, _, output := newTestRoot(server)
		root.SetArgs([]string{"artifact", "download", "agent-1", "artifact.bin", "--file", path})
		if err := root.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "file bytes" {
			t.Fatalf("file = %q", data)
		}
		var summary map[string]any
		if err := json.Unmarshal(output.Bytes(), &summary); err != nil {
			t.Fatal(err)
		}
		if summary["path"] != path || summary["bytes"] != float64(len(data)) {
			t.Fatalf("summary = %#v", summary)
		}
	})

	t.Run("stream error removes partial file", func(t *testing.T) {
		server := newArtifactServer(t, nil, func(writer io.Writer) {
			writeTestFrame(t, writer, 0x00, `{"data":"cGFydGlhbA=="}`)
			writeTestFrame(t, writer, 0x02, `{"error":{"code":"internal","message":"stream failed"}}`)
		})
		defer server.Close()
		path := filepath.Join(t.TempDir(), "partial.bin")
		root, _, _ := newTestRoot(server)
		root.SetArgs([]string{"artifact", "download", "agent-1", "artifact.bin", "--file", path})
		if err := root.Execute(); err == nil {
			t.Fatal("Execute() error = nil")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("partial file still exists: %v", err)
		}
	})
}

func newArtifactServer(
	t *testing.T,
	target *map[string]any,
	send func(io.Writer),
) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body := readStreamRequest(t, request)
		if target != nil {
			*target = body
		}
		writer.Header().Set("Content-Type", "application/connect+json")
		send(writer)
	}))
}
