package bridge

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	DefaultVersion = "v1.0.27"
	releaseBaseURL = "https://github.com/cursor/sdk-bridge/releases/download"
)

type InstallOptions struct {
	BinaryPath string
	Version    string
	HTTPClient *http.Client
	CacheDir   string
	GOOS       string
	GOARCH     string
	BaseURL    string
}

type InstallResult struct {
	Version string `json:"version"`
	Path    string `json:"path"`
	Cached  bool   `json:"cached"`
}

func EnsureInstalled(ctx context.Context, options InstallOptions) (InstallResult, error) {
	version := options.Version
	if version == "" {
		version = DefaultVersion
	}
	if filepath.Base(version) != version || version == "." || version == ".." {
		return InstallResult{}, fmt.Errorf("invalid bridge version %q", version)
	}
	if path := firstNonempty(options.BinaryPath, os.Getenv("CURSOR_SDK_BRIDGE_BIN")); path != "" {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return InstallResult{}, fmt.Errorf("resolve bridge binary: %w", err)
		}
		if err := validateBinary(absolute); err != nil {
			return InstallResult{}, err
		}
		return InstallResult{Version: version, Path: absolute}, nil
	}

	cacheDir, err := bridgeCacheDir(options.CacheDir)
	if err != nil {
		return InstallResult{}, err
	}
	targetDir := filepath.Join(cacheDir, version)
	binaryPath := filepath.Join(targetDir, "bin", "cursor-sdk-bridge")
	if validateBinary(binaryPath) == nil {
		return InstallResult{Version: version, Path: binaryPath, Cached: true}, nil
	}

	goos := firstNonempty(options.GOOS, runtime.GOOS)
	goarch := firstNonempty(options.GOARCH, runtime.GOARCH)
	platform, architecture, err := releasePlatform(goos, goarch)
	if err != nil {
		return InstallResult{}, err
	}
	archiveName := fmt.Sprintf("cursor-sdk-bridge-standalone-%s-%s.tar.gz", platform, architecture)
	baseURL := firstNonempty(options.BaseURL, releaseBaseURL)
	releaseURL := strings.TrimRight(baseURL, "/") + "/" + version
	client := options.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	checksums, err := download(ctx, client, releaseURL+"/SHA256SUMS.txt")
	if err != nil {
		return InstallResult{}, fmt.Errorf("download bridge checksums: %w", err)
	}
	expected, err := checksumFor(checksums, archiveName)
	if err != nil {
		return InstallResult{}, err
	}
	archive, err := download(ctx, client, releaseURL+"/"+archiveName)
	if err != nil {
		return InstallResult{}, fmt.Errorf("download bridge archive: %w", err)
	}
	actual := sha256.Sum256(archive)
	if !strings.EqualFold(expected, hex.EncodeToString(actual[:])) {
		return InstallResult{}, fmt.Errorf("bridge archive checksum mismatch: expected %s, got %x", expected, actual)
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("create bridge cache: %w", err)
	}
	tempDir, err := os.MkdirTemp(cacheDir, "."+strings.TrimPrefix(version, "v")+"-*")
	if err != nil {
		return InstallResult{}, fmt.Errorf("create bridge install directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	if err := extractArchive(archive, tempDir); err != nil {
		return InstallResult{}, err
	}
	tempBinary := filepath.Join(tempDir, "bin", "cursor-sdk-bridge")
	if err := os.Chmod(tempBinary, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("make bridge executable: %w", err)
	}
	if validateBinary(binaryPath) == nil {
		return InstallResult{Version: version, Path: binaryPath, Cached: true}, nil
	}
	if err := os.RemoveAll(targetDir); err != nil {
		return InstallResult{}, fmt.Errorf("remove incomplete bridge install: %w", err)
	}
	if err := os.Rename(tempDir, targetDir); err != nil {
		if validateBinary(binaryPath) != nil {
			return InstallResult{}, fmt.Errorf("publish bridge install: %w", err)
		}
	}
	return InstallResult{Version: version, Path: binaryPath}, nil
}

func bridgeCacheDir(override string) (string, error) {
	if override != "" {
		return filepath.Join(override, "cursorctl", "sdk-bridge"), nil
	}
	root, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache directory: %w", err)
	}
	return filepath.Join(root, "cursorctl", "sdk-bridge"), nil
}

func releasePlatform(goos, goarch string) (string, string, error) {
	var platform string
	switch goos {
	case "linux":
		platform = "linux"
	case "darwin":
		platform = "darwin"
	case "windows":
		platform = "win32"
	default:
		return "", "", fmt.Errorf("unsupported bridge operating system %q", goos)
	}

	var architecture string
	switch goarch {
	case "amd64":
		architecture = "x64"
	case "arm64":
		architecture = "arm64"
	default:
		return "", "", fmt.Errorf("unsupported bridge architecture %q", goarch)
	}
	if platform == "win32" && architecture != "x64" {
		return "", "", fmt.Errorf("unsupported bridge platform %s-%s", platform, architecture)
	}
	return platform, architecture, nil
}

func download(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		io.Copy(io.Discard, response.Body)
		return nil, fmt.Errorf("GET %s returned %s", url, response.Status)
	}
	return io.ReadAll(response.Body)
}

func checksumFor(data []byte, filename string) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		candidate := strings.TrimPrefix(fields[len(fields)-1], "*")
		if candidate == filename {
			if len(fields[0]) != sha256.Size*2 {
				return "", fmt.Errorf("invalid SHA-256 checksum for %s", filename)
			}
			if _, err := hex.DecodeString(fields[0]); err != nil {
				return "", fmt.Errorf("invalid SHA-256 checksum for %s: %w", filename, err)
			}
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("SHA-256 checksum for %s not found", filename)
}

func extractArchive(data []byte, destination string) error {
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("open bridge archive: %w", err)
	}
	defer gzipReader.Close()

	required := map[string]bool{
		"bin/cursor-sdk-bridge": false,
		"manifest.json":         false,
	}
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read bridge archive: %w", err)
		}
		name := filepath.ToSlash(filepath.Clean(header.Name))
		if _, wanted := required[name]; !wanted {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return fmt.Errorf("bridge archive entry %s is not a regular file", name)
		}
		target := filepath.Join(destination, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create bridge archive directory: %w", err)
		}
		file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("create bridge archive entry: %w", err)
		}
		_, copyErr := io.Copy(file, tarReader)
		closeErr := file.Close()
		if copyErr != nil {
			return fmt.Errorf("extract bridge archive entry: %w", copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close bridge archive entry: %w", closeErr)
		}
		required[name] = true
	}
	for name, found := range required {
		if !found {
			return fmt.Errorf("bridge archive is missing %s", name)
		}
	}
	return nil
}

func validateBinary(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("bridge binary %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("bridge binary %s is not a regular file", path)
	}
	return nil
}

func firstNonempty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
