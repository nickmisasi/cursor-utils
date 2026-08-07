package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Format string

const (
	FormatJSON Format = "json"
	FormatYAML Format = "yaml"
	FormatTOON Format = "toon"
)

func ParseFormat(value string) (Format, error) {
	switch Format(strings.ToLower(value)) {
	case FormatJSON:
		return FormatJSON, nil
	case FormatYAML:
		return FormatYAML, nil
	case FormatTOON:
		return FormatTOON, nil
	default:
		return "", fmt.Errorf("unsupported output format %q (expected json, yaml, or toon)", value)
	}
}

func Print(w io.Writer, format Format, value any) error {
	normalized, err := normalize(value)
	if err != nil {
		return err
	}

	switch format {
	case FormatJSON:
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(normalized)
	case FormatYAML:
		encoder := yaml.NewEncoder(w)
		encoder.SetIndent(2)
		defer encoder.Close()
		return encoder.Encode(yamlValue(normalized))
	case FormatTOON:
		data, err := marshalTOONNormalized(normalized)
		if err != nil {
			return err
		}
		data = append(data, '\n')
		_, err = w.Write(data)
		return err
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func yamlValue(value any) any {
	switch value := value.(type) {
	case json.Number:
		if integer, err := strconv.ParseInt(value.String(), 10, 64); err == nil {
			return integer
		}
		if integer, err := strconv.ParseUint(value.String(), 10, 64); err == nil {
			return integer
		}
		if decimal, err := strconv.ParseFloat(value.String(), 64); err == nil {
			return decimal
		}
		return value.String()
	case []any:
		result := make([]any, len(value))
		for i, item := range value {
			result[i] = yamlValue(item)
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, item := range value {
			result[key] = yamlValue(item)
		}
		return result
	case nil, bool, string:
		return value
	default:
		panic(fmt.Sprintf("unexpected normalized value %T", value))
	}
}

func normalize(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("normalize output: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var normalized any
	if err := decoder.Decode(&normalized); err != nil {
		return nil, fmt.Errorf("normalize output: %w", err)
	}
	return normalized, nil
}
