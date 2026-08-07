package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nickmisasi/cursor-utils/cursorctl/internal/bridge"
	"github.com/nickmisasi/cursor-utils/cursorctl/internal/output"
)

const (
	ExitOK           = 0
	ExitCLIError     = 1
	ExitAgentFailure = 2
)

type App struct {
	OutputName    string
	APIKeyEnv     string
	APIKey        string
	Workspace     string
	BridgeBin     string
	BridgeVersion string
	Timeout       time.Duration
	Verbose       bool

	In  io.Reader
	Out io.Writer
	Err io.Writer

	format output.Format
	cancel context.CancelFunc

	mu     sync.Mutex
	bridge *bridge.Bridge
}

func (a *App) prepareContext(ctx context.Context) (context.Context, error) {
	format, err := output.ParseFormat(a.OutputName)
	if err != nil {
		return nil, err
	}
	a.format = format
	workspace, err := filepath.Abs(a.Workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}
	a.Workspace = workspace
	if a.Timeout < 0 {
		return nil, fmt.Errorf("timeout must not be negative")
	}
	if a.Timeout > 0 {
		ctx, a.cancel = context.WithTimeout(ctx, a.Timeout)
	}
	return ctx, nil
}

func (a *App) Client(ctx context.Context) (*bridge.Client, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.bridge != nil {
		return a.bridge.Client(), nil
	}

	apiKey, err := a.resolveAPIKey()
	if err != nil {
		return nil, err
	}
	instance, err := bridge.Start(ctx, bridge.Options{
		BinaryPath: a.BridgeBin,
		Version:    a.BridgeVersion,
		Workspace:  a.Workspace,
		APIKey:     apiKey,
		Verbose:    a.Verbose,
	})
	if err != nil {
		return nil, err
	}
	a.bridge = instance
	return instance.Client(), nil
}

func (a *App) Print(value any) error {
	return output.Print(a.Out, a.format, value)
}

func (a *App) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	var err error
	if a.bridge != nil {
		err = a.bridge.Close()
		a.bridge = nil
	}
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	return err
}

func (a *App) resolveAPIKey() (string, error) {
	if a.APIKey != "" {
		return a.APIKey, nil
	}
	if value := os.Getenv(a.APIKeyEnv); value != "" {
		return value, nil
	}
	return "", fmt.Errorf("Cursor API key is required: set --api-key or environment variable %s", a.APIKeyEnv)
}
