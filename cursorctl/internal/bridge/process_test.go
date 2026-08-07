package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestParseHandshake(t *testing.T) {
	input := `{
		"schemaVersion": 1,
		"serverVersion": "1.0.27",
		"transport": "tcp",
		"protocol": "connect",
		"url": "http://127.0.0.1:1234",
		"authTokenFile": "/tmp/token",
		"unknown": "ignored"
	}`
	handshake, err := parseHandshake(input)
	if err != nil {
		t.Fatalf("parseHandshake() error = %v", err)
	}
	if handshake.ServerVersion != "1.0.27" ||
		handshake.URL != "http://127.0.0.1:1234" ||
		handshake.AuthTokenFile != "/tmp/token" {
		t.Fatalf("handshake = %#v", handshake)
	}
}

func TestParseHandshakeValidation(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "malformed JSON",
			input: `{`,
			want:  "parse bridge ready handshake",
		},
		{
			name:  "schema",
			input: `{"schemaVersion":2,"transport":"tcp","protocol":"connect","url":"x","authTokenFile":"x"}`,
			want:  "schema version",
		},
		{
			name:  "transport",
			input: `{"schemaVersion":1,"transport":"stdio","protocol":"connect","url":"x","authTokenFile":"x"}`,
			want:  "transport",
		},
		{
			name:  "protocol",
			input: `{"schemaVersion":1,"transport":"tcp","protocol":"grpc","url":"x","authTokenFile":"x"}`,
			want:  "protocol",
		},
		{
			name:  "URL",
			input: `{"schemaVersion":1,"transport":"tcp","protocol":"connect","authTokenFile":"x"}`,
			want:  "missing url",
		},
		{
			name:  "token file",
			input: `{"schemaVersion":1,"transport":"tcp","protocol":"connect","url":"x"}`,
			want:  "missing authTokenFile",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseHandshake(test.input)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("parseHandshake() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestProcessEnvironmentOverridesCursorValues(t *testing.T) {
	t.Setenv("CURSOR_API_KEY", "old")
	t.Setenv("CURSOR_SDK_CLIENT_LANGUAGE", "typescript")
	environment := processEnvironment("new")

	var apiKey, language int
	for _, entry := range environment {
		switch entry {
		case "CURSOR_API_KEY=new":
			apiKey++
		case "CURSOR_SDK_CLIENT_LANGUAGE=go":
			language++
		case "CURSOR_API_KEY=old", "CURSOR_SDK_CLIENT_LANGUAGE=typescript":
			t.Fatalf("stale environment entry %q", entry)
		}
	}
	if apiKey != 1 || language != 1 {
		t.Fatalf("api key entries = %d, language entries = %d", apiKey, language)
	}
}

func TestProcessEnvironmentOmitsEmptyAPIKey(t *testing.T) {
	t.Setenv("CURSOR_API_KEY", "old")
	environment := processEnvironment("")
	for _, entry := range environment {
		if strings.HasPrefix(entry, "CURSOR_API_KEY=") {
			t.Fatalf("unexpected API key environment entry %q", entry)
		}
	}
}

func TestBridgeLifecycleWithFakeProcess(t *testing.T) {
	tempDir := t.TempDir()
	tokenPath := filepath.Join(tempDir, "token")
	pidPath := filepath.Join(tempDir, "pid")
	argsPath := filepath.Join(tempDir, "args")

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/sdk.v1.SdkBridgeControlService/Shutdown" {
			t.Errorf("unexpected RPC path %q", request.URL.Path)
		}
		data, err := os.ReadFile(pidPath)
		if err != nil {
			t.Errorf("read fake bridge PID: %v", err)
		} else {
			pid, conversionErr := strconv.Atoi(strings.TrimSpace(string(data)))
			if conversionErr != nil {
				t.Errorf("parse fake bridge PID: %v", conversionErr)
			} else if signalErr := syscall.Kill(pid, syscall.SIGTERM); signalErr != nil {
				t.Errorf("terminate fake bridge: %v", signalErr)
			}
		}
		writer.Header().Set("Content-Type", "application/json")
		io.WriteString(writer, `{}`)
	}))
	defer server.Close()

	handshake, err := json.Marshal(Handshake{
		SchemaVersion: 1,
		Transport:     "tcp",
		Protocol:      "connect",
		URL:           server.URL,
		AuthTokenFile: tokenPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf(`#!/bin/sh
trap 'exit 0' TERM
printf 'token\n' > %s
printf '%%s\n' "$$" > %s
printf '%%s\n' "$*" > %s
printf 'cursor-sdk-bridge ready %%s\n' %s >&2
printf 'verbose-after-ready\n' >&2
while true; do sleep 0.05; done
`, shellQuote(tokenPath), shellQuote(pidPath), shellQuote(argsPath), shellQuote(string(handshake)))
	scriptPath := writeExecutable(t, tempDir, "fake-bridge", script)

	var logs lockedBuffer
	instance, err := Start(context.Background(), Options{
		BinaryPath: scriptPath,
		Workspace:  tempDir,
		APIKey:     "dummy",
		LocalStore: `{"type":"jsonl"}`,
		Verbose:    true,
		LogWriter:  &logs,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitForString(t, &logs, "verbose-after-ready")
	if err := instance.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if strings.Contains(logs.String(), readyPrefix) {
		t.Fatalf("verbose logs exposed ready handshake: %q", logs.String())
	}
	if instance.command.ProcessState == nil || !instance.command.ProcessState.Exited() {
		t.Fatalf("fake bridge process state = %#v", instance.command.ProcessState)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), `--local-store {"type":"jsonl"}`) ||
		!strings.Contains(string(args), "--verbose") {
		t.Fatalf("bridge arguments = %q", args)
	}
}

func TestBridgeStartupErrorIncludesStderr(t *testing.T) {
	tempDir := t.TempDir()
	scriptPath := writeExecutable(t, tempDir, "bad-bridge", `#!/bin/sh
printf 'fatal startup detail\n' >&2
exit 7
`)
	_, err := Start(context.Background(), Options{
		BinaryPath: scriptPath,
		Workspace:  tempDir,
		APIKey:     "dummy",
	})
	if err == nil || !strings.Contains(err.Error(), "fatal startup detail") {
		t.Fatalf("Start() error = %v", err)
	}
}

func TestScanBridgeStderrDrainsAfterOversizedLine(t *testing.T) {
	reader, writer := io.Pipe()
	ready := make(chan handshakeResult, 1)
	scanDone := make(chan struct{})
	go func() {
		scanBridgeStderr(reader, nil, ready)
		close(scanDone)
	}()

	writeDone := make(chan error, 1)
	go func() {
		_, err := io.WriteString(
			writer,
			strings.Repeat("x", maxReadyLineSize+1)+"\n"+strings.Repeat("y", maxReadyLineSize+1),
		)
		if closeErr := writer.Close(); err == nil {
			err = closeErr
		}
		writeDone <- err
	}()

	result := <-ready
	if result.err == nil || !strings.Contains(result.err.Error(), "exceeds") {
		t.Fatalf("scanBridgeStderr() error = %v", result.err)
	}
	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatalf("write stderr: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("stderr writer blocked; reader did not continue draining")
	}
	select {
	case <-scanDone:
	case <-time.After(time.Second):
		t.Fatal("stderr scanner did not finish after EOF")
	}
}

func writeExecutable(t *testing.T, directory, name, contents string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func waitForString(t *testing.T, buffer *lockedBuffer, value string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buffer.String(), value) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("log output %q does not contain %q", buffer.String(), value)
}
