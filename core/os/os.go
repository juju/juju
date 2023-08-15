// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package os

import (
	"strings"
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
	}
	return "Unknown"
}

var validOSTypeNames map[string]OSType

func init() {
	osTypes := []OSType{
		Unknown,
		Ubuntu,
		Windows,
		CentOS,
		GenericLinux,
	}
	validOSTypeNames = make(map[string]OSType)
	for _, osType := range osTypes {
		validOSTypeNames[strings.ToLower(osType.String())] = osType
	}
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

// HostOSTypeName returns the name of the host OS.
func HostOSTypeName() (osTypeName string) {
	defer func() {
		if err := recover(); err != nil {
			osTypeName = "unknown"
		}
	}()
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
	case Ubuntu, CentOS, GenericLinux:
		return true
	}
	return false
}
