// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package resource

import (
	"github.com/juju/errors"
)

// These are the valid resource types (except for unknown).
const (
	typeUnknown Type = iota
	TypeFile
)

var types = map[Type]string{
	TypeFile: "file",
}

// Type enumerates the recognized resource types.
type Type int

// ParseType converts a string to a Type. If the given value does not
// match a recognized type then an error is returned.
func ParseType(value string) (Type, error) {
	for rt, str := range types {
		if value == str {
			return rt, nil
		}
	}
	return typeUnknown, errors.Errorf("unsupported resource type %q", value)
}

// String returns the printable representation of the type.
func (rt Type) String() string {
	return types[rt]
}

// Validate ensures that the type is valid.
func (rt Type) Validate() error {
	// Ideally, only the (unavoidable) zero value would be invalid.
	// However, typedef'ing int means that the use of int literals
	// could result in invalid Type values other than the zero value.
	if _, ok := types[rt]; !ok {
		return errors.NewNotValid(nil, "unknown resource type")
	}
	return nil
}
