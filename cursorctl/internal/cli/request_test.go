package cli

import (
	"bytes"
	"testing"
)

func TestRPCLeafCommandsHaveJSONFlag(t *testing.T) {
	root, _ := NewRootCommand()
	paths := [][]string{
		{"agent", "create"}, {"agent", "resume"}, {"agent", "send"}, {"agent", "prompt"},
		{"agent", "get"}, {"agent", "list"}, {"agent", "messages"}, {"agent", "usage"},
		{"agent", "archive"}, {"agent", "unarchive"}, {"agent", "delete"},
		{"agent", "close"}, {"agent", "reload"},
		{"run", "get"}, {"run", "list"}, {"run", "wait"}, {"run", "watch"},
		{"run", "cancel"}, {"run", "conversation"},
		{"artifact", "list"}, {"artifact", "download"},
		{"me"}, {"models"}, {"repos"},
		{"bridge", "ping"}, {"bridge", "version"},
	}
	for _, path := range paths {
		command, _, err := root.Find(path)
		if err != nil {
			t.Fatal(err)
		}
		if command.Flags().Lookup("json") == nil {
			t.Errorf("%v has no --json flag", path)
		}
	}
}

func TestResumeDoesNotRegisterIdempotencyKey(t *testing.T) {
	root, _ := NewRootCommand()
	command, _, err := root.Find([]string{"agent", "resume"})
	if err != nil {
		t.Fatal(err)
	}
	if command.Flags().Lookup("idempotency-key") != nil {
		t.Fatal("agent resume unexpectedly registers --idempotency-key")
	}
}

func TestDefaultCompletionCommandIsHidden(t *testing.T) {
	root, _ := NewRootCommand()
	if !root.CompletionOptions.HiddenDefaultCmd {
		t.Fatal("default completion command is visible")
	}
}

func TestNonRPCCommandsDoNotHaveJSONFlag(t *testing.T) {
	root, _ := NewRootCommand()
	for _, path := range [][]string{{"bridge", "install"}, {"version"}} {
		command, _, err := root.Find(path)
		if err != nil {
			t.Fatal(err)
		}
		if command.Flags().Lookup("json") != nil {
			t.Errorf("%v unexpectedly has --json", path)
		}
	}
}

func TestEveryAppRunEPathPreparesWithoutRootHook(t *testing.T) {
	for _, path := range [][]string{{"version"}, {"bridge", "install"}} {
		root, _ := NewRootCommand()
		if root.PersistentPreRunE != nil {
			t.Fatal("root unexpectedly has PersistentPreRunE")
		}
		root.SetArgs(append(path, "--output", "invalid"))
		if err := root.Execute(); err == nil {
			t.Errorf("%v did not validate output format", path)
		}
	}
}

func TestCompletionDoesNotPrepareApp(t *testing.T) {
	root, _ := NewRootCommand()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"completion", "bash", "--output", "invalid"})
	if err := root.Execute(); err != nil {
		t.Fatalf("completion error = %v", err)
	}
	if output.Len() == 0 {
		t.Fatal("completion output is empty")
	}
}

func TestParseKeyValuesAndRepo(t *testing.T) {
	values, err := parseKeyValues([]string{"A=one", "B=two=three", "EMPTY="})
	if err != nil {
		t.Fatalf("parseKeyValues() error = %v", err)
	}
	if values["A"] != "one" || values["B"] != "two=three" || values["EMPTY"] != "" {
		t.Fatalf("values = %#v", values)
	}
	if _, err := parseKeyValues([]string{"missing"}); err == nil {
		t.Fatal("parseKeyValues() error = nil")
	}

	repo, err := parseRepo("git@github.com:acme/repo.git@feature")
	if err != nil {
		t.Fatalf("parseRepo() error = %v", err)
	}
	if repo["url"] != "git@github.com:acme/repo.git" || repo["startingRef"] != "feature" {
		t.Fatalf("repo = %#v", repo)
	}
	repo, err = parseRepo("https://user@github.com/acme/repo")
	if err != nil {
		t.Fatalf("parseRepo() error = %v", err)
	}
	if repo["url"] != "https://user@github.com/acme/repo" {
		t.Fatalf("repo = %#v", repo)
	}
}
