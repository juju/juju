// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"encoding/json"

	"github.com/juju/juju/internal/errors"
)

// encodeConfigValue encodes the input value to a string that can be stored
// in the database.
func encodeConfigValue(value any) (string, error) {
	str, err := json.Marshal(value)
	return string(str), err
}

// decodeConfigValue decodes the input string stored in the DB
// to its original type.
func decodeConfigValue(storedStr string) (any, error) {
	var value any
	err := json.Unmarshal([]byte(storedStr), &value)
	if err != nil {
		return nil, err
	}
	if str, ok := value.(string); ok && str == "" {
		return nil, errors.New("empty string")
	}
	return value, nil
}
