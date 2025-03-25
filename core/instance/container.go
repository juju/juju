// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance

import "github.com/juju/juju/internal/errors"

// ContainerType defines different container technologies known to juju.
type ContainerType string

// Known container types.
const (
	NONE ContainerType = "none"
	LXD  ContainerType = "lxd"
)

// ContainerTypes is used to validate add-machine arguments.
var ContainerTypes = []ContainerType{
	LXD,
}

// ParseContainerTypeOrNone converts the specified string into a supported
// ContainerType instance or returns an error if the container type is invalid.
// For this version of the function, 'none' is a valid value.
func ParseContainerTypeOrNone(ctype string) (ContainerType, error) {
	if ContainerType(ctype) == NONE {
		return NONE, nil
	}
	return ParseContainerType(ctype)
}

// ParseContainerType converts the specified string into a supported
// ContainerType instance or returns an error if the container type is invalid.
func ParseContainerType(ctype string) (ContainerType, error) {
	for _, supportedType := range ContainerTypes {
		if ContainerType(ctype) == supportedType {
			return supportedType, nil
		}
	}
	return "", errors.Errorf("invalid container type %q", ctype)
}
