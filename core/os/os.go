// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// Package os provides access to operating system related configuration.
package os

import (
	"strings"

	"github.com/juju/collections/set"
)

var HostOS = hostOS // for monkey patching

type OSType int

const (
	Unknown OSType = iota
	Ubuntu
	Windows
	OSX
	CentOS
	GenericLinux
	OpenSUSE
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
	case OpenSUSE:
		return "OpenSUSE"
	case Kubernetes:
		return "Kubernetes"
	}
	return "Unknown"
}

var validOSTypeNames set.Strings

func init() {
	osTypes := []string{
		Unknown.String(),
		Ubuntu.String(),
		Windows.String(),
		OSX.String(),
		CentOS.String(),
		GenericLinux.String(),
		OpenSUSE.String(),
		Kubernetes.String(),
	}
	for i, osType := range osTypes {
		osTypes[i] = strings.ToLower(osType)
	}
	validOSTypeNames = set.NewStrings(osTypes...)
}

// IsValidOSTypeName returns true if osType is a
// valid os type name.
func IsValidOSTypeName(osType string) bool {
	return validOSTypeNames.Contains(osType)
}

// HostOSTypeName returns the name of the host OS.
func HostOSTypeName() string {
	return strings.ToLower(HostOS().String())
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
	case Ubuntu, CentOS, GenericLinux, OpenSUSE:
		return true
	}
	return false
}
