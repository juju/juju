// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// Package os provides access to operating system related configuration.
package os

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
