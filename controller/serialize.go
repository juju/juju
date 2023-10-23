// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"
	"reflect"
	"time"

	"github.com/juju/errors"
	"github.com/juju/schema"
)

// EncodeToString encodes the given controller config into a map of strings.
func EncodeToString(cfg Config) (map[string]string, error) {
	result := make(map[string]string, len(cfg))
	for key, v := range cfg {
		switch v := v.(type) {
		case string:
			result[key] = v
		case bool:
			result[key] = fmt.Sprintf("%t", v)
		case int, int8, int16, int32, int64:
			result[key] = fmt.Sprintf("%d", v)
		case uint, uint8, uint16, uint32, uint64:
			result[key] = fmt.Sprintf("%d", v)
		case float32, float64:
			result[key] = fmt.Sprintf("%f", v)
		case time.Duration:
			result[key] = v.String()
		case time.Time:
			result[key] = v.Format(time.RFC3339Nano)
		default:
			return nil, errors.Errorf("unable to serialize controller config key %q: unknown type %T", key, v)
		}
	}
	return result, nil
}

var timeDurationType = reflect.TypeOf(schema.TimeDuration())

// EncodeForJSON returns a map[string]any of fields that aren't encodable for
// JSON (time.Duration) are encoded as strings.
// Currently this only takes care of time duration types, but it could easily
// be extended to handle other types.
func EncodeForJSON(c Config) map[string]any {
	m := make(map[string]any)
	for key, value := range c {
		field, ok := configFields[key]
		// Handle the case where the field is not in the schema.
		if !ok {
			m[key] = value
			continue
		}

		// Ensure that the schema identifies it as TimeDuration.
		if reflect.TypeOf(field) != timeDurationType {
			m[key] = value
			continue
		}

		// Force the time.Duration to be marshalled as a string.
		switch t := value.(type) {
		case time.Duration:
			m[key] = t.String()
		default:
			m[key] = t
		}
	}
	return m
}
