// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package ostype

import (
	"strings"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

type OSType int

const (
	Unknown OSType = iota
	Ubuntu
	Windows
	OSX
	CentOS
	GenericLinux
	Kubernetes
)

func (t OSType) String() string {
	switch t {
	case Ubuntu:
		return "Ubuntu"
	case Windows:
		return "Windows"
	case OSX:
		return "OSX"
	case CentOS:
		return "CentOS"
	case GenericLinux:
		return "GenericLinux"
	case Kubernetes:
		return "Kubernetes"
	}
	return "Unknown"
}

// EquivalentTo returns true if the OS type is equivalent to another
// OS type.
func (t OSType) EquivalentTo(t2 OSType) bool {
	if t == t2 {
		return true
	}
	return t.IsLinux() && t2.IsLinux()
}

// IsLinux returns true if the OS type is a Linux variant.
func (t OSType) IsLinux() bool {
	switch t {
	case Ubuntu, CentOS, GenericLinux:
		return true
	}
	return false
}

var validOSTypeNames = map[string]OSType{
	"ubuntu":       Ubuntu,
	"windows":      Windows,
	"osx":          OSX,
	"centos":       CentOS,
	"genericlinux": GenericLinux,
	"kubernetes":   Kubernetes,
}

// IsValidOSTypeName returns true if osType is a
// valid os type name.
func IsValidOSTypeName(osType string) bool {
	for n := range validOSTypeNames {
		if n == osType {
			return true
		}
	}
	return false
}

// OSTypeForName return the named OS.
func OSTypeForName(name string) OSType {
	os, ok := validOSTypeNames[name]
	if ok {
		return os
	}
	return Unknown
}

// ParseOSType parses a string and returns the corresponding OSType.
func ParseOSType(s string) (OSType, error) {
	osType, ok := validOSTypeNames[strings.ToLower(s)]
	if !ok {
		return Unknown, errors.Errorf("unknown os type %q %w", s, coreerrors.NotValid)
	}
	return osType, nil
}
