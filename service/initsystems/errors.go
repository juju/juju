// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initsystems

import (
	"fmt"

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

	var err error
	fieldErr := ErrUnsupportedField{
		Field: field,
	}
	err = &fieldErr

	if key != "" {
		err = &ErrUnsupportedItem{
			ErrUnsupportedField: fieldErr,
			Key:                 key,
		}
	}

	// Wrap the error in errors.NotSupported.
	err = errors.NewNotSupported(err, "")
	err.(*errors.Err).SetLocation(2)
	return err
}

// ErrUnsupportedField is an error used to describe a conf field that
// is not supported by an init system. If the value is not supported
// then errors.NotValid should be used instead.
type ErrUnsupportedField struct {
	Field  string
	Reason string
}

// Error implements error.
func (euf ErrUnsupportedField) Error() string {
	label := fmt.Sprintf("field %q", euf.Field)

	if euf.Reason == "" {
		return label
	}
	return fmt.Sprintf("%s: %s", label, euf.Reason)
}

// ErrUnsupportedField is an error used to describe a conf field that
// is not supported by an init system. If the value is not supported
// then errors.NotValid should be used instead.
type ErrUnsupportedItem struct {
	ErrUnsupportedField
	Key string
}

// Error implements error.
func (eui ErrUnsupportedItem) Error() string {
	label := fmt.Sprintf("field %q, item %q", eui.Field, eui.Key)

	if eui.Reason == "" {
		return label
	}
	return fmt.Sprintf("%s: %s", label, eui.Reason)
}
