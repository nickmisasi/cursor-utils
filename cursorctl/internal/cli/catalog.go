package cli

import "github.com/spf13/cobra"

func newCatalogCommand(app *App, use string, method string, summary string) *cobra.Command {
	var jsonValue string
	command := &cobra.Command{
		Use:   use,
		Short: summary + " with " + method,
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			return runServiceUnaryCommand(
				app,
				command,
				args,
				jsonValue,
				unaryCommandSpec{
					service:       "SdkCursorService",
					method:        method,
					argCount:      0,
					use:           use,
					injectOptions: true,
					build: func(_ []string, apiKey string) (map[string]any, error) {
						return map[string]any{"options": map[string]any{"apiKey": apiKey}}, nil
					},
				},
				nil,
			)
		},
	}
	addJSONFlag(command, &jsonValue)
	return command
}
