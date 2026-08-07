package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/nickmisasi/cursor-utils/cursorctl/internal/bridge"
	"github.com/spf13/cobra"
)

func Execute() int {
	root, app := NewRootCommand()
	err := root.Execute()
	if closeErr := app.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		fmt.Fprintf(app.Err, "error: %v\n", err)
		var exitError *ExitError
		if errors.As(err, &exitError) {
			return exitError.Code
		}
		return ExitCLIError
	}
	return ExitOK
}

func NewRootCommand() (*cobra.Command, *App) {
	workspace, err := os.Getwd()
	if err != nil {
		workspace = "."
	}
	app := &App{
		OutputName:    "json",
		APIKeyEnv:     "CURSOR_API_KEY",
		Workspace:     workspace,
		BridgeVersion: bridge.DefaultVersion,
		In:            os.Stdin,
		Out:           os.Stdout,
		Err:           os.Stderr,
	}
	root := &cobra.Command{
		Use:           "cursorctl",
		Short:         "Manage Cursor SDK agents",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(command *cobra.Command, _ []string) error {
			return prepareCommand(app, command)
		},
	}
	root.SetIn(app.In)
	root.SetOut(app.Out)
	root.SetErr(app.Err)

	flags := root.PersistentFlags()
	flags.StringVarP(&app.OutputName, "output", "o", "json", "Output format: json, yaml, or toon")
	flags.StringVar(&app.APIKeyEnv, "api-key-env", "CURSOR_API_KEY", "Environment variable containing the Cursor API key")
	flags.StringVar(&app.APIKey, "api-key", "", "Cursor API key (sensitive; overrides --api-key-env)")
	flags.StringVar(&app.Workspace, "workspace", workspace, "Workspace passed to the SDK bridge")
	flags.StringVar(&app.BridgeBin, "bridge-bin", "", "Path to the SDK bridge binary")
	flags.StringVar(&app.BridgeVersion, "bridge-version", bridge.DefaultVersion, "SDK bridge release version")
	flags.StringVar(
		&app.LocalStore,
		"local-store",
		"",
		`Bridge local store JSON (sqlite/jsonl; custom stores are an embedder feature)`,
	)
	flags.DurationVar(
		&app.Timeout,
		"timeout",
		0,
		"Invocation timeout, including bridge downloads (0 disables the deadline)",
	)
	flags.BoolVarP(&app.Verbose, "verbose", "v", false, "Show SDK bridge diagnostics")

	root.AddCommand(
		newBridgeCommand(app),
		newAgentCommand(app),
		newRunCommand(app),
		newArtifactCommand(app),
		newCatalogCommand(app, "me", "Me", "Get the authenticated Cursor user"),
		newCatalogCommand(app, "models", "ListModels", "List available Cursor models"),
		newCatalogCommand(app, "repos", "ListRepositories", "List accessible repositories"),
		newVersionCommand(app),
	)
	return root, app
}

func prepareCommand(app *App, command *cobra.Command) error {
	ctx, err := app.prepareContext(command.Context())
	if err != nil {
		return err
	}
	command.SetContext(ctx)
	return nil
}

type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error {
	return e.Err
}
