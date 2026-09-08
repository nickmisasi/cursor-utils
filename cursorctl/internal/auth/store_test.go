package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	valid := []string{"a", "work", "work.prod", "user_1", "A-B.C_9"}
	for _, name := range valid {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) = %v", name, err)
		}
	}
	invalid := []string{"", "-work", ".hidden", "work/prod", "../etc", "has space", "bad:name"}
	for _, name := range invalid {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) = nil", name)
		}
	}
}

func TestFingerprint(t *testing.T) {
	if got := Fingerprint(""); got != "…" {
		t.Fatalf("empty fingerprint = %q", got)
	}
	if got := Fingerprint("ab"); got != "…ab" {
		t.Fatalf("short fingerprint = %q", got)
	}
	if got := Fingerprint("cursor_secret_wxyz"); got != "…wxyz" {
		t.Fatalf("fingerprint = %q", got)
	}
}

func TestAddRoundTripAndDefault(t *testing.T) {
	isolateConfig(t)

	if err := Add("work", "cursor_work_key_aaaa", false, false); err != nil {
		t.Fatalf("Add() work: %v", err)
	}
	file, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if file.DefaultProfile != "work" {
		t.Fatalf("first profile default = %q", file.DefaultProfile)
	}

	if err := Add("personal", "cursor_personal_bbbb", false, false); err != nil {
		t.Fatalf("Add() personal: %v", err)
	}
	file, err = Load()
	if err != nil {
		t.Fatalf("Load() after second = %v", err)
	}
	if file.DefaultProfile != "work" {
		t.Fatalf("default changed without --default: %q", file.DefaultProfile)
	}

	if err := Add("ci", "cursor_ci_key_cccc", true, false); err != nil {
		t.Fatalf("Add() ci: %v", err)
	}
	name, profile, err := LookupDefault()
	if err != nil || name != "ci" || profile.APIKey != "cursor_ci_key_cccc" {
		t.Fatalf("LookupDefault() = %q %#v %v", name, profile, err)
	}

	work, err := Lookup("work")
	if err != nil || work.APIKey != "cursor_work_key_aaaa" {
		t.Fatalf("Lookup(work) = %#v %v", work, err)
	}
	if err := Add("ci", "cursor_ci_key_cccc", true, false); err == nil {
		t.Fatal("duplicate Add() without force succeeded")
	}
}

func TestAddForceOverwrite(t *testing.T) {
	isolateConfig(t)
	if err := Add("work", "cursor_old_key_zzzz", false, false); err != nil {
		t.Fatal(err)
	}
	if err := Add("work", "cursor_new_key_yyyy", false, true); err != nil {
		t.Fatalf("Add() force: %v", err)
	}
	profile, err := Lookup("work")
	if err != nil || profile.APIKey != "cursor_new_key_yyyy" {
		t.Fatalf("Lookup() after force = %#v %v", profile, err)
	}
	name, _, err := LookupDefault()
	if err != nil || name != "work" {
		t.Fatalf("default after force replace = %q %v", name, err)
	}
}

func TestRemoveClearsDefaultWithoutPickingAnother(t *testing.T) {
	isolateConfig(t)
	if err := Add("work", "cursor_work_key_aaaa", false, false); err != nil {
		t.Fatal(err)
	}
	if err := Add("personal", "cursor_personal_bbbb", false, false); err != nil {
		t.Fatal(err)
	}
	if err := Remove("work"); err != nil {
		t.Fatalf("Remove() = %v", err)
	}
	file, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if file.DefaultProfile != "" {
		t.Fatalf("default after removing default profile = %q", file.DefaultProfile)
	}
	if _, ok := file.Profiles["personal"]; !ok {
		t.Fatal("remaining profile was deleted")
	}
	if _, _, err := LookupDefault(); !errors.Is(err, ErrNoDefault) {
		t.Fatalf("LookupDefault() after remove = %v", err)
	}
}

