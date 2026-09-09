package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

const (
	ConfigDirEnv    = "CURSORCTL_CONFIG_DIR"
	ProfileEnv      = "CURSORCTL_PROFILE"
	credentialsName = "credentials.json"
	currentVersion  = 1
)

var (
	// ErrNoDefault is returned when no default profile is configured.
	ErrNoDefault = errors.New("no default auth profile")

	profileNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

type File struct {
	Version        int                `json:"version"`
	DefaultProfile string             `json:"defaultProfile,omitempty"`
	Profiles       map[string]Profile `json:"profiles"`
}

type Profile struct {
	APIKey string `json:"apiKey"`
}

type Summary struct {
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
	Default     bool   `json:"default"`
}

type ListResult struct {
	DefaultProfile string    `json:"defaultProfile"`
	Profiles       []Summary `json:"profiles"`
}

func ConfigDir() (string, error) {
	if dir := os.Getenv(ConfigDirEnv); dir != "" {
		absolute, err := filepath.Abs(dir)
		if err != nil {
			return "", fmt.Errorf("resolve auth config directory: %w", err)
		}
		return absolute, nil
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "cursorctl"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "cursorctl"), nil
}

func CredentialsPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, credentialsName), nil
}

func ValidateName(name string) error {
	if !profileNamePattern.MatchString(name) {
		return fmt.Errorf("invalid auth profile name %q", name)
	}
	return nil
}

func Fingerprint(apiKey string) string {
	const suffix = 4
	if apiKey == "" {
		return "…"
	}
	if len(apiKey) <= suffix {
		return "…" + apiKey
	}
	return "…" + apiKey[len(apiKey)-suffix:]
}

func Load() (*File, error) {
	path, err := CredentialsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return emptyFile(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read auth credentials: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat auth credentials: %w", err)
	}
	if err := checkCredentialsPerm(path, info.Mode()); err != nil {
		return nil, err
	}
	var file File
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("decode auth credentials: %w", err)
	}
	if file.Version == 0 {
		file.Version = currentVersion
	}
	if file.Version != currentVersion {
		return nil, fmt.Errorf("unsupported credentials file version %d", file.Version)
	}
	if file.Profiles == nil {
		file.Profiles = map[string]Profile{}
	}
	return &file, nil
}

func Save(file *File) error {
	if file == nil {
		file = emptyFile()
	}
	if err := validateFile(file); err != nil {
		return err
	}
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create auth config directory: %w", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("secure auth config directory: %w", err)
		}
	}
	file.Version = currentVersion
	if file.Profiles == nil {
		file.Profiles = map[string]Profile{}
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("encode auth credentials: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, "credentials.json.*.tmp")
	if err != nil {
		return fmt.Errorf("create auth credentials tempfile: %w", err)
	}
	tmpName := tmp.Name()
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpName)
		}
	}()
	if runtime.GOOS != "windows" {
		if err := tmp.Chmod(0o600); err != nil {
			_ = tmp.Close()
			return fmt.Errorf("secure auth credentials tempfile: %w", err)
		}
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write auth credentials: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close auth credentials tempfile: %w", err)
	}

	path, err := CredentialsPath()
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace auth credentials: %w", err)
	}
	success = true
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("secure auth credentials: %w", err)
		}
	}
	return nil
}

func Add(name, apiKey string, setDefault, force bool) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return fmt.Errorf("API key is empty")
	}
	file, err := Load()
	if err != nil {
		return err
	}
	if _, exists := file.Profiles[name]; exists && !force {
		return fmt.Errorf("auth profile %q already exists (use --force to replace)", name)
	}
	file.Profiles[name] = Profile{APIKey: apiKey}
	if setDefault || file.DefaultProfile == "" {
		file.DefaultProfile = name
	}
	return Save(file)
}

func Remove(name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	file, err := Load()
	if err != nil {
		return err
	}
	if _, ok := file.Profiles[name]; !ok {
		return unknownProfile(name)
	}
	delete(file.Profiles, name)
	if file.DefaultProfile == name {
		file.DefaultProfile = ""
	}
	return Save(file)
}

func SetDefault(name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	file, err := Load()
	if err != nil {
		return err
	}
	if _, ok := file.Profiles[name]; !ok {
		return unknownProfile(name)
	}
	file.DefaultProfile = name
	return Save(file)
}

func Lookup(name string) (Profile, error) {
	if err := ValidateName(name); err != nil {
		return Profile{}, err
	}
	file, err := Load()
	if err != nil {
		return Profile{}, err
	}
	return lookupIn(file, name)
}

func LookupDefault() (string, Profile, error) {
	file, err := Load()
	if err != nil {
		return "", Profile{}, err
	}
	if file.DefaultProfile == "" {
		return "", Profile{}, ErrNoDefault
	}
	profile, err := lookupIn(file, file.DefaultProfile)
	if err != nil {
		return "", Profile{}, err
	}
	return file.DefaultProfile, profile, nil
}

func (f *File) List() ListResult {
	result := ListResult{
		Profiles: []Summary{},
	}
	if f == nil {
		return result
	}
	result.DefaultProfile = f.DefaultProfile
	if n := len(f.Profiles); n > 0 {
		result.Profiles = make([]Summary, 0, n)
	}
	names := make([]string, 0, len(f.Profiles))
	for name := range f.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result.Profiles = append(result.Profiles, Summary{
			Name:        name,
			Fingerprint: Fingerprint(f.Profiles[name].APIKey),
			Default:     name == f.DefaultProfile,
		})
	}
	return result
}

func emptyFile() *File {
	return &File{
		Version:  currentVersion,
		Profiles: map[string]Profile{},
	}
}

func lookupIn(file *File, name string) (Profile, error) {
	profile, ok := file.Profiles[name]
	if !ok {
		return Profile{}, unknownProfile(name)
	}
	if profile.APIKey == "" {
		return Profile{}, fmt.Errorf("auth profile %q has an empty API key", name)
	}
	return profile, nil
}

func unknownProfile(name string) error {
	return fmt.Errorf("unknown auth profile %q", name)
}

func validateFile(file *File) error {
	if file.Profiles == nil {
		file.Profiles = map[string]Profile{}
	}
	for name, profile := range file.Profiles {
		if err := ValidateName(name); err != nil {
			return err
		}
		if strings.TrimSpace(profile.APIKey) == "" {
			return fmt.Errorf("auth profile %q has an empty API key", name)
		}
	}
	if file.DefaultProfile != "" {
		if _, ok := file.Profiles[file.DefaultProfile]; !ok {
			return unknownProfile(file.DefaultProfile)
		}
	}
	return nil
}

func checkCredentialsPerm(path string, mode os.FileMode) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if mode.Perm()&0o077 != 0 {
		return fmt.Errorf("credentials file %s is readable by group or others (mode %04o); expected 0600", path, mode.Perm())
	}
	return nil
}
