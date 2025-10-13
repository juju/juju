// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbinary

import (
	"github.com/juju/juju/core/objectstore"
)

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
	Unknown Architecture = iota - 1

	AMD64
	ARM64
	PPC64EL
	S390X
	RISCV64
)

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
