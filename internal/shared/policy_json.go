package shared

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

var errPolicyJSON = errors.New("invalid policy JSON object")

// PolicyJSON parses a bounded request body once, rejecting ambiguous member
// names before returning an object suitable for validation and re-encoding.
func PolicyJSON(body []byte) (map[string]any, error) {
	if !utf8.Valid(body) {
		return nil, errPolicyJSON
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	value, err := policyJSONValue(decoder, 0)
	object, ok := value.(map[string]any)
	if err != nil || !ok {
		return nil, errPolicyJSON
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errPolicyJSON
	}
	return object, nil
}

func policyJSONValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > 64 {
		return nil, errPolicyJSON
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch token {
	case json.Delim('{'):
		object := make(map[string]any)
		keys := make(map[string]struct{})
		for decoder.More() {
			token, err := decoder.Token()
			key, ok := token.(string)
			if err != nil || !ok {
				return nil, errPolicyJSON
			}
			folded := strings.Map(func(r rune) rune {
				minimum := r
				for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
					minimum = min(minimum, next)
				}
				return minimum
			}, key)
			if _, duplicate := keys[folded]; duplicate {
				return nil, errPolicyJSON
			}
			keys[folded] = struct{}{}
			value, err := policyJSONValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		_, err := decoder.Token()
		return object, err
	case json.Delim('['):
		list := []any{}
		for decoder.More() {
			value, err := policyJSONValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			list = append(list, value)
		}
		_, err := decoder.Token()
		return list, err
	default:
		return token, nil
	}
}
