package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newAgentCommand(app *App) *cobra.Command {
	command := &cobra.Command{
		Use:   "agent",
		Short: "Call SdkAgentService agent RPCs",
	}
	command.AddCommand(
		newAgentCreateCommand(app),
		newAgentResumeCommand(app),
		newAgentSendCommand(app),
		newAgentPromptCommand(app),
		newAgentGetCommand(app),
		newAgentListCommand(app),
		newAgentMessagesCommand(app),
		newAgentUsageCommand(app),
		newAgentOperationCommand(app, "archive", "ArchiveAgent", false),
		newAgentOperationCommand(app, "unarchive", "UnarchiveAgent", false),
		newAgentOperationCommand(app, "delete", "DeleteAgent", true),
		newAgentSimpleCommand(app, "close", "CloseAgent"),
		newAgentSimpleCommand(app, "reload", "ReloadAgent"),
	)
	return command
}

func newAgentCreateCommand(app *App) *cobra.Command {
	var flags agentOptionFlags
	var customFlags customToolFlags
	var jsonValue string
	command := &cobra.Command{
		Use:   "create",
		Short: "Create an agent with CreateAgent",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			tools, err := loadCustomTools(app, command, &customFlags)
			if err != nil {
				return err
			}
			request, err := buildUnaryRequest(
				app,
				command,
				args,
				jsonValue,
				0,
				"agent create",
				true,
				customToolFlagNames,
				func(_ []string, apiKey string) (map[string]any, error) {
					options, err := buildAgentOptions(app, command, &flags, apiKey)
					if err != nil {
						return nil, err
					}
					request := map[string]any{"options": options}
					if command.Flags().Changed("idempotency-key") {
						request["idempotencyKey"] = flags.idempotencyKey
					}
					return request, nil
				},
			)
			if err != nil {
				return err
			}
			if len(tools.registry) != 0 {
				if err := injectAgentCustomTools(request, tools.declarations); err != nil {
					return err
				}
			}
			return runRPC[map[string]any](
				app,
				command,
				"SdkAgentService",
				"CreateAgent",
				request,
			)
		},
	}
	addAgentOptionFlags(command, &flags)
	addIdempotencyKeyFlag(command, &flags.idempotencyKey, "Idempotency key for agent creation")
	addCustomToolFlags(command, &customFlags)
	addJSONFlag(command, &jsonValue)
	return command
}

func newAgentResumeCommand(app *App) *cobra.Command {
	var flags agentOptionFlags
	var jsonValue string
	command := &cobra.Command{
		Use:   "resume <agent-id>",
		Short: "Resume an existing agent with ResumeAgent",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			return runUnaryCommand(
				app, command, args, jsonValue,
				unaryCommandSpec{
					method:        "ResumeAgent",
					argCount:      1,
					use:           "agent resume",
					injectOptions: true,
					build: func(args []string, apiKey string) (map[string]any, error) {
						options, err := buildAgentOptions(app, command, &flags, apiKey)
						if err != nil {
							return nil, err
						}
						return map[string]any{"agentId": args[0], "options": options}, nil
					},
				},
				nil,
			)
		},
	}
	addAgentOptionFlags(command, &flags)
	addJSONFlag(command, &jsonValue)
	return command
}

func newAgentGetCommand(app *App) *cobra.Command {
	var cwd string
	var jsonValue string
	command := &cobra.Command{
		Use:   "get <agent-id>",
		Short: "Get agent details with GetAgent",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			return runUnaryCommand(
				app, command, args, jsonValue,
				unaryCommandSpec{
					method:        "GetAgent",
					argCount:      1,
					use:           "agent get",
					injectOptions: true,
					build: func(args []string, apiKey string) (map[string]any, error) {
						options := map[string]any{"apiKey": apiKey}
						if command.Flags().Changed("cwd") {
							options["cwd"] = cwd
						}
						return map[string]any{"agentId": args[0], "options": options}, nil
					},
				},
				nil,
			)
		},
	}
	command.Flags().StringVar(&cwd, "cwd", "", "Local agent working directory")
	addJSONFlag(command, &jsonValue)
	return command
}

