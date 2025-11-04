// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package service

import (
	"context"
	"io"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/agentbinary"
)

// AgentBinaryGetterStore defines a agent binary store that can be used to
// retrieve agent binaries.
type AgentBinaryGetterStore interface {
	// GetAgentBinaryWithSHA256 retrieves the agent binary corresponding to the given version
	// and stream from an external store.
	// The caller is responsible for closing the returned reader.
	//
	// The following errors may be returned:
	// - [domainagenterrors.NotFound] if the agent binary metadata does not exist.
	GetAgentBinaryWithSHA256(
		context.Context,
		coreagentbinary.Version,
		agentbinary.Stream,
	) (io.ReadCloser, int64, string, error)
}

// AgentBinaryPutterStore defines a agent binary store that can be used to
// put agent binaries.
type AgentBinaryPutterStore interface {
}

// AgentBinaryQuerierStore defines a agent binary store that can be queried for
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

	// GetAvailablePatchVersions returns a slice of [agentbinary.AgentBinary]s
	// that are available from store that share the the same major and minor
	// version as that of the supplied version.
	//
	// The following errors may be returned:
	// - [coreerrors.NotValid] if the stream value is not valid.
	GetAvailablePatchVersionsInStream(
		context.Context, semversion.Number, agentbinary.Stream,
	)
}

// AgentBinaryStore defines a agent binary store that can queried, retriefed
// from and put to.
type AgentBinaryStore interface {
	AgentBinaryGetterStore
	AgentBinaryPutterStore
	AgentBinaryQuerierStore
}
