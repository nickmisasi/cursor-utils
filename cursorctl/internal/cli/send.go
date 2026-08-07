package cli

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/spf13/cobra"
)

type sharedSendFlags struct {
	model          string
	mode           string
	mcpConfig      string
	idempotencyKey string
}

type sendFlags struct {
	force       bool
	envVars     []string
	deltas      bool
	steps       bool
	messageFile string
	images      []string
	quiet       bool
	detach      bool
}

func addSharedSendFlags(command *cobra.Command, flags *sharedSendFlags) {
	set := command.Flags()
	set.StringVar(&flags.model, "model", "", "Model identifier for this send")
	set.StringVar(&flags.mode, "mode", "", "Conversation mode: agent or plan")
	set.StringVar(&flags.mcpConfig, "mcp-config", "", "MCP server map as JSON, @file, or -")
	set.StringVar(&flags.idempotencyKey, "idempotency-key", "", "Idempotency key for this send")
}

func addSendFlags(command *cobra.Command, flags *sendFlags) {
	set := command.Flags()
	set.StringVar(&flags.messageFile, "message-file", "", "Read message text from a file or - for stdin")
	set.StringArrayVar(&flags.images, "image", nil, "Image path or http(s) URL (repeatable)")
	set.BoolVar(&flags.force, "force", false, "Force a local send")
	set.StringArrayVar(&flags.envVars, "send-env-var", nil, "Run-scoped cloud environment KEY=VAL (repeatable)")
	set.BoolVar(&flags.deltas, "deltas", false, "Stream interaction update events")
	set.BoolVar(&flags.steps, "steps", false, "Stream completed conversation steps")
	set.BoolVar(&flags.quiet, "quiet", false, "Suppress events and print only the final run result")
	set.BoolVar(
		&flags.detach,
		"detach",
		false,
		"Print agentId and runId, then close the stream without cancelling the run",
	)
}

func newAgentSendCommand(app *App) *cobra.Command {
	var shared sharedSendFlags
	var flags sendFlags
	var customFlags customToolFlags
	var jsonValue string
	command := &cobra.Command{
		Use:   "send <agent-id> [text]",
		Short: "Send a message and stream the run with Send",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			if err := validateStreamOutputFlags(flags.quiet, flags.detach); err != nil {
				return err
			}
			tools, err := loadCustomTools(app, command, &customFlags)
			if err != nil {
				return err
			}
			allowedJSONFlags := append([]string{"quiet", "detach"}, customToolFlagNames...)
			request, raw, err := jsonRequest(
				app,
				command,
				args,
				jsonValue,
				allowedJSONFlags...,
			)
			if err != nil {
				return err
			}
			if !raw {
				if len(args) < 1 || len(args) > 2 {
					return fmt.Errorf("agent send requires an agent ID and optional message text")
				}
				request, err = buildSendRequest(app, command, args[0], args[1:], &flags, &shared)
				if err != nil {
					return err
				}
			}
			agentID, _ := request["agentId"].(string)
			return withCustomToolServer(app, command, tools, func() error {
				return runStreamRPC(
					app,
					command,
					"Send",
					request,
					streamOutputOptions{quiet: flags.quiet, detach: flags.detach, agentID: agentID},
				)
			})
		},
	}
	addSharedSendFlags(command, &shared)
	addSendFlags(command, &flags)
	addCustomToolFlags(command, &customFlags)
	addJSONFlag(command, &jsonValue)
	return command
}

func buildSendRequest(
	app *App,
	command *cobra.Command,
	agentID string,
	textArgs []string,
	flags *sendFlags,
	shared *sharedSendFlags,
) (map[string]any, error) {
	message, err := buildUserMessage(app, command, textArgs, flags)
	if err != nil {
		return nil, err
	}
	request := map[string]any{
		"agentId": agentID,
		"message": message,
	}
	options, err := buildSendOptions(
		app,
		command,
		flags,
		shared.model,
		shared.mode,
		shared.mcpConfig,
	)
	if err != nil {
		return nil, err
	}
	if len(options) != 0 {
		request["options"] = options
	}
	if command.Flags().Changed("idempotency-key") {
		request["idempotencyKey"] = shared.idempotencyKey
	}
	return request, nil
}

