package output

import (
	"encoding/json"
	"math"
	"testing"
)

func TestMarshalTOON(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{
			name: "documentation tabular example",
			value: map[string]any{"users": []any{
				map[string]any{"id": 1, "name": "Alice", "role": "admin"},
				map[string]any{"id": 2, "name": "Bob", "role": "user"},
			}},
			want: "users[2]{id,name,role}:\n  1,Alice,admin\n  2,Bob,user",
		},
		{
			name: "documentation nested example",
			value: map[string]any{
				"user":    map[string]any{"id": 123, "name": "Ada"},
				"tags":    []any{"a", "b"},
				"items":   []any{},
				"note":    "Smith, Bob",
				"version": "123",
			},
			want: "items: []\nnote: \"Smith, Bob\"\ntags[2]: a,b\nuser:\n  id: 123\n  name: Ada\nversion: \"123\"",
		},
		{
			name:  "documentation mixed list example",
			value: map[string]any{"items": []any{1, map[string]any{"a": 1}, "text"}},
			want:  "items[3]:\n  - 1\n  - a: 1\n  - text",
		},
		{
			name:  "primitive arrays",
			value: map[string]any{"flags": []any{true, false, nil}},
			want:  "flags[3]: true,false,null",
		},
		{
			name:  "arrays of primitive arrays",
			value: map[string]any{"pairs": []any{[]any{1, 2}, []any{3, 4}}},
			want:  "pairs[2]:\n  - [2]: 1,2\n  - [2]: 3,4",
		},
		{
			name: "nonuniform object list",
			value: map[string]any{"items": []any{
				map[string]any{"id": 1, "name": "a"},
				map[string]any{"id": 2},
			}},
			want: "items[2]:\n  - id: 1\n    name: a\n  - id: 2",
		},
		{
			name:  "nested empty object",
			value: map[string]any{"empty": map[string]any{}, "nested": map[string]any{"ok": true}},
			want:  "empty:\nnested:\n  ok: true",
		},
		{
			name:  "empty root object",
			value: map[string]any{},
			want:  "",
		},
		{
			name:  "empty root array",
			value: []any{},
			want:  "[]",
		},
		{
			name:  "root primitive array",
			value: []any{"a", "b"},
			want:  "[2]: a,b",
		},
		{
			name: "quoted keys",
			value: map[string]any{
				"1bad":    1,
				"also-ok": 2,
				"good.ok": 3,
			},
			want: "\"1bad\": 1\n\"also-ok\": 2\ngood.ok: 3",
		},
		{
			name: "string quoting rules",
			value: map[string]any{
				"bare":     "hello world",
				"boolean":  "true",
				"bracket":  "a[b",
				"colon":    "a:b",
				"comma":    "a,b",
				"control":  "a\x01b",
				"empty":    "",
				"hash":     "#tag",
				"leading":  " x",
				"numeric":  "-12.3e2",
				"quote":    `a"b`,
				"slash":    `a\b`,
				"trailing": "x ",
			},
			want: "bare: hello world\nboolean: \"true\"\nbracket: \"a[b\"\ncolon: \"a:b\"\ncomma: \"a,b\"\ncontrol: \"a\\u0001b\"\nempty: \"\"\nhash: \"#tag\"\nleading: \" x\"\nnumeric: \"-12.3e2\"\nquote: \"a\\\"b\"\nslash: \"a\\\\b\"\ntrailing: \"x \"",
		},
		{
			name: "escapes",
			value: map[string]any{
				"value": "line\nreturn\rtab\t",
			},
			want: "value: \"line\\nreturn\\rtab\\t\"",
		},
		{
			name: "canonical numbers",
			value: map[string]any{
				"decimal":      json.Number("1.2300"),
				"large":        1e21,
				"negativeZero": math.Copysign(0, -1),
				"regular":      1.5e6,
				"small":        1e-7,
				"zero":         json.Number("-0"),
			},
			want: "decimal: 1.23\nlarge: 1e+21\nnegativeZero: 0\nregular: 1500000\nsmall: 1e-7\nzero: 0",
		},
		{
			name: "tabular rejected for nested values",
			value: map[string]any{"items": []any{
				map[string]any{"id": 1, "tags": []any{"a"}},
				map[string]any{"id": 2, "tags": []any{"b"}},
			}},
			want: "items[2]:\n  - id: 1\n    tags[1]: a\n  - id: 2\n    tags[1]: b",
		},
		{
			name: "nested first field in object list",
			value: map[string]any{"items": []any{
				map[string]any{"a": map[string]any{"x": 1}, "b": 2},
				map[string]any{"a": map[string]any{"x": 3}, "c": 4},
			}},
			want: "items[2]:\n  - a:\n      x: 1\n    b: 2\n  - a:\n      x: 3\n    c: 4",
		},
		{
			name:  "object list item starts on hyphen",
			value: []any{map[string]any{"a": 1, "b": 2}},
			want:  "[1]{a,b}:\n  1,2",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := MarshalTOON(test.value)
			if err != nil {
				t.Fatalf("MarshalTOON() error = %v", err)
			}
			if string(got) != test.want {
				t.Fatalf("MarshalTOON() =\n%s\nwant:\n%s", got, test.want)
			}
		})
	}
}

func TestMarshalTOONRejectsNonFiniteNumbers(t *testing.T) {
	_, err := MarshalTOON(map[string]any{"nan": math.NaN()})
	if err == nil {
		t.Fatal("MarshalTOON() error = nil")
	}
}

func TestTOONDeterministicMapOrder(t *testing.T) {
	value := map[string]any{"z": 1, "a": 2, "m": 3}
	const want = "a: 2\nm: 3\nz: 1"
	for range 20 {
		got, err := MarshalTOON(value)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("MarshalTOON() = %q, want %q", got, want)
		}
	}
}