func TestSetDefaultAndUnknownProfile(t *testing.T) {
	isolateConfig(t)
	if err := Add("work", "cursor_work_key_aaaa", false, false); err != nil {
		t.Fatal(err)
	}
	if err := Add("personal", "cursor_personal_bbbb", false, false); err != nil {
		t.Fatal(err)
	}
	if err := SetDefault("personal"); err != nil {
		t.Fatal(err)
	}
	name, _, err := LookupDefault()
	if err != nil || name != "personal" {
		t.Fatalf("LookupDefault() = %q %v", name, err)
	}
	if err := SetDefault("missing"); err == nil || !strings.Contains(err.Error(), `unknown auth profile "missing"`) {
		t.Fatalf("SetDefault(missing) = %v", err)
	}
	if _, err := Lookup("missing"); err == nil || !strings.Contains(err.Error(), `unknown auth profile "missing"`) {
		t.Fatalf("Lookup(missing) = %v", err)
	}
	if err := Remove("missing"); err == nil || !strings.Contains(err.Error(), `unknown auth profile "missing"`) {
		t.Fatalf("Remove(missing) = %v", err)
	}
}

func TestListOmitsAPIKeysAndSortsNames(t *testing.T) {
	isolateConfig(t)
	if err := Add("zeta", "cursor_zeta_key_zzzz", false, false); err != nil {
		t.Fatal(err)
	}
	if err := Add("alpha", "cursor_alpha_key_aaaa", true, false); err != nil {
		t.Fatal(err)
	}
	file, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	result := file.List()
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(data)
	if strings.Contains(encoded, "cursor_zeta_key_zzzz") || strings.Contains(encoded, "cursor_alpha_key_aaaa") {
		t.Fatalf("list leaked API key: %s", encoded)
	}
	if result.DefaultProfile != "alpha" || len(result.Profiles) != 2 {
		t.Fatalf("list = %#v", result)
	}
	if result.Profiles[0].Name != "alpha" || result.Profiles[1].Name != "zeta" {
		t.Fatalf("profile order = %#v", result.Profiles)
	}
	if result.Profiles[0].Fingerprint != "…aaaa" || !result.Profiles[0].Default {
		t.Fatalf("alpha summary = %#v", result.Profiles[0])
	}
}

func TestSaveIsAtomicAndMode0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not enforced on Windows")
	}
	dir := isolateConfig(t)
	if err := Add("work", "cursor_first_key_1111", false, false); err != nil {
		t.Fatal(err)
	}
	if err := Add("work", "cursor_second_key_2222", false, true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, credentialsName)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("credentials mode = %04o", info.Mode().Perm())
	}
	profile, err := Lookup("work")
	if err != nil || profile.APIKey != "cursor_second_key_2222" {
		t.Fatalf("Lookup() after replace = %#v %v", profile, err)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("config dir mode = %04o", dirInfo.Mode().Perm())
	}
}

func TestLoadRejectsGroupReadableCredentials(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not enforced on Windows")
	}
	dir := isolateConfig(t)
	if err := Add("work", "cursor_work_key_aaaa", false, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, credentialsName)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "readable by group or others") {
		t.Fatalf("Load() insecure file = %v", err)
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	isolateConfig(t)
	file, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if file.DefaultProfile != "" || len(file.Profiles) != 0 {
		t.Fatalf("empty load = %#v", file)
	}
	if _, _, err := LookupDefault(); !errors.Is(err, ErrNoDefault) {
		t.Fatalf("LookupDefault() empty = %v", err)
	}
}

func TestAddRejectsEmptyKey(t *testing.T) {
	isolateConfig(t)
	if err := Add("work", "   \n", false, false); err == nil {
		t.Fatal("Add() empty key succeeded")
	}
}

func TestConfigDirPrefersCursorctlOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(ConfigDirEnv, dir)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	got, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	absolute, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != absolute {
		t.Fatalf("ConfigDir() = %q, want %q", got, absolute)
	}
}

func isolateConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(ConfigDirEnv, dir)
	t.Setenv(ProfileEnv, "")
	return dir
}
