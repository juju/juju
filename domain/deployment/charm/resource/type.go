// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package resource

import (
	coreerrors "github.com/juju/juju/core/errors"
	internalerrors "github.com/juju/juju/internal/errors"
)

// These are the valid resource types (except for unknown).
const (
	typeUnknown Type = iota
	TypeFile
	TypeContainerImage
)

var types = map[Type]string{
	TypeFile:           "file",
	TypeContainerImage: "oci-image",
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
	return typeUnknown, internalerrors.Errorf("unsupported resource type %q", value)
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
		return internalerrors.Errorf("unknown resource type").Add(coreerrors.NotValid)
	}
	return nil
}
