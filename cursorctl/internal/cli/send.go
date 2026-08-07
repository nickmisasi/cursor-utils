package cli

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/nickmisasi/cursor-utils/cursorctl/internal/bridge"
	"github.com/spf13/cobra"
)

type sendFlags struct {
	model          string
	mode           string
	mcpConfig      string
	force          bool
	envVars        []string
	deltas         bool
	steps          bool
	idempotencyKey string
	messageFile    string
	images         []string
	quiet          bool
	detach         bool
}

var sendRequestFlags = []string{
	"message-file", "image", "model", "mode", "mcp-config", "force",
	"send-env-var", "deltas", "steps", "idempotency-key",
}

func addSendFlags(command *cobra.Command, flags *sendFlags, shared bool) {
	set := command.Flags()
	set.StringVar(&flags.messageFile, "message-file", "", "Read message text from a file or - for stdin")
	set.StringArrayVar(&flags.images, "image", nil, "Image path or http(s) URL (repeatable)")
	if !shared {
		set.StringVar(&flags.model, "model", "", "Model identifier for this send")
		set.StringVar(&flags.mode, "mode", "", "Conversation mode: agent or plan")
		set.StringVar(&flags.mcpConfig, "mcp-config", "", "MCP server map as JSON, @file, or -")
		set.StringVar(&flags.idempotencyKey, "idempotency-key", "", "Idempotency key for this send")
	}
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
	var flags sendFlags
	var jsonValue string
	command := &cobra.Command{
		Use:   "send <agent-id> [text]",
		Short: "Send a message and stream the run",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			if err := validateStreamOutputFlags(flags.quiet, flags.detach); err != nil {
				return err
			}
			request, raw, err := jsonRequest(
				app,
				command,
				args,
				jsonValue,
				sendRequestFlags...,
			)
			if err != nil {
				return err
			}
			if !raw {
				if len(args) < 1 || len(args) > 2 {
					return fmt.Errorf("agent send requires an agent ID and optional message text")
				}
				request, err = buildSendRequest(app, command, args[0], args[1:], &flags)
				if err != nil {
					return err
				}
			}
			agentID, _ := request["agentId"].(string)
			return runStreamRPC(
				app,
				command,
				"Send",
				request,
				streamOutputOptions{quiet: flags.quiet, detach: flags.detach, agentID: agentID},
			)
		},
	}
	addSendFlags(command, &flags, false)
	addJSONFlag(command, &jsonValue)
	return command
}

func buildSendRequest(
	app *App,
	command *cobra.Command,
	agentID string,
	textArgs []string,
	flags *sendFlags,
) (map[string]any, error) {
	message, err := buildUserMessage(app, command, textArgs, flags)
	if err != nil {
		return nil, err
	}
	request := map[string]any{
		"agentId": agentID,
		"message": message,
	}
	options, err := buildSendOptions(app, command, flags)
	if err != nil {
		return nil, err
	}
	if len(options) != 0 {
		request["options"] = options
	}
	if command.Flags().Changed("idempotency-key") {
		request["idempotencyKey"] = flags.idempotencyKey
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
) (map[string]any, error) {
	options := map[string]any{}
	if command.Flags().Changed("model") {
		options["model"] = map[string]any{"id": flags.model}
	}
	if command.Flags().Changed("mode") {
		mode, err := prefixedEnum(flags.mode, "AGENT_MODE_OPTION_", "AGENT", "PLAN")
		if err != nil {
			return nil, err
		}
		options["mode"] = mode
	}
	if command.Flags().Changed("mcp-config") {
		config, err := app.ReadJSONPayload(flags.mcpConfig)
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

func runStreamRPC(
	app *App,
	command *cobra.Command,
	method string,
	request map[string]any,
	options streamOutputOptions,
) error {
	if err := prepareCommand(app, command); err != nil {
		return err
	}
	client, err := app.Client(command.Context())
	if err != nil {
		return err
	}
	reader, err := client.Stream(
		command.Context(),
		"SdkAgentService",
		method,
		request,
	)
	if err != nil {
		return err
	}
	_, err = consumeRunStream(app, reader, options)
	return err
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
