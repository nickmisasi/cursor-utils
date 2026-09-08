package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/nickmisasi/cursor-utils/cursorctl/internal/bridge"
	"github.com/spf13/cobra"
)

func newAgentPromptCommand(app *App) *cobra.Command {
	var agentFlags agentOptionFlags
	var streamFlags sendFlags
	var customFlags customToolFlags
	var jsonValue string
	command := &cobra.Command{
		Use:   "prompt <text>",
		Short: "Compose CreateAgent, Send, and CloseAgent for one prompt",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			if err := validateStreamOutputFlags(streamFlags.quiet, streamFlags.detach); err != nil {
				return err
			}
			tools, err := loadCustomTools(app, command, &customFlags)
			if err != nil {
				return err
			}
			allowedJSONFlags := append([]string{"quiet", "detach"}, customToolFlagNames...)
			composite, raw, err := jsonRequest(
				app,
				command,
				args,
				jsonValue,
				allowedJSONFlags...,
			)
			if err != nil {
				return err
			}
			apiKey, err := app.ResolvedAPIKey()
			if err != nil {
				return err
			}
			if raw {
				if err := injectAPIKey(composite, "options", apiKey); err != nil {
					return err
				}
			} else {
				if len(args) > 1 || (len(args) == 0 && !command.Flags().Changed("message-file")) {
					return fmt.Errorf("agent prompt requires message text or --message-file")
				}
				options, err := buildAgentOptions(app, command, &agentFlags, apiKey)
				if err != nil {
					return err
				}
				message, err := buildUserMessage(app, command, args, &streamFlags)
				if err != nil {
					return err
				}
				sendOptions, err := buildSendOptions(
					app,
					command,
					&streamFlags,
					agentFlags.model,
					agentFlags.mode,
					agentFlags.mcpConfig,
				)
				if err != nil {
					return err
				}
				composite = map[string]any{
					"options":     options,
					"message":     message,
					"sendOptions": sendOptions,
				}
				if command.Flags().Changed("idempotency-key") {
					composite["idempotencyKey"] = agentFlags.idempotencyKey
				}
			}
			if len(tools.registry) != 0 {
				if err := injectAgentCustomTools(composite, tools.declarations); err != nil {
					return err
				}
			}
			return withCustomToolServer(app, command, tools, func() error {
				return runPrompt(app, command, composite, streamFlags)
			})
		},
	}
	addAgentOptionFlags(command, &agentFlags)
	addIdempotencyKeyFlag(command, &agentFlags.idempotencyKey, "Idempotency key for create and send")
	addSendFlags(command, &streamFlags)
	addCustomToolFlags(command, &customFlags)
	command.Flags().StringVar(
		&jsonValue,
		"json",
		"",
		`Composite JSON {options, message, sendOptions, idempotencyKey?}, @file, or -`,
	)
	return command
}

func runPrompt(
	app *App,
	command *cobra.Command,
	composite map[string]any,
	flags sendFlags,
) error {
	if err := prepareCommand(app, command); err != nil {
		return err
	}
	client, err := app.Client(command.Context())
	if err != nil {
		return err
	}
	options, ok := composite["options"].(map[string]any)
	if !ok {
		return fmt.Errorf("prompt options must be a JSON object")
	}
	message, ok := composite["message"].(map[string]any)
	if !ok {
		return fmt.Errorf("prompt message must be a JSON object")
	}
	createRequest := map[string]any{"options": options}
	idempotencyKey, hasIdempotencyKey := composite["idempotencyKey"]
	if hasIdempotencyKey {
		createRequest["idempotencyKey"] = idempotencyKey
	}
	var createResponse map[string]any
	if err := client.Call(
		command.Context(),
		"SdkAgentService",
		"CreateAgent",
		createRequest,
		&createResponse,
	); err != nil {
		return err
	}
	agentID, ok := createResponse["agentId"].(string)
	if !ok || agentID == "" {
		return fmt.Errorf("CreateAgent response is missing agentId")
	}

	sendRequest := map[string]any{
		"agentId": agentID,
		"message": message,
	}
	if sendOptions, exists := composite["sendOptions"]; exists {
		options, ok := sendOptions.(map[string]any)
		if !ok {
			_ = closeAgent(client, agentID)
			return fmt.Errorf("prompt sendOptions must be a JSON object")
		}
		if len(options) != 0 {
			sendRequest["options"] = options
		}
	}
	if hasIdempotencyKey {
		sendRequest["idempotencyKey"] = idempotencyKey
	}

	reader, streamErr := client.Stream(
		command.Context(),
		"SdkAgentService",
		"Send",
		sendRequest,
	)
	if streamErr == nil {
		streamErr = consumeRunStream(
			app,
			reader,
			streamOutputOptions{quiet: flags.quiet, detach: flags.detach, agentID: agentID},
		)
	}
	if flags.detach && streamErr == nil {
		return nil
	}
	closeErr := closeAgent(client, agentID)
	if streamErr != nil {
		return streamErr
	}
	if closeErr != nil {
		return fmt.Errorf("close prompt agent: %w", closeErr)
	}
	return nil
}

func closeAgent(client *bridge.Client, agentID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var response map[string]any
	return client.Call(
		ctx,
		"SdkAgentService",
		"CloseAgent",
		map[string]any{"agentId": agentID},
		&response,
	)
}
