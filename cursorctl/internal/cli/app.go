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
	LocalStore    string
	Timeout       time.Duration
	Verbose       bool

	In  io.Reader
	Out io.Writer
	Err io.Writer

	format output.Format
	cancel context.CancelFunc

	mu              sync.Mutex
	bridge          *bridge.Bridge
	clientOverride  *bridge.Client
	prepared        bool
	preparedContext context.Context
}

func (a *App) prepareContext(ctx context.Context) (context.Context, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.prepared {
		return a.preparedContext, nil
	}
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
	a.prepared = true
	a.preparedContext = ctx
	return ctx, nil
}

func (a *App) Client(ctx context.Context) (*bridge.Client, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.clientOverride != nil {
		return a.clientOverride, nil
	}
	if a.bridge != nil {
		return a.bridge.Client(), nil
	}

	apiKey, err := a.resolveAPIKey()
	if err != nil {
		return nil, err
	}
	var logWriter io.Writer
	if a.Verbose {
		logWriter = a.Err
	}
	instance, err := bridge.Start(ctx, bridge.Options{
		BinaryPath: a.BridgeBin,
		Version:    a.BridgeVersion,
		Workspace:  a.Workspace,
		APIKey:     apiKey,
		LocalStore: a.LocalStore,
		Verbose:    a.Verbose,
		LogWriter:  logWriter,
	})
	if err != nil {
		return nil, err
	}
	a.bridge = instance
	return instance.Client(), nil
}

func (a *App) ResolvedAPIKey() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.resolveAPIKey()
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
	a.prepared = false
	a.preparedContext = nil
	return err
}

func (a *App) resolveAPIKey() (string, error) {
	if a.APIKey != "" {
		return a.APIKey, nil
	}
	if value := os.Getenv(a.APIKeyEnv); value != "" {
		return value, nil
	}
	return "", fmt.Errorf("missing Cursor API key: set --api-key or environment variable %s", a.APIKeyEnv)
}
