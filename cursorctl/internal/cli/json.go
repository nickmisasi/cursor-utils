package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

func ReadJSONPayload(flagValue string) (map[string]any, error) {
	return readJSONPayload(flagValue, os.Stdin)
}

func readJSONPayload(flagValue string, stdin io.Reader) (map[string]any, error) {
	var data []byte
	var err error
	switch {
	case flagValue == "-":
		data, err = io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("read JSON from stdin: %w", err)
		}
	case strings.HasPrefix(flagValue, "@"):
		path := strings.TrimPrefix(flagValue, "@")
		if path == "" {
			return nil, fmt.Errorf("JSON file path after @ is empty")
		}
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read JSON file %s: %w", path, err)
		}
	default:
		data = []byte(flagValue)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode JSON payload: %w", err)
	}
	if payload == nil {
		return nil, fmt.Errorf("JSON payload must be an object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("JSON payload contains multiple values")
		}
		return nil, fmt.Errorf("decode JSON payload: %w", err)
	}
	return payload, nil
}
