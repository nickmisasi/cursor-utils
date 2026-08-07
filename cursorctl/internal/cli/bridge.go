package cli

import (
	"context"

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
		Short: "Install and inspect the per-invocation SDK bridge",
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
		Short: "Install the pinned SDK bridge release (no RPC)",
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
	var jsonValue string
	command := &cobra.Command{
		Use:   "ping",
		Short: "Check bridge responsiveness with Ping",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			request, err := buildUnaryRequest(
				app,
				command,
				args,
				jsonValue,
				0,
				"bridge ping",
				false,
				nil,
				func(_ []string, _ string) (map[string]any, error) {
					return map[string]any{}, nil
				},
			)
			if err != nil {
				return err
			}
			return runControlRPC[bridgePingResponse](
				app, command, "SdkBridgeControlService", "Ping", request,
			)
		},
	}
	addJSONFlag(command, &jsonValue)
	return command
}

func newBridgeVersionCommand(app *App) *cobra.Command {
	var jsonValue string
	command := &cobra.Command{
		Use:   "version",
		Short: "Show SDK bridge protocol information with GetVersion",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			request, err := buildUnaryRequest(
				app,
				command,
				args,
				jsonValue,
				0,
				"bridge version",
				false,
				nil,
				func(_ []string, _ string) (map[string]any, error) {
					return map[string]any{}, nil
				},
			)
			if err != nil {
				return err
			}
			return runControlRPC[bridgeVersionResponse](
				app, command, "SdkBridgeControlService", "GetVersion", request,
			)
		},
	}
	addJSONFlag(command, &jsonValue)
	return command
}

type rpcClientProvider func(context.Context) (*bridge.Client, error)

func runRPC[T any](
	app *App,
	command *cobra.Command,
	service string,
	method string,
	request any,
) error {
	return runRPCWithClient[T](app, command, service, method, request, app.Client)
}

func runControlRPC[T any](
	app *App,
	command *cobra.Command,
	service string,
	method string,
	request any,
) error {
	return runRPCWithClient[T](app, command, service, method, request, app.ControlClient)
}

func runRPCWithClient[T any](
	app *App,
	command *cobra.Command,
	service string,
	method string,
	request any,
	clientProvider rpcClientProvider,
) error {
	// Commands call this directly so a child PersistentPreRunE cannot bypass initialization.
	if err := prepareCommand(app, command); err != nil {
		return err
	}
	client, err := clientProvider(command.Context())
	if err != nil {
		return err
	}
	var response T
	if err := client.Call(command.Context(), service, method, request, &response); err != nil {
		return err
	}
	return app.Print(response)
}
