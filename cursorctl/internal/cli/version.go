package cli

import (
	"runtime"

	"github.com/nickmisasi/cursor-utils/cursorctl/internal/bridge"
	"github.com/spf13/cobra"
)

var Version = "dev"

func newVersionCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show cursorctl, pinned bridge, and Go versions",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.Print(map[string]any{
				"version":       Version,
				"bridgeVersion": bridge.DefaultVersion,
				"goVersion":     runtime.Version(),
			})
		},
	}
}
