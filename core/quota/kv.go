// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package quota

import (
	"encoding/json"
	"reflect"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

var _ Checker = (*MapKeyValueSizeChecker)(nil)

// A MapKeyValueSizeChecker can be used to verify that none of the keys and
// values in a map exceed a particular limit.
type MapKeyValueSizeChecker struct {
	maxKeySize   int
	maxValueSize int
	lastErr      error
}

// NewMapKeyValueSizeChecker returns a new MapKeyValueSizeChecker instance that
// limits map keys to maxKeySize and map values to maxValueSize. Any of the
// max values may also set to 0 to disable quota checks.
func NewMapKeyValueSizeChecker(maxKeySize, maxValueSize int) *MapKeyValueSizeChecker {
	return &MapKeyValueSizeChecker{
		maxKeySize:   maxKeySize,
		maxValueSize: maxValueSize,
	}
}

// Check applies the configured size checks to v and updates the checker's
// internal state. Check expects a map as an argument where both the keys and
// the values can be serialized to JSON; any other value will cause an error
// to be returned when Outcome is called.
func (c *MapKeyValueSizeChecker) Check(v interface{}) {
	if v == nil || c.lastErr != nil {
		return
	}

	reflMap := reflect.ValueOf(v)
	if reflMap.Kind() != reflect.Map {
		c.lastErr = errors.Errorf("key/value size check for non map-values %w", coreerrors.NotImplemented)
		return
	}

	for _, mapKey := range reflMap.MapKeys() {
		mapVal := reflMap.MapIndex(mapKey)
		if err := CheckTupleSize(mapKey.Interface(), mapVal.Interface(), c.maxKeySize, c.maxValueSize); err != nil {
			c.lastErr = err
			return
		}
	}
}

// Outcome returns the check outcome or whether an error occurred within a call
// to the Check method.
func (c *MapKeyValueSizeChecker) Outcome() error {
	return c.lastErr
}

// CheckTupleSize checks whether the length of the provided key-value pair is
// within the provided limits. If the key or value is a string, then its length
// will be used for comparison purposes. Otherwise, the effective length is
// calculated by serializing to BSON and counting the length of the serialized
// data.
//
// Any of the max values can be set to zero to bypass the size check.
func CheckTupleSize(key, value interface{}, maxKeyLen, maxValueLen int) error {
	size, err := effectiveSize(key)
	if err != nil {
		return err
	} else if maxKeyLen > 0 && size > maxKeyLen {
		return errors.Errorf("max allowed key length (%d) exceeded %w", maxKeyLen, coreerrors.QuotaLimitExceeded)
	}

	size, err = effectiveSize(value)
	if err != nil {
		return err
	} else if maxValueLen > 0 && size > maxValueLen {
		return errors.Errorf("max allowed value length (%d) exceeded %w", maxValueLen, coreerrors.QuotaLimitExceeded)
	}

	return nil
}

func effectiveSize(v interface{}) (int, error) {
	switch rawValue := v.(type) {
	case string:
		return len(rawValue), nil
	default: // marshal non-string values to bson and return the serialized length
		d, err := json.Marshal(rawValue)
		if err != nil {
			return -1, errors.Errorf("marshaling value to JSON: %w", err)
		}
		return len(d), nil
	}
}
