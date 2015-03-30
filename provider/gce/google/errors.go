// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"fmt"

	"github.com/juju/errors"
)

// TODO(ericsnow) Implement InvalidCredential in terms of errors.Chain.

// InvalidCredential indicates that one of the credentials failed validation.
type InvalidCredential struct {
	errors.Err
	cause error

	// Key is the OS env var corresponding to the field with the bad value.
	Key string

	// Value is the invalid value.
	Value interface{}

	// Reason is the underlying error.
	Reason error
}

// NewInvalidCredential returns a new InvalidCredential for the given
// info. If the provided reason is an error then Reason is set to that
// error. Otherwise a non-nil value is treated as a string and Reason is
// set to a non-nil value that wraps it.
func NewInvalidCredential(key string, value, reason interface{}) error {
	var underlying error
	switch reason := reason.(type) {
	case error:
		underlying = reason
	default:
		if reason != nil {
			underlying = errors.Errorf("%v", reason)
		}
	}
	err := &InvalidCredential{
		cause:  errors.NewNotValid(underlying, "GCE auth value"),
		Key:    key,
		Value:  value,
		Reason: underlying,
	}
	msg := "auth value for " + key
	if value != nil {
		if strValue, ok := value.(string); ok {
			if strValue != "" {
				msg = fmt.Sprintf("%s %q", msg, strValue)
			}
		} else {
			msg = fmt.Sprintf("%s (%v)", msg, value)
		}
	}
	err.Err = errors.NewErr("auth value")
	err.Err.SetLocation(1)
	return err
}

// Cause implements errors.causer. This is necessary so that
// errors.IsNotValid works.
func (err *InvalidCredential) Cause() error {
	return err.cause
}

// Underlying implements errors.wrapper.
func (err InvalidCredential) Underlying() error {
	return err.cause
}

// Error implements error.
func (err InvalidCredential) Error() string {
	return fmt.Sprintf("invalid auth value (%s) for %q: %v", err.Value, err.Key)
}
