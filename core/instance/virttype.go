// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance

import (
	"strings"

	"github.com/juju/errors"
	"github.com/lxc/lxd/shared/api"
)

// VirtType represents the type of virtualisation used by a container.
type VirtType = api.InstanceType

const (
	// DefaultInstanceType is the default instance type to use when no virtType
	// is specified.
	DefaultInstanceType = api.InstanceTypeContainer
)

// ParseVirtType parses a string into a VirtType.
func ParseVirtType(s string) (VirtType, error) {
	switch strings.ToLower(s) {
	case "container":
		return api.InstanceTypeContainer, nil
	case "virtual-machine":
		return api.InstanceTypeVM, nil
	case "":
		// Constraints are optional and the absence of a constraint will
		// fallback to the default instance type (container). This allows
		// adding a machine without specifying a virt-type.
		return DefaultInstanceType, nil
	}
	return "", errors.NotValidf("LXD VirtType %q", s)
}

// MustParseVirtType returns the VirtType for the given string, or panics if
// it's not valid.
// Only use this in tests.
func MustParseVirtType(s string) VirtType {
	v, err := ParseVirtType(s)
	if err != nil {
		panic(err)
	}
	return v
}

// NormaliseVirtType converts the "any" type, which represents an unspecified
// virtual type to a container type. Juju doesn't current support the idea of
// selecting any type of container type.
func NormaliseVirtType(virtType api.InstanceType) api.InstanceType {
	if virtType == api.InstanceTypeAny {
		return api.InstanceTypeContainer
	}
	return virtType
}
