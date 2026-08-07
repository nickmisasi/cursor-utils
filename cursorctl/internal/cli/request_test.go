package cli

import "testing"

func TestPhase2LeafCommandsHaveJSONFlag(t *testing.T) {
	root, _ := NewRootCommand()
	for _, namespace := range []string{"agent", "run"} {
		command, _, err := root.Find([]string{namespace})
		if err != nil {
			t.Fatal(err)
		}
		for _, child := range command.Commands() {
			if child.Flags().Lookup("json") == nil {
				t.Errorf("%s %s has no --json flag", namespace, child.Name())
			}
		}
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