func buildUserMessage(
	app *App,
	command *cobra.Command,
	textArgs []string,
	flags *sendFlags,
) (map[string]any, error) {
	if len(textArgs) > 1 {
		return nil, fmt.Errorf("message text must be a single positional argument")
	}
	if len(textArgs) == 1 && command.Flags().Changed("message-file") {
		return nil, fmt.Errorf("cannot combine message text with --message-file")
	}
	text := ""
	if len(textArgs) == 1 {
		text = textArgs[0]
	}
	if command.Flags().Changed("message-file") {
		data, err := readInputFile(app, flags.messageFile)
		if err != nil {
			return nil, err
		}
		text = string(data)
	}
	message := map[string]any{"text": text}
	if command.Flags().Changed("image") {
		images := make([]map[string]any, len(flags.images))
		for index, value := range flags.images {
			image, err := readImage(value)
			if err != nil {
				return nil, err
			}
			images[index] = image
		}
		message["images"] = images
	}
	if text == "" && len(flags.images) == 0 {
		return nil, fmt.Errorf("message text or at least one --image is required")
	}
	return message, nil
}

func buildSendOptions(
	app *App,
	command *cobra.Command,
	flags *sendFlags,
	model string,
	modeValue string,
	mcpConfig string,
) (map[string]any, error) {
	options := map[string]any{}
	if command.Flags().Changed("model") {
		options["model"] = map[string]any{"id": model}
	}
	if command.Flags().Changed("mode") {
		// proto/sdk/v1/sdk_messages.proto defines AGENT_MODE_OPTION_* literals.
		mode, err := prefixedEnum(modeValue, "AGENT_MODE_OPTION_", "AGENT", "PLAN")
		if err != nil {
			return nil, err
		}
		options["mode"] = mode
	}
	if command.Flags().Changed("mcp-config") {
		config, err := app.ReadJSONPayload(mcpConfig)
		if err != nil {
			return nil, fmt.Errorf("read --mcp-config: %w", err)
		}
		options["mcpServers"] = config
	}
	if command.Flags().Changed("force") {
		options["local"] = map[string]any{"force": flags.force}
	}
	if command.Flags().Changed("send-env-var") {
		values, err := parseKeyValues(flags.envVars)
		if err != nil {
			return nil, fmt.Errorf("parse --send-env-var: %w", err)
		}
		options["cloud"] = map[string]any{"envVars": values}
	}
	if command.Flags().Changed("deltas") {
		options["enableDeltas"] = flags.deltas
	}
	if command.Flags().Changed("steps") {
		options["enableSteps"] = flags.steps
	}
	return options, nil
}

func readInputFile(app *App, path string) ([]byte, error) {
	if path == "-" {
		data, err := io.ReadAll(app.In)
		if err != nil {
			return nil, fmt.Errorf("read message from stdin: %w", err)
		}
		return data, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read message file %s: %w", path, err)
	}
	return data, nil
}

func readImage(value string) (map[string]any, error) {
	parsed, err := url.Parse(value)
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" {
		return map[string]any{"url": map[string]any{"url": value}}, nil
	}
	data, err := os.ReadFile(value)
	if err != nil {
		return nil, fmt.Errorf("read image %s: %w", value, err)
	}
	return map[string]any{
		"data": map[string]any{
			"data":     base64.StdEncoding.EncodeToString(data),
			"mimeType": http.DetectContentType(data),
		},
	}, nil
}

func validateStreamOutputFlags(quiet bool, detach bool) error {
	if quiet && detach {
		return fmt.Errorf("cannot combine --quiet with --detach")
	}
	return nil
}
