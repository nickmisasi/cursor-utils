package cli

import (
	"github.com/nickmisasi/cursor-utils/cursorctl/internal/bridge"
	"github.com/spf13/cobra"
)

type bridgePingResponse struct {
	Message string `json:"message"`
}

type bridgeVersionResponse struct {
	BridgeVersion   string   `json:"bridgeVersion"`
	ProtocolVersion string   `json:"protocolVersion"`
	Capabilities    []string `json:"capabilities"`
}

func newBridgeCommand(app *App) *cobra.Command {
	command := &cobra.Command{
		Use:   "bridge",
		Short: "Manage the Cursor SDK bridge",
	}
	command.AddCommand(
		newBridgeInstallCommand(app),
		newBridgePingCommand(app),
		newBridgeVersionCommand(app),
	)
	return command
}

func newBridgeInstallCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install the pinned SDK bridge release",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := bridge.EnsureInstalled(command.Context(), bridge.InstallOptions{
				BinaryPath: app.BridgeBin,
				Version:    app.BridgeVersion,
			})
			if err != nil {
				return err
			}
			return app.Print(result)
		},
	}
}

func newBridgePingCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "ping",
		Short: "Check that the SDK bridge is responsive",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runRPC[bridgePingResponse](
				app,
				command,
				"SdkBridgeControlService",
				"Ping",
				map[string]any{},
			)
		},
	}
}

func newBridgeVersionCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show SDK bridge protocol information",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runRPC[bridgeVersionResponse](
				app,
				command,
				"SdkBridgeControlService",
				"GetVersion",
				map[string]any{},
			)
		},
	}
}

func runRPC[T any](
	app *App,
	command *cobra.Command,
	service string,
	method string,
	request any,
) error {
	// Commands call this directly so a child PersistentPreRunE cannot bypass initialization.
	if err := prepareCommand(app, command); err != nil {
		return err
	}
	client, err := app.Client(command.Context())
	if err != nil {
		return err
	}
	var response T
	if err := client.Call(command.Context(), service, method, request, &response); err != nil {
		return err
	}
	return app.Print(response)
}
