// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"
	"time"

	"github.com/juju/errors"
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
