package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func newRunCommand(app *App) *cobra.Command {
	command := &cobra.Command{
		Use:   "run",
		Short: "Call SdkAgentService run RPCs",
	}
	command.AddCommand(
		newRunGetCommand(app),
		newRunListCommand(app),
		newRunWaitCommand(app),
		newRunWatchCommand(app),
		newRunCancelCommand(app),
		newRunConversationCommand(app),
	)
	return command
}

func newRunGetCommand(app *App) *cobra.Command {
	var runtime string
	var cwd string
	var agentID string
	var jsonValue string
	command := &cobra.Command{
		Use:   "get <run-id>",
		Short: "Get a run snapshot with GetRun",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			return runUnaryCommand(
				app, command, args, jsonValue,
				unaryCommandSpec{
					method:        "GetRun",
					argCount:      1,
					use:           "run get",
					injectOptions: true,
					build: func(args []string, apiKey string) (map[string]any, error) {
						options, err := getRunOptions(command, runtime, cwd, agentID, apiKey)
						if err != nil {
							return nil, err
						}
						return map[string]any{"runId": args[0], "options": options}, nil
					},
				},
				nil,
			)
		},
	}
	set := command.Flags()
	set.StringVar(&runtime, "runtime", "", "Runtime: local or cloud")
	set.StringVar(&cwd, "cwd", "", "Local working directory")
	set.StringVar(&agentID, "agent-id", "", "Agent routing hint")
	addJSONFlag(command, &jsonValue)
	return command
}

func newRunListCommand(app *App) *cobra.Command {
	var limit uint32
	var cursor string
	var runtime string
	var cwd string
	var jsonValue string
	command := &cobra.Command{
		Use:   "list <agent-id>",
		Short: "List runs for an agent with ListRuns",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			return runUnaryCommand(
				app, command, args, jsonValue,
				unaryCommandSpec{
					method:        "ListRuns",
					argCount:      1,
					use:           "run list",
					injectOptions: true,
					build: func(args []string, apiKey string) (map[string]any, error) {
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
						return map[string]any{"agentId": args[0], "options": options}, nil
					},
				},
				nil,
			)
		},
	}
	set := command.Flags()
	set.Uint32Var(&limit, "limit", 0, "Maximum number of runs")
	set.StringVar(&cursor, "cursor", "", "Pagination cursor")
	set.StringVar(&runtime, "runtime", "", "Runtime: local or cloud")
	set.StringVar(&cwd, "cwd", "", "Local working directory")
	addJSONFlag(command, &jsonValue)
	return command
}

func newRunWaitCommand(app *App) *cobra.Command {
	var jsonValue string
	command := &cobra.Command{
		Use:   "wait <run-id>",
		Short: "Wait for a run to finish with WaitLiveRun",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			request, err := buildUnaryRequest(
				app, command, args, jsonValue, 1, "run wait", false, nil,
				func(args []string, _ string) (map[string]any, error) {
					return map[string]any{"runId": args[0]}, nil
				},
			)
			if err != nil {
				return err
			}
			if err := prepareCommand(app, command); err != nil {
				return err
			}
			client, err := app.Client(command.Context())
			if err != nil {
				return err
			}
			var response struct {
				Result map[string]any `json:"result"`
			}
			if err := client.Call(
				command.Context(),
				"SdkAgentService",
				"WaitLiveRun",
				request,
				&response,
			); err != nil {
				return err
			}
			if err := app.Print(response.Result); err != nil {
				return err
			}
			return runResultError(response.Result)
		},
	}
	addJSONFlag(command, &jsonValue)
	return command
}

func newRunWatchCommand(app *App) *cobra.Command {
	var afterOffset string
	var quiet bool
	var jsonValue string
	command := &cobra.Command{
		Use:   "watch <run-id>",
		Short: "Stream durable run events with ObserveRun",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			request, raw, err := jsonRequest(app, command, args, jsonValue, "quiet")
			if err != nil {
				return err
			}
			if !raw {
				if err := requireArgs(args, 1, "run watch"); err != nil {
					return err
				}
				request = map[string]any{"runId": args[0]}
				if command.Flags().Changed("after-offset") {
					request["afterOffset"] = afterOffset
				}
			}
			return runStreamRPC(
				app,
				command,
				"ObserveRun",
				request,
				streamOutputOptions{quiet: quiet},
			)
		},
	}
	command.Flags().StringVar(&afterOffset, "after-offset", "", "Resume after an ObserveRun offset")
	command.Flags().BoolVar(&quiet, "quiet", false, "Suppress events and print only the final run result")
	addJSONFlag(command, &jsonValue)
	return command
}

func newRunCancelCommand(app *App) *cobra.Command {
	var agentID string
	var jsonValue string
	command := &cobra.Command{
		Use:   "cancel <run-id>",
		Short: "Cancel a run with CancelRun",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			return runUnaryCommand(
				app, command, args, jsonValue,
				unaryCommandSpec{
					method:   "CancelRun",
					argCount: 1,
					use:      "run cancel",
					build: func(args []string, _ string) (map[string]any, error) {
						request := map[string]any{"runId": args[0]}
						if command.Flags().Changed("agent-id") {
							request["agentId"] = agentID
						}
						return request, nil
					},
				},
				nil,
			)
		},
	}
	command.Flags().StringVar(&agentID, "agent-id", "", "Agent routing hint")
	addJSONFlag(command, &jsonValue)
	return command
}

func newRunConversationCommand(app *App) *cobra.Command {
	var jsonValue string
	command := &cobra.Command{
		Use:   "conversation <run-id>",
		Short: "Get a run conversation with GetRunConversation",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			request, err := buildUnaryRequest(
				app, command, args, jsonValue, 1, "run conversation", false, nil,
				func(args []string, _ string) (map[string]any, error) {
					return map[string]any{"runId": args[0]}, nil
				},
			)
			if err != nil {
				return err
			}
			if err := prepareCommand(app, command); err != nil {
				return err
			}
			client, err := app.Client(command.Context())
			if err != nil {
				return err
			}
			var response struct {
				ConversationJSON string `json:"conversationJson"`
			}
			if err := client.Call(
				command.Context(),
				"SdkAgentService",
				"GetRunConversation",
				request,
				&response,
			); err != nil {
				return err
			}
			value, err := decodeJSONValue(response.ConversationJSON)
			if err != nil {
				return fmt.Errorf("decode conversationJson: %w", err)
			}
			return app.Print(value)
		},
	}
	addJSONFlag(command, &jsonValue)
	return command
}

func getRunOptions(
	command *cobra.Command,
	runtime string,
	cwd string,
	agentID string,
	apiKey string,
) (map[string]any, error) {
	options := map[string]any{"apiKey": apiKey}
	if err := setRuntimeOption(command, options, runtime); err != nil {
		return nil, err
	}
	if command.Flags().Changed("cwd") {
		options["cwd"] = cwd
	}
	if command.Flags().Changed("agent-id") {
		options["agentId"] = agentID
	}
	return options, nil
}

func decodeJSONValue(value string) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader([]byte(value)))
	decoder.UseNumber()
	var result any
	if err := decoder.Decode(&result); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values")
		}
		return nil, err
	}
	return result, nil
}
