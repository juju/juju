// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

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
	Arch
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
	case Arch:
		return "Arch"
	}
	return "Unknown"
}
