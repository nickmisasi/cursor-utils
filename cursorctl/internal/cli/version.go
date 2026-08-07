package cli

import (
	"runtime"
	"runtime/debug"

	"github.com/nickmisasi/cursor-utils/cursorctl/internal/bridge"
	"github.com/spf13/cobra"
)

// Override with: -ldflags "-X github.com/nickmisasi/cursor-utils/cursorctl/internal/cli.Version=<version>".
var Version = "dev"

func newVersionCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show cursorctl, pinned bridge, and Go versions",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.Print(map[string]any{
				"version":       resolvedVersion(),
				"bridgeVersion": bridge.DefaultVersion,
				"goVersion":     runtime.Version(),
			})
		},
	}
}

func resolvedVersion() string {
	if Version != "dev" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return Version
}
