// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"bytes"
	"maps"

	"gopkg.in/yaml.v3"

	"github.com/juju/juju/internal/errors"
)

// decodeRows converts archive rows to generated bindings before any writes.
// KnownFields rejects columns that are not part of the compiled schema.
func decodeRows[T any](rows []row, binaryColumns ...string) ([]T, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	if len(binaryColumns) > 0 {
		// Only schema-identified byte columns are converted. Clone their
		// rows so validation and retries leave the archive input unchanged.
		converted := make([]row, len(rows))
		for i, source := range rows {
			converted[i] = maps.Clone(source)
			for _, column := range binaryColumns {
				value, present := source[column]
				if !present || value == nil {
					continue
				}
				decoded, err := decodeBinaryValue(value)
				if err != nil {
					return nil, errors.Errorf("row %d column %q: %w", i, column, err)
				}
				converted[i][column] = decoded
			}
		}
		rows = converted
	}
	data, err := yaml.Marshal(rows)
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
