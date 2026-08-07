# TOON encoding reference (spec v4.1)

[TOON](https://github.com/toon-format/toon) (Token-Oriented Object Notation) is a line-oriented, indentation-based text format encoding the JSON data model, optimized for LLM token efficiency. Spec: https://github.com/toon-format/spec — v4.1 Working Draft (2026-07-26), MIT. Media type `text/toon`, UTF-8.

The official Go module (`github.com/toon-format/toon-go`) lags the spec (no semver tags, targets v3.x, still exposes removed `[#N]` length markers, sorts map keys instead of preserving order). We therefore maintain a small **encode-only** implementation targeting v4.1 defaults (`indentSize=2`, comma delimiter).

## Encoder forms

| Form | When | Example |
|------|------|---------|
| Inline | non-empty primitive arrays | `tags[3]: admin,ops,dev` |
| Tabular | uniform object arrays | `items[2]{sku,qty}:` + rows |
| List | everything else | `items[3]:` then `- …` |

(The spec also defines keyed-tabular `[N:]` for objects-of-uniform-objects; optional for encoders — plain nested form is also valid output.)

## Rules

### Indentation / whitespace
- Spaces only, 2 per level. LF only. No trailing spaces. No trailing newline on the document.
- Exactly one space after `:` when a value follows.
- Never emit `#` comment lines.

### Objects
- Primitive field: `key: value`
- Nested / empty object: `key:` alone; children at depth+1
- Empty root object → empty document
- Keys unquoted iff `^[A-Za-z_][A-Za-z0-9_.]*$`; else quoted with string escapes
- Preserve encounter order (for Go maps, sort keys deterministically and document it)

### Arrays
| Case | Encoding |
|------|----------|
| Empty field | `key: []` (not `key[0]:`) |
| Empty root array | `[]` |
| Primitive | `key[N]: v1,v2,…` |
| Uniform objects | `key[N]{f1,f2,…}:` then one comma-joined row per element at depth+1 |
| Non-uniform / mixed | `key[N]:` then `- item` list entries at depth+1 |
| Array of primitive arrays | `pairs[2]:` then `- [2]: 1,2` |
| Object list item | first field on the hyphen line: `- id: 1`, remaining fields at same depth+1 |

Tabular detection: every element is a non-empty object, same key set, every value primitive. Otherwise list form.

### String quoting — quote if any of:
empty; leading/trailing space; equals `true`/`false`/`null`; numeric-looking; contains `:`, `"`, `\`, `[`, `]`, `{`, `}`, control chars, or the active delimiter (comma); starts with `-` or `#`.

Escapes: `\\`, `\"`, `\n`, `\r`, `\t`, `\uXXXX` for other controls.

### Scalars
Canonical decimal numbers; `-0` → `0`; `true`/`false`/`null` lowercase; NaN/±Inf → `null`.

## Examples

```json
{"users":[{"id":1,"name":"Alice","role":"admin"},{"id":2,"name":"Bob","role":"user"}]}
```
```
users[2]{id,name,role}:
  1,Alice,admin
  2,Bob,user
```

```json
{"user":{"id":123,"name":"Ada"},"tags":["a","b"],"items":[],"note":"Smith, Bob","version":"123"}
```
```
user:
  id: 123
  name: Ada
tags[2]: a,b
items: []
note: "Smith, Bob"
version: "123"
```

```json
{"items":[1,{"a":1},"text"]}
```
```
items[3]:
  - 1
  - a: 1
  - text
```
