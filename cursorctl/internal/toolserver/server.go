package toolserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	callbackPath       = "/sdk.v1.SdkCustomToolCallbackService/CallCustomTool"
	maxRequestBytes    = 1 << 20
	maxToolOutputBytes = 4 << 20
)

type Tool struct {
	Description string
	InputSchema map[string]any
	Command     string
}

type Server struct {
	server    *http.Server
	listener  net.Listener
	token     string
	serveDone chan error
}

func Start(tools map[string]Tool) (*Server, error) {
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for custom tool callbacks: %w", err)
	}
	mux := http.NewServeMux()
	instance := &Server{
		listener:  listener,
		token:     token,
		serveDone: make(chan error, 1),
	}
	mux.Handle(callbackPath, newHandler(token, tools))
	instance.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		err := instance.server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		instance.serveDone <- err
	}()
	return instance, nil
}

func (s *Server) URL() string {
	return "http://" + s.listener.Addr().String()
}

func (s *Server) AuthToken() string {
	return s.token
}

func (s *Server) Close(ctx context.Context) error {
	shutdownErr := s.server.Shutdown(ctx)
	if shutdownErr != nil {
		return fmt.Errorf("shut down custom tool server: %w", shutdownErr)
	}
	serveErr := <-s.serveDone
	if serveErr != nil {
		return fmt.Errorf("serve custom tool callbacks: %w", serveErr)
	}
	return nil
}

func newHandler(token string, tools map[string]Tool) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.Header().Set("Allow", http.MethodPost)
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		expectedAuth := []byte("Bearer " + token)
		actualAuth := []byte(request.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(actualAuth, expectedAuth) != 1 {
			writeConnectError(writer, http.StatusUnauthorized, "unauthenticated", "Unauthorized")
			return
		}
		request.Body = http.MaxBytesReader(writer, request.Body, maxRequestBytes)
		var call struct {
			ToolName   string          `json:"toolName"`
			Args       json.RawMessage `json:"args"`
			ToolCallID string          `json:"toolCallId"`
			AgentID    string          `json:"agentId"`
		}
		if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
			writeConnectError(writer, http.StatusBadRequest, "invalid_argument", "invalid custom tool request")
			return
		}
		var args map[string]any
		if json.Unmarshal(call.Args, &args) != nil || args == nil {
			writeConnectError(writer, http.StatusBadRequest, "invalid_argument", "custom tool args must be an object")
			return
		}
		tool, ok := tools[call.ToolName]
		if !ok {
			writeResult(writer, map[string]any{"error": "unknown custom tool " + call.ToolName})
			return
		}
		result := execute(request.Context(), tool.Command, call.ToolName, call.ToolCallID, call.AgentID, args)
		writeResult(writer, result)
	})
}

func execute(
	ctx context.Context,
	command string,
	toolName string,
	toolCallID string,
	agentID string,
	args map[string]any,
) map[string]any {
	input, err := json.Marshal(args)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	shell, arguments := "/bin/sh", []string{"-c", command}
	if runtime.GOOS == "windows" {
		shell, arguments = "cmd.exe", []string{"/C", command}
	}
	process := exec.CommandContext(ctx, shell, arguments...)
	configureProcessCancellation(process)
	process.WaitDelay = time.Second
	process.Stdin = bytes.NewReader(input)
	process.Env = append(
		os.Environ(),
		"CURSORCTL_TOOL_NAME="+toolName,
		"CURSORCTL_TOOL_CALL_ID="+toolCallID,
		"CURSORCTL_AGENT_ID="+agentID,
	)
	stdout := &limitedBuffer{limit: maxToolOutputBytes}
	stdout.onLimit = func() {
		if process.Cancel != nil {
			_ = process.Cancel()
		}
	}
	stderr := &limitedBuffer{limit: maxToolOutputBytes}
	process.Stdout = stdout
	process.Stderr = stderr
	runErr := process.Run()
	if stdout.exceeded {
		return map[string]any{"error": "tool output too large"}
	}
	if runErr != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = runErr.Error()
		}
		return map[string]any{"error": message}
	}
	var result map[string]any
	if json.Unmarshal(stdout.Bytes(), &result) == nil && result != nil {
		return result
	}
	return map[string]any{"output": stdout.String()}
}

type limitedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
	onLimit  func()
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	if b.exceeded {
		return len(data), nil
	}
	remaining := b.limit - b.buffer.Len()
	if len(data) <= remaining {
		return b.buffer.Write(data)
	}
	if remaining > 0 {
		_, _ = b.buffer.Write(data[:remaining])
	}
	b.exceeded = true
	if b.onLimit != nil {
		b.onLimit()
	}
	return len(data), nil
}

func (b *limitedBuffer) Bytes() []byte {
	return b.buffer.Bytes()
}

func (b *limitedBuffer) String() string {
	return b.buffer.String()
}

func writeResult(writer http.ResponseWriter, result map[string]any) {
	_ = json.NewEncoder(writer).Encode(map[string]any{"result": result})
}

func writeConnectError(writer http.ResponseWriter, status int, code string, message string) {
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{"code": code, "message": message})
}

func randomToken() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate custom tool auth token: %w", err)
	}
	return hex.EncodeToString(data), nil
}
