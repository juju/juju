// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package quota

import (
	"encoding/json"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// KeyValue describes a setting or state entry that has a string key and value.
type KeyValue interface {
	Key() string
	Value() string
}

// CheckTupleSize checks whether the length of the provided key-value pair is
// within the provided limits. If the key or value is a string, then its length
// will be used for comparison purposes. Otherwise, the effective length is
// calculated by serializing to JSON and counting the length of the serialized
// data.
//
// Any of the max values can be set to zero to bypass the size check.
func CheckTupleSize(key, value any, maxKeyLen, maxValueLen int) error {
	size, err := effectiveSize(key)
	if err != nil {
		return err
	} else if maxKeyLen > 0 && size > maxKeyLen {
		return errors.Errorf("max allowed key length (%d): %w", maxKeyLen, coreerrors.QuotaLimitExceeded)
	}

	size, err = effectiveSize(value)
	if err != nil {
		return err
	} else if maxValueLen > 0 && size > maxValueLen {
		return errors.Errorf("max allowed value length (%d): %w", maxValueLen, coreerrors.QuotaLimitExceeded)
	}

	return nil
}

// CheckRelationSettingsSize checks whether the total raw byte length of all
// relation setting keys and values is within the allowed limit.
func CheckRelationSettingsSize(settings []KeyValue) error {
	return CheckKeyValueTotalSize(settings, MaxRelationSettingsSize)
}

// CheckApplicationConfigSize checks whether the total raw byte length of all
// application config keys and values is within the allowed limit.
func CheckApplicationConfigSize(config []KeyValue) error {
	return CheckKeyValueTotalSize(config, MaxApplicationConfigSize)
}

// CheckKeyValueTotalSize checks whether the total raw byte length of all keys
// and values is within the provided limit.
// A maxSize of zero disables the check.
// Note that we are counting bytes here.
func CheckKeyValueTotalSize(settings []KeyValue, maxSize int) error {
	if maxSize <= 0 {
		return nil
	}

	var total int
	for _, setting := range settings {
		total += len(setting.Key()) + len(setting.Value())
		if total > maxSize {
			return errors.Errorf("max allowed total size (%d): %w", maxSize, coreerrors.QuotaLimitExceeded)
		}
	}
	return nil
}

func effectiveSize(v any) (int, error) {
	switch rawValue := v.(type) {
	case string:
		return len(rawValue), nil
	default:
		// Marshal non-string values to json and return the serialized length.
		d, err := json.Marshal(rawValue)
		if err != nil {
			return -1, errors.Errorf("marshaling value to JSON: %w", err)
		}
		return len(d), nil
	}
}
