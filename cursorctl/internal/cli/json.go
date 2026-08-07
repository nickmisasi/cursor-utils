package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

var errMultipleJSONValues = errors.New("multiple JSON values")

func (a *App) ReadJSONPayload(flagValue string) (map[string]any, error) {
	var data []byte
	var err error
	switch {
	case flagValue == "-":
		data, err = io.ReadAll(a.In)
		if err != nil {
			return nil, fmt.Errorf("read JSON from stdin: %w", err)
		}
	case strings.HasPrefix(flagValue, "@"):
		path := strings.TrimPrefix(flagValue, "@")
		if path == "" {
			return nil, fmt.Errorf("empty JSON file path after @")
		}
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read JSON file %s: %w", path, err)
		}
	default:
		data = []byte(flagValue)
	}

	var payload map[string]any
	if err := decodeSingleJSON(data, &payload); err != nil {
		if errors.Is(err, errMultipleJSONValues) {
			return nil, fmt.Errorf("payload contains multiple JSON values")
		}
		return nil, fmt.Errorf("decode JSON payload: %w", err)
	}
	if payload == nil {
		return nil, fmt.Errorf("payload must be a JSON object")
	}
	return payload, nil
}

func decodeSingleJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errMultipleJSONValues
		}
		return err
	}
	return nil
}
