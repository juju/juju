// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"fmt"

	"github.com/juju/errors"
)

// InvalidConfigValueError indicates that one of the config values failed validation.
type InvalidConfigValueError struct {
	errors.Err

	// Key is the OS env var corresponding to the field with the bad value.
	Key string

	// Value is the invalid value.
	Value interface{}
}

// IsInvalidConfigValueError returns whether or not the cause of
// the provided error is a *InvalidConfigValueError.
func IsInvalidConfigValueError(err error) bool {
	_, ok := errors.Cause(err).(*InvalidConfigValueError)
	return ok
}

// NewInvalidConfigValueError returns a new InvalidConfigValueError for the given
// info. If the provided reason is an error then Reason is set to that
// error. Otherwise a non-nil value is treated as a string and Reason is
// set to a non-nil value that wraps it.
func NewInvalidConfigValueError(key, value string, reason error) error {
	err := &InvalidConfigValueError{
		Err:   *errors.Mask(reason).(*errors.Err),
		Key:   key,
		Value: value,
	}
	err.Err.SetLocation(1)
	return err
}

// Cause implements errors.Causer.Cause.
func (err *InvalidConfigValueError) Cause() error {
	return err
}

// NewMissingConfigValue returns a new error for a missing config field.
func NewMissingConfigValue(key, field string) error {
	return NewInvalidConfigValueError(key, "", errors.New("missing "+field))
}

// Error implements error.
func (err InvalidConfigValueError) Error() string {
	return fmt.Sprintf("invalid config value (%s) for %q: %v", err.Value, err.Key, &err.Err)
}
