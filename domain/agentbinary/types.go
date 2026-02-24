// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbinary

import (
	"iter"

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

// function to remove all [AgentBinary] values from a slice that have the same
// version.
//
// This func ignores [AgentBinary.Stream] and [AgentBinary.Architecture] when
// comparing two [AgentBinary] values.
func AgentBinaryCompactOnVersion(a, b AgentBinary) bool {
	return a.Version.Compare(b.Version) == 0
}

// AgentBinaryCompareOnVersion provides a comparison func for comparing
// [AgentBinary] against their [AgentBinary.Version] field.
func AgentBinaryCompareOnVersion(a, b AgentBinary) int {
	return a.Version.Compare(b.Version)
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
