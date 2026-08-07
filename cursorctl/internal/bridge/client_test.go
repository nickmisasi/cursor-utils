package bridge

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientCallSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/sdk.v1.SdkBridgeControlService/Ping" {
			t.Errorf("path = %q", request.URL.Path)
		}
		assertRequestHeaders(t, request, "application/json")
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body["value"] != "request" {
			t.Errorf("request body = %#v", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		io.WriteString(writer, `{"message":"pong"}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret", server.Client())
	var response struct {
		Message string `json:"message"`
	}
	err := client.Call(
		context.Background(),
		"SdkBridgeControlService",
		"Ping",
		map[string]any{"value": "request"},
		&response,
	)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if response.Message != "pong" {
		t.Fatalf("response message = %q", response.Message)
	}
}

func TestClientCallConnectError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		io.WriteString(writer, `{"code":"invalid_argument","message":"bad request","details":[{"type":"sdk.v1.SdkErrorDetails","value":"AA"}]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret", server.Client())
	err := client.Call(context.Background(), "Service", "Method", map[string]any{}, &map[string]any{})
	var rpcError *RPCError
	if !errors.As(err, &rpcError) {
		t.Fatalf("Call() error = %T %v, want *RPCError", err, err)
	}
	if rpcError.Code != "invalid_argument" || rpcError.Message != "bad request" || len(rpcError.Details) != 1 {
		t.Fatalf("RPCError = %#v", rpcError)
	}
}

func TestClientStreamDataAndCleanEnd(t *testing.T) {
	server := streamServer(t, func(writer io.Writer) {
		mustWriteFrame(t, writer, 0x00, `{"sequence":1}`)
		mustWriteFrame(t, writer, 0x00, `{"sequence":2}`)
		mustWriteFrame(t, writer, 0x02, `{}`)
	})
	defer server.Close()

	reader, err := NewClient(server.URL, "secret", server.Client()).Stream(
		context.Background(),
		"SdkAgentService",
		"ObserveRun",
		map[string]any{"runId": "run-1"},
	)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer reader.Close()

	for want := 1; want <= 2; want++ {
		var message struct {
			Sequence int `json:"sequence"`
		}
		if err := reader.Next(&message); err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		if message.Sequence != want {
			t.Fatalf("sequence = %d, want %d", message.Sequence, want)
		}
	}
	if err := reader.Next(&map[string]any{}); err != io.EOF {
		t.Fatalf("final Next() error = %v, want EOF", err)
	}
	if err := reader.Next(&map[string]any{}); err != io.EOF {
		t.Fatalf("Next() after end error = %v, want EOF", err)
	}
}

func TestClientStreamEndError(t *testing.T) {
	server := streamServer(t, func(writer io.Writer) {
		mustWriteFrame(t, writer, 0x02, `{"error":{"code":"unavailable","message":"try again"}}`)
	})
	defer server.Close()

	reader, err := NewClient(server.URL, "secret", server.Client()).Stream(
		context.Background(),
		"Service",
		"Method",
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer reader.Close()

	err = reader.Next(&map[string]any{})
	var rpcError *RPCError
	if !errors.As(err, &rpcError) {
		t.Fatalf("Next() error = %T %v, want *RPCError", err, err)
	}
	if rpcError.Code != "unavailable" || rpcError.Message != "try again" {
		t.Fatalf("RPCError = %#v", rpcError)
	}
}

func TestFrameRoundTrip(t *testing.T) {
	var buffer bytes.Buffer
	payload := []byte(`{"ok":true}`)
	if err := writeFrame(&buffer, 0x00, payload); err != nil {
		t.Fatalf("writeFrame() error = %v", err)
	}
	flags, got, err := readFrame(&buffer, maxFrameSize)
	if err != nil {
		t.Fatalf("readFrame() error = %v", err)
	}
	if flags != 0x00 || !bytes.Equal(got, payload) {
		t.Fatalf("frame = flags %x payload %q", flags, got)
	}
}

func TestReadFrameRejectsOversizedFrame(t *testing.T) {
	var header [5]byte
	binary.BigEndian.PutUint32(header[1:], 11)
	_, _, err := readFrame(bytes.NewReader(header[:]), 10)
	if err == nil {
		t.Fatal("readFrame() error = nil")
	}
}

func streamServer(t *testing.T, send func(io.Writer)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertRequestHeaders(t, request, "application/connect+json")
		flags, payload, err := readFrame(request.Body, maxFrameSize)
		if err != nil {
			t.Errorf("read request frame: %v", err)
		}
		if flags != 0x00 {
			t.Errorf("request flags = 0x%x", flags)
		}
		var body map[string]any
		if err := json.Unmarshal(payload, &body); err != nil {
			t.Errorf("decode request payload: %v", err)
		}
		writer.Header().Set("Content-Type", "application/connect+json")
		send(writer)
	}))
}

func assertRequestHeaders(t *testing.T, request *http.Request, contentType string) {
	t.Helper()
	if got := request.Header.Get("Content-Type"); got != contentType {
		t.Errorf("Content-Type = %q, want %q", got, contentType)
	}
	if got := request.Header.Get("Authorization"); got != "Bearer secret" {
		t.Errorf("Authorization = %q", got)
	}
	if got := request.Header.Get("Connect-Protocol-Version"); got != "1" {
		t.Errorf("Connect-Protocol-Version = %q", got)
	}
}

func mustWriteFrame(t *testing.T, writer io.Writer, flags byte, payload string) {
	t.Helper()
	if err := writeFrame(writer, flags, []byte(payload)); err != nil {
		t.Fatalf("writeFrame() error = %v", err)
	}
}
