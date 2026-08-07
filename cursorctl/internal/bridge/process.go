package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const readyPrefix = "cursor-sdk-bridge ready "

type Options struct {
	BinaryPath string
	Version    string
	Workspace  string
	APIKey     string
	Verbose    bool
	HTTPClient *http.Client
}

type Handshake struct {
	SchemaVersion int    `json:"schemaVersion"`
	ServerVersion string `json:"serverVersion"`
	PID           int    `json:"pid"`
	Transport     string `json:"transport"`
	Protocol      string `json:"protocol"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	URL           string `json:"url"`
	AuthTokenFile string `json:"authTokenFile"`
	WorkspaceRef  string `json:"workspaceRef"`
	StateRoot     string `json:"stateRoot"`
}

type Bridge struct {
	client    *Client
	command   *exec.Cmd
	wait      <-chan error
	closeOnce sync.Once
	closeErr  error
}

func Start(ctx context.Context, options Options) (*Bridge, error) {
	install, err := EnsureInstalled(ctx, InstallOptions{
		BinaryPath: options.BinaryPath,
		Version:    options.Version,
		HTTPClient: options.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	workspace := options.Workspace
	if workspace == "" {
		workspace, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("resolve workspace: %w", err)
		}
	}
	workspace, err = filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}

	args := []string{"--workspace", workspace, "--port", "0"}
	if options.Verbose {
		args = append(args, "--verbose")
	}
	command := exec.CommandContext(ctx, install.Path, args...)
	command.Env = processEnvironment(options.APIKey)
	stderr, err := command.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("open bridge stderr: %w", err)
	}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start bridge: %w", err)
	}

	wait := make(chan error, 1)
	go func() {
		wait <- command.Wait()
		close(wait)
	}()

	ready := make(chan handshakeResult, 1)
	go scanBridgeStderr(stderr, options.Verbose, ready)

	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	var handshake Handshake
	select {
	case result := <-ready:
		if result.err != nil {
			stopProcess(command, wait)
			return nil, result.err
		}
		handshake = result.handshake
	case err := <-wait:
		if err == nil {
			err = fmt.Errorf("bridge exited before becoming ready")
		}
		return nil, fmt.Errorf("start bridge: %w", err)
	case <-timer.C:
		stopProcess(command, wait)
		return nil, fmt.Errorf("bridge did not become ready within 30s")
	case <-ctx.Done():
		stopProcess(command, wait)
		return nil, ctx.Err()
	}

	token, err := os.ReadFile(handshake.AuthTokenFile)
	if err != nil {
		stopProcess(command, wait)
		return nil, fmt.Errorf("read bridge auth token: %w", err)
	}
	trimmedToken := strings.TrimSpace(string(token))
	if trimmedToken == "" {
		stopProcess(command, wait)
		return nil, fmt.Errorf("bridge auth token is empty")
	}

	return &Bridge{
		client:  NewClient(handshake.URL, trimmedToken, options.HTTPClient),
		command: command,
		wait:    wait,
	}, nil
}

func (b *Bridge) Client() *Client {
	return b.client
}

func (b *Bridge) Close() error {
	b.closeOnce.Do(func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = b.client.Call(
			shutdownContext,
			"SdkBridgeControlService",
			"Shutdown",
			map[string]any{"graceSeconds": 0},
			&map[string]any{},
		)

		select {
		case err := <-b.wait:
			b.closeErr = acceptableExitError(err)
		case <-time.After(750 * time.Millisecond):
			if err := b.command.Process.Kill(); err != nil && !isProcessDone(err) {
				b.closeErr = fmt.Errorf("kill bridge: %w", err)
			}
			if err := <-b.wait; b.closeErr == nil {
				b.closeErr = acceptableExitError(err)
			}
		}
	})
	return b.closeErr
}

type handshakeResult struct {
	handshake Handshake
	err       error
}

func scanBridgeStderr(reader io.Reader, verbose bool, ready chan<- handshakeResult) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	found := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, readyPrefix) {
			if !found {
				handshake, err := parseHandshake(strings.TrimPrefix(line, readyPrefix))
				ready <- handshakeResult{handshake: handshake, err: err}
				found = true
			}
			continue
		}
		if verbose {
			fmt.Fprintln(os.Stderr, line)
		}
	}
	if !found {
		err := scanner.Err()
		if err == nil {
			err = fmt.Errorf("bridge stderr closed before ready handshake")
		} else {
			err = fmt.Errorf("read bridge stderr: %w", err)
		}
		ready <- handshakeResult{err: err}
	}
}

func parseHandshake(data string) (Handshake, error) {
	var handshake Handshake
	if err := json.Unmarshal([]byte(data), &handshake); err != nil {
		return Handshake{}, fmt.Errorf("parse bridge ready handshake: %w", err)
	}
	if handshake.SchemaVersion != 1 {
		return Handshake{}, fmt.Errorf("unsupported bridge handshake schema version %d", handshake.SchemaVersion)
	}
	if handshake.Transport != "tcp" {
		return Handshake{}, fmt.Errorf("unsupported bridge transport %q", handshake.Transport)
	}
	if handshake.Protocol != "connect" {
		return Handshake{}, fmt.Errorf("unsupported bridge protocol %q", handshake.Protocol)
	}
	if handshake.URL == "" {
		return Handshake{}, fmt.Errorf("bridge handshake is missing url")
	}
	if handshake.AuthTokenFile == "" {
		return Handshake{}, fmt.Errorf("bridge handshake is missing authTokenFile")
	}
	return handshake, nil
}

func processEnvironment(apiKey string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "CURSOR_API_KEY=") ||
			strings.HasPrefix(entry, "CURSOR_SDK_CLIENT_LANGUAGE=") {
			continue
		}
		environment = append(environment, entry)
	}
	environment = append(
		environment,
		"CURSOR_API_KEY="+apiKey,
		"CURSOR_SDK_CLIENT_LANGUAGE=go",
	)
	return environment
}

func stopProcess(command *exec.Cmd, wait <-chan error) {
	_ = command.Process.Kill()
	<-wait
}

func acceptableExitError(err error) error {
	if err == nil {
		return nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return nil
	}
	return fmt.Errorf("wait for bridge: %w", err)
}

func isProcessDone(err error) bool {
	return errors.Is(err, os.ErrProcessDone)
}
