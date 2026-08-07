package output

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const toonIndent = 2

var (
	toonKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)
	numberPattern  = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)
)

// MarshalTOON accepts JSON-marshalable input; normalization rejects non-finite floats.
func MarshalTOON(value any) ([]byte, error) {
	normalized, err := normalize(value)
	if err != nil {
		return nil, err
	}
	return marshalTOONNormalized(normalized)
}

func marshalTOONNormalized(value any) ([]byte, error) {
	encoder := toonEncoder{}
	if err := encoder.root(value); err != nil {
		return nil, err
	}
	return []byte(strings.Join(encoder.lines, "\n")), nil
}

type toonEncoder struct {
	lines []string
}

func (e *toonEncoder) root(value any) error {
	switch value := value.(type) {
	case map[string]any:
		return e.object(value, 0)
	case []any:
		return e.array("", value, 0)
	case nil, bool, string, json.Number:
		e.line(0, toonScalar(value))
		return nil
	default:
		return fmt.Errorf("unsupported TOON value %T", value)
	}
}

func (e *toonEncoder) object(value map[string]any, depth int) error {
	for _, key := range sortedKeys(value) {
		if err := e.field(key, value[key], depth); err != nil {
			return err
		}
	}
	return nil
}

func (e *toonEncoder) field(key string, value any, depth int) error {
	name := toonKey(key)
	switch value := value.(type) {
	case map[string]any:
		e.line(depth, name+":")
		return e.object(value, depth+1)
	case []any:
		return e.array(name, value, depth)
	case nil, bool, string, json.Number:
		e.line(depth, name+": "+toonScalar(value))
		return nil
	default:
		return fmt.Errorf("unsupported TOON field %q value %T", key, value)
	}
}

