// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance

import (
	"strings"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// VirtType represents the type of virtualisation used by a container.
type VirtType string

const (
	// DefaultInstanceType is the default instance type to use when no virtType
	// is specified.
	DefaultInstanceType VirtType = "container"

	// AnyInstanceType is a special instance type that represents an unspecified
	// virtual type. If this is used, then the instance type will be determined
	// by the default instance type.
	AnyInstanceType VirtType = ""

	// InstanceTypeContainer is the instance type for a container.
	InstanceTypeContainer VirtType = "container"

	// InstanceTypeVM is the instance type for a virtual machine.
	InstanceTypeVM VirtType = "virtual-machine"
)

// IsAny returns true if the VirtType is AnyInstanceType.
func (v VirtType) IsAny() bool {
	return v == AnyInstanceType || v == ""
}

func (v VirtType) String() string {
	return string(v)
}

// ParseVirtType parses a string into a VirtType.
func ParseVirtType(s string) (VirtType, error) {
	switch strings.ToLower(s) {
	case "container":
		return InstanceTypeContainer, nil
	case "virtual-machine":
		return InstanceTypeVM, nil
	case "":
		// Constraints are optional and the absence of a constraint will
		// fallback to the default instance type (container). This allows
		// adding a machine without specifying a virt-type.
		return DefaultInstanceType, nil
	}
	return "", errors.Errorf("LXD VirtType %q %w", s, coreerrors.NotValid)
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
