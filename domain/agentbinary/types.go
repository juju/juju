// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbinary

import (
	"iter"
	"slices"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
)

// AgentBinary represents an agent binary without implying a source or
// availability.
type AgentBinary struct {
	// Architecture is the architecture of the agent binary.
	Architecture Architecture

	// Stream represents the stream the agent binary is applicable to.
	Stream Stream

	// Version is the version of the agent binary.
	Version semversion.Number
}

// RegisterAgentBinaryArg describes the arguments for adding an agent binary.
// It contains the version, architecture, and object store UUID of the agent binary.
// The object store UUID is the primary key of the object store record where the
// agent binary is stored.
type RegisterAgentBinaryArg struct {
	// Version is the version of the agent binary.
	Version string
	// Arch is the architecture of the agent binary.
	Arch string
	// ObjectStoreUUID is the UUID primary key of the object store record where the agent binary is stored.
	ObjectStoreUUID objectstore.UUID
}

// Metadata describes the metadata of an agent binary.
// It contains the version, size, and SHA256 hash of the agent binary.
type Metadata struct {
	// Version is the version of the agent binary.
	Version string
	// Arch is the architecture of the agent binary.
	Arch string
	// Size is the size of the agent binary.
	Size int64
	// SHA256 is the SHA256 hash of the agent binary.
	// TODO: do we want to switch to the SHA384 hash?
	SHA256 string
}

// Architecture represents the architecture of the agent.
type Architecture int

const (
	AMD64 Architecture = iota
	ARM64
	PPC64EL
	S390X
	RISCV64
)

// AgentBinaryArchitectures provides a sequence of architectures from a slice
// of [AgentBinary]s. No deduplication is performed.
func AgentBinaryArchitectures(abs []AgentBinary) iter.Seq[Architecture] {
	return func(yield func(Architecture) bool) {
		for _, ab := range abs {
			if !yield(ab.Architecture) {
				return
			}
		}
	}
}

// AgentBinaryCompactOnVersion is a func for use with the [slices.CompactFunc]
// function to remove all [AgentBinary] values from a slice that have the same
// version.
//
// This func ignores [AgentBinary.Stream] and [AgentBinary.Architecture] when
// comparing two [AgentBinary] values.
func AgentBinaryCompactOnVersion(a, b AgentBinary) bool {
	return a.Version.Compare(b.Version) == 0
}

// AgentBinaryNotMatchingVersion provides a helper closure to use with the
// slices package for filtering agent binaries that do match the supplied
// version.
func AgentBinaryNotMatchingVersion(v semversion.Number) func(AgentBinary) bool {
	return func(a AgentBinary) bool {
		return a.Version.Compare(v) != 0
	}
}

// AgentBinaryHighestVersion is a func for use with [slices.MaxFunc] to extract
// the highest [AgentBinary.Version] available in a slice.
//
// This func ignores [AgentBinary.Stream] and [AgentBinary.Architecture] when
// comparing two [AgentBinary] values.
func AgentBinaryHighestVersion(a, b AgentBinary) int {
	return a.Version.Compare(b.Version)
}

// ArchitectureNotIn returns a the slice of [Architecture]s from a that do not
// exist in b. Nilnes of a is guaranteed to be preserved.
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

// ArchitectureFromString takes a string representation of an architecture and
// returns the equivalent [Architecture] value. If the string  is not recognised
// a zero value [Architecture] and false is returned.
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
		return 0, false
	}
}

// String returns the primitive string values for [Architecture].
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

// SupportedArchitectures returns a slice of all supported architectures for
// Juju binaries.
func SupportedArchitectures() []Architecture {
	return []Architecture{AMD64, ARM64, PPC64EL, S390X, RISCV64}
}
