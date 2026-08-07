package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func addJSONFlag(command *cobra.Command, target *string) {
	command.Flags().StringVar(target, "json", "", "Raw RPC request JSON, @file, or - for stdin")
}

func jsonRequest(
	app *App,
	command *cobra.Command,
	args []string,
	value string,
	allowedFlags ...string,
) (map[string]any, bool, error) {
	if !command.Flags().Changed("json") {
		return nil, false, nil
	}
	if len(args) != 0 {
		return nil, true, fmt.Errorf("cannot combine --json with positional arguments")
	}
	allowed := map[string]bool{"json": true}
	for _, name := range allowedFlags {
		allowed[name] = true
	}
	var conflict string
	command.LocalNonPersistentFlags().VisitAll(func(flag *pflag.Flag) {
		if conflict == "" && flag.Changed && !allowed[flag.Name] {
			conflict = flag.Name
		}
	})
	if conflict != "" {
		return nil, true, fmt.Errorf("cannot combine --json with --%s", conflict)
	}
	request, err := app.ReadJSONPayload(value)
	return request, true, err
}

type unaryRequestBuilder func(args []string, apiKey string) (map[string]any, error)

func runUnaryCommand(
	app *App,
	command *cobra.Command,
	args []string,
	jsonValue string,
	method string,
	argCount int,
	usage string,
	injectOptions bool,
	allowedJSONFlags []string,
	build unaryRequestBuilder,
) error {
	request, err := buildUnaryRequest(
		app,
		command,
		args,
		jsonValue,
		argCount,
		usage,
		injectOptions,
		allowedJSONFlags,
		build,
	)
	if err != nil {
		return err
	}
	return runRPC[map[string]any](app, command, "SdkAgentService", method, request)
}

func buildUnaryRequest(
	app *App,
	command *cobra.Command,
	args []string,
	jsonValue string,
	argCount int,
	usage string,
	injectOptions bool,
	allowedJSONFlags []string,
	build unaryRequestBuilder,
) (map[string]any, error) {
	request, raw, err := jsonRequest(app, command, args, jsonValue, allowedJSONFlags...)
	if err != nil {
		return nil, err
	}
	var apiKey string
	if injectOptions {
		apiKey, err = app.ResolvedAPIKey()
		if err != nil {
			return nil, err
		}
	}
	if raw {
		if injectOptions {
			if err := injectAPIKey(request, "options", apiKey); err != nil {
				return nil, err
			}
		}
	} else {
		if err := requireArgs(args, argCount, usage); err != nil {
			return nil, err
		}
		request, err = build(args, apiKey)
		if err != nil {
			return nil, err
		}
	}
	return request, nil
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

func setRuntimeOption(command *cobra.Command, options map[string]any, value string) error {
	if !command.Flags().Changed("runtime") {
		return nil
	}
	runtime, err := prefixedEnum(value, "RUNTIME_", "LOCAL", "CLOUD")
	if err != nil {
		return err
	}
	options["runtime"] = runtime
	return nil
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
