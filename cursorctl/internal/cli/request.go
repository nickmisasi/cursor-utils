package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func addJSONFlag(command *cobra.Command, target *string) {
	command.Flags().StringVar(target, "json", "", "Raw RPC request JSON, @file, or - for stdin")
}

func jsonRequest(
	app *App,
	command *cobra.Command,
	args []string,
	value string,
	requestFlags ...string,
) (map[string]any, bool, error) {
	if !command.Flags().Changed("json") {
		return nil, false, nil
	}
	if len(args) != 0 {
		return nil, true, fmt.Errorf("cannot combine --json with positional arguments")
	}
	for _, name := range requestFlags {
		if command.Flags().Changed(name) {
			return nil, true, fmt.Errorf("cannot combine --json with --%s", name)
		}
	}
	request, err := app.ReadJSONPayload(value)
	return request, true, err
}

func injectAPIKey(request map[string]any, field string, apiKey string) error {
	options, ok := request[field]
	if !ok || options == nil {
		request[field] = map[string]any{"apiKey": apiKey}
		return nil
	}
	optionMap, ok := options.(map[string]any)
	if !ok {
		return fmt.Errorf("%s must be a JSON object", field)
	}
	if _, exists := optionMap["apiKey"]; !exists {
		optionMap["apiKey"] = apiKey
	}
	return nil
}

func parseKeyValues(values []string) (map[string]string, error) {
	result := make(map[string]string, len(values))
	for _, value := range values {
		key, item, ok := strings.Cut(value, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid KEY=VAL value %q", value)
		}
		result[key] = item
	}
	return result, nil
}

func parseRepo(value string) (map[string]any, error) {
	if value == "" {
		return nil, fmt.Errorf("repository URL must not be empty")
	}
	url := value
	var ref string
	if at := strings.LastIndex(value, "@"); at >= 0 && at > strings.LastIndex(value, "/") {
		url, ref = value[:at], value[at+1:]
		if url == "" || ref == "" {
			return nil, fmt.Errorf("invalid repository %q (expected URL[@ref])", value)
		}
	}
	repo := map[string]any{"url": url}
	if ref != "" {
		repo["startingRef"] = ref
	}
	return repo, nil
}

func enumValue(value string, allowed ...string) (string, error) {
	normalized := strings.ToUpper(value)
	for _, candidate := range allowed {
		if normalized == candidate {
			return normalized, nil
		}
	}
	return "", fmt.Errorf("invalid value %q (expected %s)", value, strings.Join(allowed, " or "))
}

func prefixedEnum(value string, prefix string, allowed ...string) (string, error) {
	normalized, err := enumValue(value, allowed...)
	if err != nil {
		return "", err
	}
	return prefix + normalized, nil
}

func requireArgs(args []string, count int, usage string) error {
	if len(args) != count {
		return fmt.Errorf("%s requires %s", usage, argumentCount(count))
	}
	return nil
}

func argumentCount(count int) string {
	switch count {
	case 0:
		return "no positional arguments"
	case 1:
		return "exactly one positional argument"
	default:
		return fmt.Sprintf("exactly %d positional arguments", count)
	}
}
