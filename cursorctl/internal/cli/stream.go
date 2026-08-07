package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/nickmisasi/cursor-utils/cursorctl/internal/bridge"
	"github.com/spf13/cobra"
)

type streamOutputOptions struct {
	quiet   bool
	detach  bool
	agentID string
}

func consumeRunStream(
	app *App,
	reader *bridge.StreamReader,
	options streamOutputOptions,
) error {
	defer reader.Close()
	encoder := json.NewEncoder(app.Out)
	var finalResult map[string]any

	for {
		var message map[string]json.RawMessage
		err := reader.Next(&message)
		if err == io.EOF {
			break
		}
		if err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				return fmt.Errorf("run stream ended unexpectedly: %w", err)
			}
			return err
		}

		eventName, payload, ok, err := decodeStreamEnvelope(message)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if eventName == "result" {
			// proto/sdk/v1/sdk_messages.proto nests RunStreamResult.result.
			value, exists := payload["result"].(map[string]any)
			if !exists {
				return fmt.Errorf("run result envelope is missing result")
			}
			finalResult = value
		}

		if options.detach {
			if runID := streamRunID(eventName, payload); runID != "" {
				output := map[string]any{"runId": runID}
				if options.agentID != "" {
					output["agentId"] = options.agentID
				}
				return app.Print(output)
			}
			continue
		}
		if !options.quiet {
			output := map[string]any{"event": eventName}
			for key, value := range payload {
				output[key] = value
			}
			if rawOffset, exists := message["offset"]; exists {
				var offset string
				if err := json.Unmarshal(rawOffset, &offset); err != nil {
					return fmt.Errorf("decode stream offset: %w", err)
				}
				output["offset"] = offset
			}
			if err := encoder.Encode(output); err != nil {
				return fmt.Errorf("write stream event: %w", err)
			}
		}
	}

	if options.detach {
		return fmt.Errorf("run stream ended before reporting a run ID")
	}
	if finalResult == nil {
		return fmt.Errorf("run stream ended without a result")
	}
	if options.quiet {
		if err := app.Print(finalResult); err != nil {
			return err
		}
	}
	return runResultError(finalResult)
}

func decodeStreamEnvelope(
	message map[string]json.RawMessage,
) (string, map[string]any, bool, error) {
	envelopes := []string{"sdkMessage", "result", "done", "interactionUpdate", "step"}
	for _, name := range envelopes {
		raw, exists := message[name]
		if !exists || string(raw) == "null" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			return "", nil, false, fmt.Errorf("decode %s stream envelope: %w", name, err)
		}
		return name, payload, true, nil
	}
	return "", nil, false, nil
}

func streamRunID(eventName string, payload map[string]any) string {
	switch eventName {
	case "sdkMessage":
		if payload["type"] != "system" {
			return ""
		}
		message, ok := payload["message"].(map[string]any)
		if !ok || message["subtype"] != "init" {
			return ""
		}
		runID, _ := message["runId"].(string)
		return runID
	case "result", "done":
		runID, _ := payload["runId"].(string)
		return runID
	case "interactionUpdate", "step":
		return ""
	default:
		return ""
	}
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
	return consumeRunStream(app, reader, options)
}

func runResultError(result map[string]any) error {
	statusValue, ok := result["status"].(string)
	if !ok || statusValue == "" {
		return fmt.Errorf("run result is missing status")
	}
	status := strings.TrimPrefix(statusValue, "RUN_LIFECYCLE_STATUS_")
	switch status {
	case "FINISHED":
		return nil
	case "ERROR", "CANCELLED", "EXPIRED":
		return &ExitError{
			Code: ExitAgentFailure,
			Err:  fmt.Errorf("run ended with status %s", status),
		}
	case "CREATING", "RUNNING":
		return fmt.Errorf("run returned non-terminal status %s", status)
	default:
		return fmt.Errorf("run returned unknown status %q", statusValue)
	}
}
