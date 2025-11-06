// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package service

import (
	"context"
	"io"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/agentbinary"
)

// AgentBinaryGetterStore defines a agent binary store that can be used to
// quer and retrieve agent binaries.
type AgentBinaryGetterStore interface {
	AgentBinaryQuerierStore

	// GetAgentBinaryForSHA256 returns the agent binary associated with the
	// given SHA256 sum.
	//
	// The following errors can be expected:
	// - [agentbinaryerrors.NotFound] when no agent binaries exist for the
	// provided sha.
	GetAgentBinaryForSHA256(
		ctx context.Context,
		sha256Sum string,
	) (io.ReadCloser, int64, error)

	// GetAgentBinaryForVersionStream retrieves the agent binary
	// corresponding to the given version and stream. If successfully found the
	// the agent binary stream is returned along with its size and sha256 sum.
	// It is the caller's responsibility to close the returned stream when no
	// error condition exists.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/agentbinary/errors.NotFound] if the agent
	// binary does not exist.
	GetAgentBinaryForVersionStreamSHA256(
		context.Context,
		agentbinary.Version,
		agentbinary.Stream,
	) (io.ReadCloser, int64, string, error)
}

// AgentBinaryPutterStore defines a agent binary store that can be used to
// put agent binaries.
type AgentBinaryPutterStore interface {
	// AddAgentBinaryWithSHA256 adds a new agent binary to the store. The agent
	// binary stream WILL be strictly checked for size and sha.
	//
	// The following errors can be returned:
	// - [coreerrors.NotSupported] if the architecture is not supported.
	// - [agentbinaryerrors.AlreadyExists] if an agent binary already exists for
	// this version and architecture.
	// - [coreerrors.NotValid] if the agent version is not valid.
	// - [agentbinaryerrors.HashMismatch] when the expected sha does not match
	// that which was computed against the binary data.
	AddAgentBinaryWithSHA256(
		context.Context, io.Reader, agentbinary.Version, int64, string,
	) error
}

// AgentBinaryQuerierStore defines an agent binary store that can be queried for
// what is available.
type AgentBinaryQuerierStore interface {
	// GetAvailableForVersionInStream returns the available agent binaries for
	// the provided version and stream in the store.
	//
	// The following errors may be returned:
	// - [coreerrors.NotValid] if the stream value is not valid.
	GetAvailableForVersionInStream(
		context.Context, semversion.Number, agentbinary.Stream,
	) ([]agentbinary.AgentBinary, error)

	// GetAvailablePatchVersionsInStream returns a slice of [agentbinary.AgentBinary]s
	// that are available from store that share the the same major and minor
	// version as that of the supplied version.
	//
	// The following errors may be returned:
	// - [coreerrors.NotValid] if the stream value is not valid.
	GetAvailablePatchVersionsInStream(
		context.Context, semversion.Number, agentbinary.Stream,
	) ([]agentbinary.AgentBinary, error)
}

// AgentBinaryStore defines a agent binary store that can queried, retriefed
// from and put to.
type AgentBinaryStore interface {
	AgentBinaryGetterStore
	AgentBinaryPutterStore
}
