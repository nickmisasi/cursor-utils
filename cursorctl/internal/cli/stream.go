package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/nickmisasi/cursor-utils/cursorctl/internal/bridge"
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
) (map[string]any, error) {
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
				return nil, fmt.Errorf("run stream ended unexpectedly: %w", err)
			}
			return nil, err
		}

		eventName, payload, ok, err := decodeStreamEnvelope(message)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if eventName == "result" {
			value, exists := payload["result"].(map[string]any)
			if !exists {
				return nil, fmt.Errorf("run result envelope is missing result")
			}
			finalResult = value
		}

		if options.detach {
			if runID := findRunID(payload); runID != "" {
				return nil, app.Print(map[string]any{
					"agentId": options.agentID,
					"runId":   runID,
				})
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
					return nil, fmt.Errorf("decode stream offset: %w", err)
				}
				output["offset"] = offset
			}
			if err := encoder.Encode(output); err != nil {
				return nil, fmt.Errorf("write stream event: %w", err)
			}
		}
	}

	if options.detach {
		return nil, fmt.Errorf("run stream ended before reporting a run ID")
	}
	if finalResult == nil {
		return nil, fmt.Errorf("run stream ended without a result")
	}
	if options.quiet {
		if err := app.Print(finalResult); err != nil {
			return nil, err
		}
	}
	return finalResult, runResultError(finalResult)
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

func findRunID(value any) string {
	switch value := value.(type) {
	case map[string]any:
		if runID, ok := value["runId"].(string); ok && runID != "" {
			return runID
		}
		if runID, ok := value["run_id"].(string); ok && runID != "" {
			return runID
		}
		for _, item := range value {
			if runID := findRunID(item); runID != "" {
				return runID
			}
		}
	case []any:
		for _, item := range value {
			if runID := findRunID(item); runID != "" {
				return runID
			}
		}
	case nil, bool, string, float64:
	default:
		return ""
	}
	return ""
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
