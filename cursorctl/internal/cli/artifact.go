package cli

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/nickmisasi/cursor-utils/cursorctl/internal/bridge"
	"github.com/spf13/cobra"
)

func newArtifactCommand(app *App) *cobra.Command {
	command := &cobra.Command{
		Use:   "artifact",
		Short: "List and download agent artifacts",
	}
	command.AddCommand(
		newArtifactListCommand(app),
		newArtifactDownloadCommand(app),
	)
	return command
}

func newArtifactListCommand(app *App) *cobra.Command {
	var jsonValue string
	command := &cobra.Command{
		Use:   "list <agent-id>",
		Short: "List agent artifacts with ListArtifacts",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			return runUnaryCommand(
				app, command, args, jsonValue,
				unaryCommandSpec{
					method:   "ListArtifacts",
					argCount: 1,
					use:      "artifact list",
					build: func(args []string, _ string) (map[string]any, error) {
						return map[string]any{"agentId": args[0]}, nil
					},
				},
				nil,
			)
		},
	}
	addJSONFlag(command, &jsonValue)
	return command
}

func newArtifactDownloadCommand(app *App) *cobra.Command {
	var outputPath string
	var jsonValue string
	command := &cobra.Command{
		Use:   "download <agent-id> <path>",
		Short: "Stream artifact bytes with DownloadArtifact",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			request, err := buildUnaryRequest(
				app,
				command,
				args,
				jsonValue,
				2,
				"artifact download",
				false,
				[]string{"file"},
				func(args []string, _ string) (map[string]any, error) {
					return map[string]any{"agentId": args[0], "path": args[1]}, nil
				},
			)
			if err != nil {
				return err
			}
			return downloadArtifact(app, command, request, outputPath)
		},
	}
	command.Flags().StringVar(&outputPath, "file", "", "Write bytes to PATH; - or unset writes stdout")
	addJSONFlag(command, &jsonValue)
	return command
}

func downloadArtifact(
	app *App,
	command *cobra.Command,
	request map[string]any,
	outputPath string,
) (resultErr error) {
	if err := prepareCommand(app, command); err != nil {
		return err
	}
	client, err := app.Client(command.Context())
	if err != nil {
		return err
	}
	reader, err := client.Stream(
		command.Context(),
		"SdkAgentService",
		"DownloadArtifact",
		request,
	)
	if err != nil {
		return err
	}
	defer reader.Close()

	writer := app.Out
	var file *os.File
	fileClosed := false
	if outputPath != "" && outputPath != "-" {
		file, err = os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("create artifact file %s: %w", outputPath, err)
		}
		writer = file
		defer func() {
			if !fileClosed {
				if err := file.Close(); resultErr == nil && err != nil {
					resultErr = fmt.Errorf("close artifact file %s: %w", outputPath, err)
				}
			}
			if resultErr != nil {
				if err := os.Remove(outputPath); err != nil && !errors.Is(err, os.ErrNotExist) {
					resultErr = errors.Join(resultErr, fmt.Errorf("remove partial artifact %s: %w", outputPath, err))
				}
			}
		}()
	}
	count, err := copyArtifactChunks(writer, reader)
	if err != nil {
		return err
	}
	if file == nil {
		return nil
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close artifact file %s: %w", outputPath, err)
	}
	fileClosed = true
	return app.Print(map[string]any{"path": outputPath, "bytes": count})
}

func copyArtifactChunks(writer io.Writer, reader *bridge.StreamReader) (int64, error) {
	var total int64
	for {
		var chunk struct {
			Data string `json:"data"`
		}
		err := reader.Next(&chunk)
		if errors.Is(err, io.EOF) {
			return total, nil
		}
		if err != nil {
			return total, err
		}
		// Proto3 JSON permits URL-safe/unpadded base64; the pinned bridge emits standard base64.
		data, err := base64.StdEncoding.DecodeString(chunk.Data)
		if err != nil {
			return total, fmt.Errorf("decode artifact chunk: %w", err)
		}
		written, err := writer.Write(data)
		total += int64(written)
		if err != nil {
			return total, fmt.Errorf("write artifact: %w", err)
		}
		if written != len(data) {
			return total, io.ErrShortWrite
		}
	}
}