func (e *toonEncoder) array(name string, values []any, depth int) error {
	prefix := name
	if len(values) == 0 {
		if name == "" {
			e.line(depth, "[]")
		} else {
			e.line(depth, name+": []")
		}
		return nil
	}

	if allPrimitive(values) {
		e.line(depth, fmt.Sprintf("%s[%d]: %s", prefix, len(values), joinScalars(values)))
		return nil
	}
	if fields, ok := tabularFields(values); ok {
		e.line(depth, fmt.Sprintf("%s[%d]{%s}:", prefix, len(values), strings.Join(quotedKeys(fields), ",")))
		for _, item := range values {
			object := item.(map[string]any)
			row := make([]any, len(fields))
			for i, field := range fields {
				row[i] = object[field]
			}
			e.line(depth+1, joinScalars(row))
		}
		return nil
	}

	e.line(depth, fmt.Sprintf("%s[%d]:", prefix, len(values)))
	for _, item := range values {
		if err := e.listItem(item, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (e *toonEncoder) listItem(value any, depth int) error {
	switch value := value.(type) {
	case nil, bool, string, json.Number:
		e.line(depth, "- "+toonScalar(value))
		return nil
	case []any:
		if len(value) == 0 {
			e.line(depth, "- []")
			return nil
		}
		if allPrimitive(value) {
			e.line(depth, fmt.Sprintf("- [%d]: %s", len(value), joinScalars(value)))
			return nil
		}
		e.line(depth, fmt.Sprintf("- [%d]:", len(value)))
		for _, item := range value {
			if err := e.listItem(item, depth+1); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		keys := sortedKeys(value)
		if len(keys) == 0 {
			e.line(depth, "-")
			return nil
		}
		first := keys[0]
		if err := e.firstObjectField(first, value[first], depth); err != nil {
			return err
		}
		for _, key := range keys[1:] {
			if err := e.field(key, value[key], depth+1); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported TOON list value %T", value)
	}
}

func (e *toonEncoder) firstObjectField(key string, value any, depth int) error {
	name := toonKey(key)
	switch value := value.(type) {
	case nil, bool, string, json.Number:
		e.line(depth, "- "+name+": "+toonScalar(value))
		return nil
	case map[string]any:
		e.line(depth, "- "+name+":")
		return e.object(value, depth+2)
	case []any:
		if len(value) == 0 {
			e.line(depth, "- "+name+": []")
			return nil
		}
		if allPrimitive(value) {
			e.line(depth, fmt.Sprintf("- %s[%d]: %s", name, len(value), joinScalars(value)))
			return nil
		}
		e.line(depth, fmt.Sprintf("- %s[%d]:", name, len(value)))
		for _, item := range value {
			if err := e.listItem(item, depth+2); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported TOON object field %q value %T", key, value)
	}
}

func (e *toonEncoder) line(depth int, value string) {
	e.lines = append(e.lines, strings.Repeat(" ", depth*toonIndent)+value)
}

func sortedKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	// Maps have no encounter order, so byte-order sorting keeps output stable.
	sort.Strings(keys)
	return keys
}

func quotedKeys(keys []string) []string {
	result := make([]string, len(keys))
	for i, key := range keys {
		result[i] = toonKey(key)
	}
	return result
}

func allPrimitive(values []any) bool {
	for _, value := range values {
		if !isPrimitive(value) {
			return false
		}
	}
	return true
}

func isPrimitive(value any) bool {
	switch value.(type) {
	case nil, bool, string, json.Number:
		return true
	default:
		return false
	}
}

func tabularFields(values []any) ([]string, bool) {
	var fields []string
	for i, value := range values {
		object, ok := value.(map[string]any)
		if !ok || len(object) == 0 {
			return nil, false
		}
		keys := sortedKeys(object)
		if i == 0 {
			fields = keys
		} else if strings.Join(keys, "\x00") != strings.Join(fields, "\x00") {
			return nil, false
		}
		for _, key := range keys {
			if !isPrimitive(object[key]) {
				return nil, false
			}
		}
	}
	return fields, true
}

func joinScalars(values []any) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = toonScalar(value)
	}
	return strings.Join(parts, ",")
}

func toonKey(value string) string {
	if toonKeyPattern.MatchString(value) {
		return value
	}
	return quoteTOON(value)
}

func toonScalar(value any) string {
	switch value := value.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(value)
	case string:
		if needsQuotes(value) {
			return quoteTOON(value)
		}
		return value
	case json.Number:
		return canonicalNumber(value.String())
	default:
		panic(fmt.Sprintf("unexpected TOON scalar %T", value))
	}
}

func canonicalNumber(value string) string {
	if integerPattern(value) {
		if strings.TrimLeft(value, "-0") == "" {
			return "0"
		}
		return value
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return "null"
	}
	encoded, err := json.Marshal(number)
	if err != nil {
		panic(fmt.Sprintf("marshal normalized TOON number %q: %v", value, err))
	}
	return string(encoded)
}

func integerPattern(value string) bool {
	for i, char := range value {
		if char == '-' && i == 0 {
			continue
		}
		if char < '0' || char > '9' {
			return false
		}
	}
	return value != "" && value != "-"
}

func needsQuotes(value string) bool {
	if value == "" || strings.TrimSpace(value) != value {
		return true
	}
	if value == "true" || value == "false" || value == "null" || numberPattern.MatchString(value) {
		return true
	}
	if strings.HasPrefix(value, "-") || strings.HasPrefix(value, "#") {
		return true
	}
	for _, char := range value {
		if strings.ContainsRune(`:"\[]{},`, char) || unicode.IsControl(char) {
			return true
		}
	}
	return false
}

func quoteTOON(value string) string {
	var result strings.Builder
	result.WriteByte('"')
	for _, char := range value {
		switch char {
		case '\\':
			result.WriteString(`\\`)
		case '"':
			result.WriteString(`\"`)
		case '\n':
			result.WriteString(`\n`)
		case '\r':
			result.WriteString(`\r`)
		case '\t':
			result.WriteString(`\t`)
		default:
			if unicode.IsControl(char) {
				fmt.Fprintf(&result, `\u%04X`, char)
			} else {
				result.WriteRune(char)
			}
		}
	}
	result.WriteByte('"')
	return result.String()
}
