// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbinary

import (
	"slices"
)

// Architecture represents the architecture of the agent.
type Architecture int

// Exhaustive list of supported architectures for agent binaries within a Juju
// controller installation. These sets of values MUST be kept inline with the
// enum values defined in the database DDL. See
// domain/schema/model/sql/0070-platform.sql and
// domain/schema/controller/sql/0110-platform-core.sql and 0250-platform.sql.
const (
	AMD64 Architecture = iota
	ARM64
	PPC64EL
	S390X
	RISCV64
)

// ArchitectureFromString takes a string representation of an architecture and
// returns the equivalent [Architecture] value. If the string is not recognised
// a invalid [Architecture] value and false is returned.
func ArchitectureFromString(a string) (Architecture, bool) {
	switch a {
	case "amd64":
		return AMD64, true
	case "arm64":
		return ARM64, true
	case "ppc64el":
		return PPC64EL, true
	case "s390x":
		return S390X, true
	case "riscv64":
		return RISCV64, true
	default:
		return -1, false
	}
}

// ArchitectureNotIn returns a the slice of [Architecture]s from a that do not
// exist in b. Nilness of a is guaranteed to be preserved.
func ArchitectureNotIn(a, b []Architecture) []Architecture {
	var retVal []Architecture
	for _, archA := range a {
		if slices.Contains(b, archA) {
			continue
		}
		retVal = append(retVal, archA)
	}
	return retVal
}

// IsValid returns true if the [Architecture] is a supported valid value.
func (a Architecture) IsValid() bool {
	switch a {
	case AMD64, ARM64, PPC64EL, S390X, RISCV64:
		return true
	}
	return false
}

// String returns the primitive string values for [Architecture].
//
// Implements the [fmt.Stringer] interface.
func (a Architecture) String() string {
	switch a {
	case AMD64:
		return "amd64"
	case ARM64:
		return "arm64"
	case PPC64EL:
		return "ppc64el"
	case S390X:
		return "s390x"
	case RISCV64:
		return "riscv64"
	default:
		return ""
	}
}
