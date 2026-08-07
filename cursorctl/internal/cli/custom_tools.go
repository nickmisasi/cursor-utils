package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nickmisasi/cursor-utils/cursorctl/internal/toolserver"
	"github.com/spf13/cobra"
)

type customToolFlags struct {
	entries []string
	config  string
}

type customToolSet struct {
	registry     map[string]toolserver.Tool
	declarations map[string]any
}

var customToolFlagNames = []string{"custom-tool", "custom-tool-config"}

func addCustomToolFlags(command *cobra.Command, flags *customToolFlags) {
	command.Flags().StringArrayVar(
		&flags.entries,
		"custom-tool",
		nil,
		"Tool NAME=COMMAND (repeatable); on agent send, solely starts the callback executor for tools declared at create",
	)
	command.Flags().StringVar(
		&flags.config,
		"custom-tool-config",
		"",
		"Tool map {name:{description,inputSchema,command}} as JSON, @file, or -; on agent send, solely starts callback executors for tools declared at create",
	)
}

func loadCustomTools(app *App, command *cobra.Command, flags *customToolFlags) (customToolSet, error) {
	set := customToolSet{
		registry:     map[string]toolserver.Tool{},
		declarations: map[string]any{},
	}
	if command.Flags().Changed("custom-tool-config") {
		config, err := app.ReadJSONPayload(flags.config)
		if err != nil {
			return set, fmt.Errorf("read --custom-tool-config: %w", err)
		}
		for name, raw := range config {
			tool, err := decodeConfiguredTool(name, raw)
			if err != nil {
				return set, err
			}
			set.registry[name] = tool
		}
	}
	for _, entry := range flags.entries {
		name, commandValue, ok := strings.Cut(entry, "=")
		if !ok || name == "" || commandValue == "" {
			return set, fmt.Errorf("invalid --custom-tool %q (expected NAME=COMMAND)", entry)
		}
		tool, exists := set.registry[name]
		if !exists {
			tool = toolserver.Tool{
				InputSchema: map[string]any{"type": "object"},
			}
		}
		tool.Command = commandValue
		set.registry[name] = tool
	}
	for name, tool := range set.registry {
		set.declarations[name] = map[string]any{
			"description": tool.Description,
			"inputSchema": tool.InputSchema,
		}
	}
	return set, nil
}

func decodeConfiguredTool(name string, raw any) (toolserver.Tool, error) {
	if name == "" {
		return toolserver.Tool{}, fmt.Errorf("custom tool name must not be empty")
	}
	value, ok := raw.(map[string]any)
	if !ok {
		return toolserver.Tool{}, fmt.Errorf("custom tool %q must be a JSON object", name)
	}
	command, ok := value["command"].(string)
	if !ok || command == "" {
		return toolserver.Tool{}, fmt.Errorf("custom tool %q requires a non-empty command", name)
	}
	description := ""
	if rawDescription, exists := value["description"]; exists {
		var descriptionOK bool
		description, descriptionOK = rawDescription.(string)
		if !descriptionOK {
			return toolserver.Tool{}, fmt.Errorf("custom tool %q description must be a string", name)
		}
	}
	schema := map[string]any{"type": "object"}
	if rawSchema, exists := value["inputSchema"]; exists {
		var schemaOK bool
		schema, schemaOK = rawSchema.(map[string]any)
		if !schemaOK {
			return toolserver.Tool{}, fmt.Errorf("custom tool %q inputSchema must be a JSON object", name)
		}
	}
	return toolserver.Tool{Description: description, InputSchema: schema, Command: command}, nil
}

func withCustomToolServer(
	app *App,
	command *cobra.Command,
	tools customToolSet,
	run func() error,
) (resultErr error) {
	if len(tools.registry) == 0 {
		return run()
	}
	if err := prepareCommand(app, command); err != nil {
		return err
	}
	server, err := toolserver.Start(tools.registry)
	if err != nil {
		return err
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Close(ctx); resultErr == nil && err != nil {
			resultErr = err
		}
	}()
	client, err := app.Client(command.Context())
	if err != nil {
		return err
	}
	var response map[string]any
	if err := client.Call(
		command.Context(),
		"SdkBridgeControlService",
		"SetToolCallback",
		map[string]any{"url": server.URL(), "authToken": server.AuthToken()},
		&response,
	); err != nil {
		return fmt.Errorf("register custom tool callback: %w", err)
	}
	return run()
}

func injectAgentCustomTools(request map[string]any, declarations map[string]any) error {
	options, err := objectField(request, "options", true)
	if err != nil {
		return err
	}
	return injectLocalCustomTools(options, declarations)
}

func injectPromptCustomTools(composite map[string]any, declarations map[string]any) error {
	return injectAgentCustomTools(composite, declarations)
}

func injectLocalCustomTools(options map[string]any, declarations map[string]any) error {
	if cloud, exists := options["cloud"]; exists && cloud != nil {
		return fmt.Errorf("custom tools are supported for local agents only")
	}
	local, err := objectField(options, "local", false)
	if err != nil {
		return err
	}
	local["customTools"] = declarations
	return nil
}

func objectField(parent map[string]any, name string, required bool) (map[string]any, error) {
	raw, exists := parent[name]
	if !exists || raw == nil {
		if required {
			return nil, fmt.Errorf("%s must be a JSON object", name)
		}
		value := map[string]any{}
		parent[name] = value
		return value, nil
	}
	value, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a JSON object", name)
	}
	return value, nil
}
