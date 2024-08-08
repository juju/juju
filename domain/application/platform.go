// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/os/ostype"
)

// OSType represents the type of an application's OS.
type OSType int

const (
	Ubuntu OSType = iota
)

// MarshallOSType converts an os type to a db os type id.
func MarshallOSType(os ostype.OSType) OSType {
	switch os {
	case ostype.Ubuntu:
		return Ubuntu
	default:
		// Ubuntu is all we support.
		return Ubuntu
	}
}

// Architecture represents an application's architecture.
type Architecture int

const (
	AMD64 Architecture = iota
	ARM64
	PPC64EL
	S390X
	RISV64
)

// MarshallArchitecture converts an architecture to a db architecture id.
func MarshallArchitecture(a arch.Arch) Architecture {
	switch a {
	case arch.AMD64:
		return AMD64
	case arch.ARM64:
		return ARM64
	case arch.PPC64EL:
		return PPC64EL
	case arch.S390X:
		return S390X
	case arch.RISCV64:
		return RISV64
	default:
		return AMD64
	}
}
