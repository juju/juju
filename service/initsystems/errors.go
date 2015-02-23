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
	err := ErrUnsupportedField{
		Err:   baseErr,
		Field: field,
	}
	if key == "" {
		return &err
	}

	// Build the item error.
	return &ErrUnsupportedItem{
		ErrUnsupportedField: err,
		Key:                 key,
	}
}

// ErrUnsupportedField is an error used to describe a conf field that
// is not supported by an init system. If the value is not supported
// then errors.NotValid should be used instead.
type ErrUnsupportedField struct {
	errors.Err

	// Field is the name of the field that the init system does not support.
	Field string

	// Reason indicates why the field is not supported.
	Reason string
}

// ErrUnsupportedField is an error used to describe a conf field that
// is not supported by an init system. If the value is not supported
// then errors.NotValid should be used instead.
type ErrUnsupportedItem struct {
	ErrUnsupportedField

	// Key is the mapping field's key which the init system does not support.
	Key string
}
