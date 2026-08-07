package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newAgentCommand(app *App) *cobra.Command {
	command := &cobra.Command{
		Use:   "agent",
		Short: "Create and manage Cursor agents",
	}
	command.AddCommand(
		newAgentCreateCommand(app),
		newAgentResumeCommand(app),
		newAgentSendCommand(app),
		newAgentPromptCommand(app),
		newAgentGetCommand(app),
		newAgentListCommand(app),
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
	var jsonValue string
	command := &cobra.Command{
		Use:   "create",
		Short: "Create an agent",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			request, raw, err := jsonRequest(
				app,
				command,
				args,
				jsonValue,
				agentOptionRequestFlags...,
			)
			if err != nil {
				return err
			}
			apiKey, err := app.ResolvedAPIKey()
			if err != nil {
				return err
			}
			if raw {
				if err := injectAPIKey(request, "options", apiKey); err != nil {
					return err
				}
			} else {
				if err := requireArgs(args, 0, "agent create"); err != nil {
					return err
				}
				options, err := buildAgentOptions(app, command, &flags, apiKey)
				if err != nil {
					return err
				}
				request = map[string]any{"options": options}
				if command.Flags().Changed("idempotency-key") {
					request["idempotencyKey"] = flags.idempotencyKey
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
	addJSONFlag(command, &jsonValue)
	return command
}

func newAgentResumeCommand(app *App) *cobra.Command {
	var flags agentOptionFlags
	var jsonValue string
	command := &cobra.Command{
		Use:   "resume <agent-id>",
		Short: "Resume an existing agent",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			request, raw, err := jsonRequest(
				app,
				command,
				args,
				jsonValue,
				agentOptionRequestFlags...,
			)
			if err != nil {
				return err
			}
			apiKey, err := app.ResolvedAPIKey()
			if err != nil {
				return err
			}
			if raw {
				if err := injectAPIKey(request, "options", apiKey); err != nil {
					return err
				}
			} else {
				if err := requireArgs(args, 1, "agent resume"); err != nil {
					return err
				}
				if command.Flags().Changed("idempotency-key") {
					return fmt.Errorf("--idempotency-key is not supported by ResumeAgent")
				}
				options, err := buildAgentOptions(app, command, &flags, apiKey)
				if err != nil {
					return err
				}
				request = map[string]any{"agentId": args[0], "options": options}
			}
			return runRPC[map[string]any](
				app,
				command,
				"SdkAgentService",
				"ResumeAgent",
				request,
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
		Short: "Get agent details",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			request, raw, err := jsonRequest(app, command, args, jsonValue, "cwd")
			if err != nil {
				return err
			}
			apiKey, err := app.ResolvedAPIKey()
			if err != nil {
				return err
			}
			if raw {
				if err := injectAPIKey(request, "options", apiKey); err != nil {
					return err
				}
			} else {
				if err := requireArgs(args, 1, "agent get"); err != nil {
					return err
				}
				options := map[string]any{"apiKey": apiKey}
				if command.Flags().Changed("cwd") {
					options["cwd"] = cwd
				}
				request = map[string]any{"agentId": args[0], "options": options}
			}
			return runRPC[map[string]any](app, command, "SdkAgentService", "GetAgent", request)
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
	requestFlags := []string{"limit", "cursor", "runtime", "cwd", "pr-url", "include-archived"}
	command := &cobra.Command{
		Use:   "list",
		Short: "List agents",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			request, raw, err := jsonRequest(app, command, args, jsonValue, requestFlags...)
			if err != nil {
				return err
			}
			apiKey, err := app.ResolvedAPIKey()
			if err != nil {
				return err
			}
			if raw {
				if err := injectAPIKey(request, "options", apiKey); err != nil {
					return err
				}
			} else {
				if err := requireArgs(args, 0, "agent list"); err != nil {
					return err
				}
				options := map[string]any{"apiKey": apiKey}
				if command.Flags().Changed("limit") {
					options["limit"] = limit
				}
				if command.Flags().Changed("cursor") {
					options["cursor"] = cursor
				}
				if command.Flags().Changed("runtime") {
					value, err := prefixedEnum(runtime, "RUNTIME_", "LOCAL", "CLOUD")
					if err != nil {
						return err
					}
					options["runtime"] = value
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
				request = map[string]any{"options": options}
			}
			return runRPC[map[string]any](app, command, "SdkAgentService", "ListAgents", request)
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
		Short: name + " an agent",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			if requireForce && !force {
				return fmt.Errorf("refusing to delete without --force")
			}
			request, raw, err := jsonRequest(app, command, args, jsonValue, "cwd")
			if err != nil {
				return err
			}
			apiKey, err := app.ResolvedAPIKey()
			if err != nil {
				return err
			}
			if raw {
				if err := injectAPIKey(request, "options", apiKey); err != nil {
					return err
				}
			} else {
				if err := requireArgs(args, 1, "agent "+name); err != nil {
					return err
				}
				options := map[string]any{"apiKey": apiKey}
				if command.Flags().Changed("cwd") {
					options["cwd"] = cwd
				}
				request = map[string]any{"agentId": args[0], "options": options}
			}
			return runRPC[map[string]any](app, command, "SdkAgentService", method, request)
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
		Short: name + " an agent",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			request, raw, err := jsonRequest(app, command, args, jsonValue)
			if err != nil {
				return err
			}
			if !raw {
				if err := requireArgs(args, 1, "agent "+name); err != nil {
					return err
				}
				request = map[string]any{"agentId": args[0]}
			}
			return runRPC[map[string]any](app, command, "SdkAgentService", method, request)
		},
	}
	addJSONFlag(command, &jsonValue)
	return command
}
