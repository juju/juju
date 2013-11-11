// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance

import (
	"fmt"
)

type ContainerType string

const (
	NONE = ContainerType("none")
	LXC  = ContainerType("lxc")
	KVM  = ContainerType("kvm")
)

// AllowedContainerTypes is used to validate add-machine arguments.
var AllowedContainerTypes []ContainerType = []ContainerType{
	LXC,
	KVM,
}

// ParseAllowedContainerTypeOrNone converts the specified string into a supported
// ContainerType instance or returns an error if the container type is invalid.
// For this version of the function, 'none' is a valid value.
func ParseAllowedContainerTypeOrNone(ctype string) (ContainerType, error) {
	if ContainerType(ctype) == NONE {
		return NONE, nil
	}
	return ParseAllowedContainerType(ctype)
}

// ParseAllowedContainerType converts the specified string into a supported
// ContainerType instance or returns an error if the container type is invalid.
func ParseAllowedContainerType(ctype string) (ContainerType, error) {
	for _, supportedType := range AllowedContainerTypes {
		if ContainerType(ctype) == supportedType {
			return supportedType, nil
		}
	}
	return "", fmt.Errorf("invalid container type %q", ctype)
}
