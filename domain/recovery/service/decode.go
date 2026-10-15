// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"bytes"
	"maps"
	"reflect"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/juju/juju/internal/errors"
)

// decodeRows converts complete native archive rows to the shared export types.
// Validate presence and nullability before decoding: YAML otherwise silently
// turns missing or null values into zero values for non-pointer fields.
// KnownFields rejects columns that are not part of the compiled schema.
func decodeRows[T any](rows []row, binaryColumns ...string) ([]T, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	rowType := reflect.TypeFor[T]()
	converted := make([]row, len(rows))
	for i, source := range rows {
		converted[i] = maps.Clone(source)
		for field := range rowType.Fields() {
			column := field.Tag.Get("yaml")
			value, present := source[column]
			if !present {
				return nil, errors.Errorf("row %d is missing column %q", i, column)
			}
			kind := field.Type.Kind()
			if value == nil {
				if kind != reflect.Pointer && kind != reflect.Slice && kind != reflect.Interface {
					return nil, errors.Errorf("row %d column %q cannot be null", i, column)
				}
				continue
			}
			if !slices.Contains(binaryColumns, column) {
				continue
			}
			decoded, err := decodeBinaryValue(value)
			if err != nil {
				return nil, errors.Errorf("row %d column %q: %w", i, column, err)
			}
			if kind == reflect.Pointer {
				kind = field.Type.Elem().Kind()
			}
			if kind == reflect.String {
				// Legacy TEXT columns use export strings. Replay converts them
				// to byte bindings before they reach the database driver.
				converted[i][column] = string(decoded)
			} else {
				converted[i][column] = decoded
			}
		}
	}
	data, err := yaml.Marshal(converted)
	if err != nil {
		return nil, errors.Capture(err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var result []T
	if err := decoder.Decode(&result); err != nil {
		return nil, errors.Capture(err)
	}
	return result, nil
}

// decodeBinaryValue accepts the byte representations emitted by native
// exporters. YAML decodes !!binary scalars as strings and byte sequences as
// slices of integers; valid UTF-8 binary values can also be plain strings.
func decodeBinaryValue(value any) ([]byte, error) {
	switch value := value.(type) {
	case string:
		return []byte(value), nil
	case []byte:
		return value, nil
	case []any:
		result := make([]byte, len(value))
		for i, element := range value {
			n, ok := element.(int)
			if !ok || n < 0 || n > 255 {
				return nil, errors.Errorf("byte sequence element %d is not a byte (%T)", i, element)
			}
			result[i] = byte(n)
		}
		return result, nil
	default:
		return nil, errors.Errorf("invalid binary value type %T", value)
	}
}
