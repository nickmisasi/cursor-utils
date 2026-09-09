package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nickmisasi/cursor-utils/cursorctl/internal/auth"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type authDefaultResult struct {
	DefaultProfile string `json:"defaultProfile"`
}

type authRemoveResult struct {
	Name    string `json:"name"`
	Removed bool   `json:"removed"`
}

func newAuthCommand(app *App) *cobra.Command {
	command := &cobra.Command{
		Use:   "auth",
		Short: "Manage stored Cursor API key profiles",
	}
	command.AddCommand(
		newAuthAddCommand(app),
		newAuthListCommand(app),
		newAuthDefaultCommand(app),
		newAuthRemoveCommand(app),
	)
	return command
}

func newAuthAddCommand(app *App) *cobra.Command {
	var fromStdin bool
	var setDefault bool
	var force bool
	command := &cobra.Command{
		Use:   "add <name>",
		Short: "Store a named Cursor API key profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := prepareCommand(app, command); err != nil {
				return err
			}
			name := args[0]
			if err := auth.ValidateName(name); err != nil {
				return err
			}
			apiKey, err := readAPIKey(app, fromStdin)
			if err != nil {
				return err
			}
			if err := auth.Add(name, apiKey, setDefault, force); err != nil {
				return err
			}
			file, err := auth.Load()
			if err != nil {
				return err
			}
			return app.Print(auth.Summary{
				Name:        name,
				Fingerprint: auth.Fingerprint(apiKey),
				Default:     file.DefaultProfile == name,
			})
		},
	}
	command.Flags().BoolVar(&fromStdin, "stdin", false, "Read the API key from stdin")
	command.Flags().BoolVar(&setDefault, "default", false, "Make this profile the default")
	command.Flags().BoolVar(&force, "force", false, "Replace an existing profile with the same name")
	return command
}

func newAuthListCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List stored auth profiles without printing API keys",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := prepareCommand(app, command); err != nil {
				return err
			}
			file, err := auth.Load()
			if err != nil {
				return err
			}
			return app.Print(file.List())
		},
	}
}

func newAuthDefaultCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "default [name]",
		Short: "Show or set the default auth profile",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := prepareCommand(app, command); err != nil {
				return err
			}
			if len(args) == 1 {
				if err := auth.SetDefault(args[0]); err != nil {
					return err
				}
			}
			file, err := auth.Load()
			if err != nil {
				return err
			}
			return app.Print(authDefaultResult{DefaultProfile: file.DefaultProfile})
		},
	}
}

func newAuthRemoveCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Delete a stored auth profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := prepareCommand(app, command); err != nil {
				return err
			}
			name := args[0]
			if err := auth.Remove(name); err != nil {
				return err
			}
			return app.Print(authRemoveResult{Name: name, Removed: true})
		},
	}
}

func readAPIKey(app *App, fromStdin bool) (string, error) {
	if fromStdin {
		data, err := io.ReadAll(app.In)
		if err != nil {
			return "", fmt.Errorf("read API key from stdin: %w", err)
		}
		apiKey := strings.TrimSpace(string(data))
		if apiKey == "" {
			return "", fmt.Errorf("API key is empty")
		}
		return apiKey, nil
	}
	file, ok := app.In.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return "", fmt.Errorf("read API key from --stdin or a terminal")
	}
	fmt.Fprint(app.Err, "API key: ")
	data, err := term.ReadPassword(int(file.Fd()))
	fmt.Fprintln(app.Err)
	if err != nil {
		return "", fmt.Errorf("read API key: %w", err)
	}
	apiKey := strings.TrimSpace(string(data))
	if apiKey == "" {
		return "", fmt.Errorf("API key is empty")
	}
	return apiKey, nil
}
