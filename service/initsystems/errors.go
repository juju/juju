// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initsystems

import (
	"github.com/juju/errors"
)

// NewUnsupportedField creates a new error for an unsupported conf
// field. No reason is set. The underlying ErrUnsupportedField is
// wrapped in errors.NotSupported.
func NewUnsupportedField(field string) error {
	return newUnsupportedError(field, "", "")
}

// NewUnsupportedItem creates a new error for an unsupported key in a
// conf field (that has a map value). No reason is set. The underlying
// ErrUnsupportedItem is wrapped in errors.NotSupported.
func NewUnsupportedItem(field, key string) error {
	return newUnsupportedError(field, key, "")
}

func newUnsupportedError(field, key, reason string) error {
	if field == "" {
		return nil
	}

	// Compose the message.
	message := "field %q"
	parts := []interface{}{field}
	if key != "" {
		message += ", item %q"
		parts = append(parts, key)
	}
	message += " not supported"
	if reason != "" {
		message += ": %s"
		parts = append(parts, reason)
	}

	// Build the base error.
	baseErr := errors.NewErr(message, parts...)
	baseErr.SetLocation(2)

	// Build the field error.
	var err error
	fieldErr := ErrUnsupportedField{
		Err:   baseErr,
		cause: errors.NotSupportedf(message, parts...),
		Field: field,
	}
	err = &fieldErr

	if key != "" {
		// Build the item error.
		err = &ErrUnsupportedItem{
			ErrUnsupportedField: fieldErr,
			Key:                 key,
		}
	}

	return err
}

// ErrUnsupportedField is an error used to describe a conf field that
// is not supported by an init system. If the value is not supported
// then errors.NotValid should be used instead.
type ErrUnsupportedField struct {
	errors.Err

	cause  error
	Field  string
	Reason string
}

func (euf ErrUnsupportedField) Cause() error {
	return euf.cause
}

// ErrUnsupportedField is an error used to describe a conf field that
// is not supported by an init system. If the value is not supported
// then errors.NotValid should be used instead.
type ErrUnsupportedItem struct {
	ErrUnsupportedField
	Key string
}