func newAgentListCommand(app *App) *cobra.Command {
	var limit uint32
	var cursor string
	var runtime string
	var cwd string
	var prURL string
	var includeArchived bool
	var jsonValue string
	command := &cobra.Command{
		Use:   "list",
		Short: "List agents with ListAgents",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			return runUnaryCommand(
				app, command, args, jsonValue,
				unaryCommandSpec{
					method:        "ListAgents",
					argCount:      0,
					use:           "agent list",
					injectOptions: true,
					build: func(_ []string, apiKey string) (map[string]any, error) {
						options := map[string]any{"apiKey": apiKey}
						if command.Flags().Changed("limit") {
							options["limit"] = limit
						}
						if command.Flags().Changed("cursor") {
							options["cursor"] = cursor
						}
						if err := setRuntimeOption(command, options, runtime); err != nil {
							return nil, err
						}
						if command.Flags().Changed("cwd") {
							options["cwd"] = cwd
						}
						if command.Flags().Changed("pr-url") {
							options["prUrl"] = prURL
						}
						if command.Flags().Changed("include-archived") {
							options["includeArchived"] = includeArchived
						}
						return map[string]any{"options": options}, nil
					},
				},
				nil,
			)
		},
	}
	set := command.Flags()
	set.Uint32Var(&limit, "limit", 0, "Maximum number of agents")
	set.StringVar(&cursor, "cursor", "", "Pagination cursor")
	set.StringVar(&runtime, "runtime", "", "Runtime filter: local or cloud")
	set.StringVar(&cwd, "cwd", "", "Local working directory filter")
	set.StringVar(&prURL, "pr-url", "", "Pull request URL filter")
	set.BoolVar(&includeArchived, "include-archived", false, "Include archived agents")
	addJSONFlag(command, &jsonValue)
	return command
}

func newAgentOperationCommand(
	app *App,
	name string,
	method string,
	requireForce bool,
) *cobra.Command {
	var cwd string
	var force bool
	var jsonValue string
	command := &cobra.Command{
		Use:   name + " <agent-id>",
		Short: strings.ToUpper(name[:1]) + name[1:] + " an agent with " + method,
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			if requireForce && !force {
				return fmt.Errorf("refusing to delete without --force")
			}
			var allowed []string
			if requireForce {
				allowed = []string{"force"}
			}
			return runUnaryCommand(
				app, command, args, jsonValue,
				unaryCommandSpec{
					method:        method,
					argCount:      1,
					use:           "agent " + name,
					injectOptions: true,
					build: func(args []string, apiKey string) (map[string]any, error) {
						options := map[string]any{"apiKey": apiKey}
						if command.Flags().Changed("cwd") {
							options["cwd"] = cwd
						}
						return map[string]any{"agentId": args[0], "options": options}, nil
					},
				},
				allowed,
			)
		},
	}
	command.Flags().StringVar(&cwd, "cwd", "", "Local agent working directory")
	if requireForce {
		command.Flags().BoolVar(&force, "force", false, "Confirm irreversible deletion")
	}
	addJSONFlag(command, &jsonValue)
	return command
}

func newAgentSimpleCommand(app *App, name string, method string) *cobra.Command {
	var jsonValue string
	command := &cobra.Command{
		Use:   name + " <agent-id>",
		Short: strings.ToUpper(name[:1]) + name[1:] + " an agent with " + method,
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			return runUnaryCommand(
				app, command, args, jsonValue,
				unaryCommandSpec{
					method:   method,
					argCount: 1,
					use:      "agent " + name,
					build: func(args []string, _ string) (map[string]any, error) {
						return map[string]any{"agentId": args[0]}, nil
					},
				},
				nil,
			)
		},
	}
	addJSONFlag(command, &jsonValue)
	return command
}
