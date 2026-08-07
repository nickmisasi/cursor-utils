package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestArtifactDownloadDecodesStreamToStdout(t *testing.T) {
	var request map[string]any
	server := newStreamServer(t, func(writer io.Writer) {
		writeTestFrame(t, writer, 0x00, `{"data":"`+base64.StdEncoding.EncodeToString([]byte("hello "))+`"}`)
		writeTestFrame(t, writer, 0x00, `{"data":"`+base64.StdEncoding.EncodeToString([]byte("world"))+`"}`)
		writeTestFrame(t, writer, 0x02, `{}`)
	})
	server.Config.Handler = captureStreamRequest(t, server.Config.Handler, &request)
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

func captureStreamRequest(
	t *testing.T,
	next http.Handler,
	target *map[string]any,
) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body := readStreamRequest(t, request)
		*target = body
		var framed bytes.Buffer
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		writeTestFrame(t, &framed, 0x00, string(payload))
		request.Body = io.NopCloser(&framed)
		next.ServeHTTP(writer, request)
	})
}
