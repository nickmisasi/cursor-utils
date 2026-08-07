package toolserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const callbackPath = "/sdk.v1.SdkCustomToolCallbackService/CallCustomTool"

type Tool struct {
	Description string
	InputSchema map[string]any
	Command     string
}

type Server struct {
	server   *http.Server
	listener net.Listener
	token    string
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
	instance := &Server{listener: listener, token: token}
	mux.Handle(callbackPath, newHandler(token, tools))
	instance.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		_ = instance.server.Serve(listener)
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
	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shut down custom tool server: %w", err)
	}
	return nil
}

func newHandler(token string, tools map[string]Tool) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method != http.MethodPost {
			writeConnectError(writer, http.StatusMethodNotAllowed, "unimplemented", "method not allowed")
			return
		}
		if request.Header.Get("Authorization") != "Bearer "+token {
			writeConnectError(writer, http.StatusUnauthorized, "unauthenticated", "Unauthorized")
			return
		}
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
	process.Stdin = bytes.NewReader(input)
	process.Env = append(
		toolEnvironment(),
		"CURSORCTL_TOOL_NAME="+toolName,
		"CURSORCTL_TOOL_CALL_ID="+toolCallID,
		"CURSORCTL_AGENT_ID="+agentID,
	)
	var stdout, stderr bytes.Buffer
	process.Stdout = &stdout
	process.Stderr = &stderr
	if err := process.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return map[string]any{"error": message}
	}
	var result map[string]any
	if json.Unmarshal(stdout.Bytes(), &result) == nil && result != nil {
		return result
	}
	return map[string]any{"output": stdout.String()}
}

func toolEnvironment() []string {
	environment := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "CURSORCTL_TOOL_NAME=") ||
			strings.HasPrefix(entry, "CURSORCTL_TOOL_CALL_ID=") ||
			strings.HasPrefix(entry, "CURSORCTL_AGENT_ID=") {
			continue
		}
		environment = append(environment, entry)
	}
	return environment
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
