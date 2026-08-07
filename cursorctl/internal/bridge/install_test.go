package bridge

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureInstalledDownloadsVerifiesAndCaches(t *testing.T) {
	t.Setenv("CURSOR_SDK_BRIDGE_BIN", "")
	archive := testArchive(t)
	sum := sha256.Sum256(archive)
	const archiveName = "cursor-sdk-bridge-standalone-linux-x64.tar.gz"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		switch request.URL.Path {
		case "/v-test/SHA256SUMS.txt":
			fmt.Fprintf(writer, "%x  %s\n", sum, archiveName)
		case "/v-test/" + archiveName:
			writer.Write(archive)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	options := InstallOptions{
		Version:    "v-test",
		HTTPClient: server.Client(),
		CacheDir:   t.TempDir(),
		GOOS:       "linux",
		GOARCH:     "amd64",
		BaseURL:    server.URL,
	}
	first, err := EnsureInstalled(context.Background(), options)
	if err != nil {
		t.Fatalf("EnsureInstalled() error = %v", err)
	}
	if first.Cached {
		t.Fatal("first install reported cached")
	}
	if data, err := os.ReadFile(first.Path); err != nil || string(data) != "bridge" {
		t.Fatalf("installed binary = %q, error = %v", data, err)
	}
	info, err := os.Stat(first.Path)
	if err != nil {
		t.Fatalf("stat installed binary: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("installed binary mode = %v", info.Mode())
	}
	manifest := filepath.Join(filepath.Dir(filepath.Dir(first.Path)), "manifest.json")
	if data, err := os.ReadFile(manifest); err != nil || string(data) != `{"version":"test"}` {
		t.Fatalf("installed manifest = %q, error = %v", data, err)
	}

	second, err := EnsureInstalled(context.Background(), options)
	if err != nil {
		t.Fatalf("cached EnsureInstalled() error = %v", err)
	}
	if !second.Cached || second.Path != first.Path {
		t.Fatalf("cached result = %#v, first = %#v", second, first)
	}
	if requests != 2 {
		t.Fatalf("HTTP requests = %d, want 2", requests)
	}
}

func TestEnsureInstalledRejectsChecksumMismatch(t *testing.T) {
	t.Setenv("CURSOR_SDK_BRIDGE_BIN", "")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if filepath.Base(request.URL.Path) == "SHA256SUMS.txt" {
			io.WriteString(writer, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  cursor-sdk-bridge-standalone-linux-x64.tar.gz\n")
			return
		}
		io.WriteString(writer, "not an archive")
	}))
	defer server.Close()

	_, err := EnsureInstalled(context.Background(), InstallOptions{
		Version:    "v-test",
		HTTPClient: server.Client(),
		CacheDir:   t.TempDir(),
		GOOS:       "linux",
		GOARCH:     "amd64",
		BaseURL:    server.URL,
	})
	if err == nil {
		t.Fatal("EnsureInstalled() error = nil")
	}
}

func TestEnsureInstalledUsesExplicitBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bridge")
	if err := os.WriteFile(path, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := EnsureInstalled(context.Background(), InstallOptions{
		BinaryPath: path,
		Version:    "v-test",
	})
	if err != nil {
		t.Fatalf("EnsureInstalled() error = %v", err)
	}
	if result.Path != path || result.Cached {
		t.Fatalf("result = %#v", result)
	}
}

func TestReleasePlatform(t *testing.T) {
	tests := []struct {
		goos, goarch      string
		platform, arch    string
		shouldReturnError bool
	}{
		{goos: "linux", goarch: "amd64", platform: "linux", arch: "x64"},
		{goos: "darwin", goarch: "arm64", platform: "darwin", arch: "arm64"},
		{goos: "windows", goarch: "amd64", platform: "win32", arch: "x64"},
		{goos: "windows", goarch: "arm64", shouldReturnError: true},
		{goos: "plan9", goarch: "amd64", shouldReturnError: true},
		{goos: "linux", goarch: "386", shouldReturnError: true},
	}
	for _, test := range tests {
		t.Run(test.goos+"-"+test.goarch, func(t *testing.T) {
			platform, arch, err := releasePlatform(test.goos, test.goarch)
			if (err != nil) != test.shouldReturnError {
				t.Fatalf("releasePlatform() error = %v", err)
			}
			if platform != test.platform || arch != test.arch {
				t.Fatalf("releasePlatform() = %s-%s, want %s-%s", platform, arch, test.platform, test.arch)
			}
		})
	}
}

func testArchive(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	files := []struct {
		name string
		data string
	}{
		{name: "bin/cursor-sdk-bridge", data: "bridge"},
		{name: "manifest.json", data: `{"version":"test"}`},
		{name: "proto/sdk/v1/service.proto", data: "ignored"},
	}
	for _, file := range files {
		header := &tar.Header{
			Name: file.name,
			Mode: 0o644,
			Size: int64(len(file.data)),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tarWriter, file.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
