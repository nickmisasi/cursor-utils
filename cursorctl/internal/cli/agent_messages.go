package cli

import "github.com/spf13/cobra"

func newAgentMessagesCommand(app *App) *cobra.Command {
	var limit uint32
	var offset uint32
	var runtime string
	var cwd string
	var jsonValue string
	command := &cobra.Command{
		Use:   "messages <agent-id>",
		Short: "List agent messages with ListAgentMessages",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			return runUnaryCommand(
				app, command, args, jsonValue,
				unaryCommandSpec{
					method:        "ListAgentMessages",
					argCount:      1,
					use:           "agent messages",
					injectOptions: true,
					build: func(args []string, apiKey string) (map[string]any, error) {
						options := map[string]any{"apiKey": apiKey}
						if command.Flags().Changed("limit") {
							options["limit"] = limit
						}
						if command.Flags().Changed("offset") {
							options["offset"] = offset
						}
						if err := setRuntimeOption(command, options, runtime); err != nil {
							return nil, err
						}
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
	set := command.Flags()
	set.Uint32Var(&limit, "limit", 0, "Maximum number of messages")
	set.Uint32Var(&offset, "offset", 0, "Message offset")
	set.StringVar(&runtime, "runtime", "", "Runtime: local or cloud")
	set.StringVar(&cwd, "cwd", "", "Local working directory")
	addJSONFlag(command, &jsonValue)
	return command
}

func newAgentUsageCommand(app *App) *cobra.Command {
	var runID string
	var jsonValue string
	command := &cobra.Command{
		Use:   "usage <agent-id>",
		Short: "Get cloud agent usage with GetUsage",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			return runUnaryCommand(
				app, command, args, jsonValue,
				unaryCommandSpec{
					method:   "GetUsage",
					argCount: 1,
					use:      "agent usage",
					build: func(args []string, _ string) (map[string]any, error) {
						request := map[string]any{"agentId": args[0]}
						if command.Flags().Changed("run-id") {
							request["runId"] = runID
						}
						return request, nil
					},
				},
				nil,
			)
		},
	}
	command.Flags().StringVar(&runID, "run-id", "", "Limit usage to one run")
	addJSONFlag(command, &jsonValue)
	return command
}
