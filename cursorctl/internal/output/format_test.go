package output

import (
	"bytes"
	"strings"
	"testing"
)

type printFixture struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestParseFormat(t *testing.T) {
	tests := []struct {
		input string
		want  Format
		ok    bool
	}{
		{input: "json", want: FormatJSON, ok: true},
		{input: "YAML", want: FormatYAML, ok: true},
		{input: "ToOn", want: FormatTOON, ok: true},
		{input: "xml", ok: false},
	}
	for _, test := range tests {
		got, err := ParseFormat(test.input)
		if (err == nil) != test.ok {
			t.Fatalf("ParseFormat(%q) error = %v, ok = %v", test.input, err, test.ok)
		}
		if got != test.want {
			t.Fatalf("ParseFormat(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestPrint(t *testing.T) {
	tests := []struct {
		name   string
		format Format
		want   string
	}{
		{
			name:   "json",
			format: FormatJSON,
			want:   "{\n  \"count\": 2,\n  \"name\": \"widgets\"\n}\n",
		},
		{
			name:   "yaml",
			format: FormatYAML,
			want:   "count: 2\nname: widgets\n",
		},
		{
			name:   "toon",
			format: FormatTOON,
			want:   "count: 2\nname: widgets",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			err := Print(&output, test.format, printFixture{Name: "widgets", Count: 2})
			if err != nil {
				t.Fatalf("Print() error = %v", err)
			}
			if output.String() != test.want {
				t.Fatalf("Print() = %q, want %q", output.String(), test.want)
			}
		})
	}
}

func TestPrintRejectsUnexpectedFormat(t *testing.T) {
	err := Print(&bytes.Buffer{}, Format("xml"), map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "unsupported output format") {
		t.Fatalf("Print() error = %v", err)
	}
}
