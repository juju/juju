// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbinary

import (
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// Stream represents the agent stream that is currently being used by a
// model agent.
type Stream int

const (
	// AgentStreamReleased represents the released stream for agent binaries.
	AgentStreamReleased Stream = iota
	// AgentStreamProposed represents the proposed stream for agent binaries.
	AgentStreamProposed
	// AgentStreamTesting represents the testing stream for agent binaries.
	AgentStreamTesting
	// AgentStreamDevel represents the devel stream for agent binaries.
	AgentStreamDevel
)

// StreamFromCoreAgentBinaryStream converts a [coreagentbinary.AgentStream] to a
// corresponding [Stream]. It returns an error if the value is not
// recognised or supported satisfying [coreerrors.NotValid].
func StreamFromCoreAgentBinaryStream(
	agentStream coreagentbinary.AgentStream,
) (Stream, error) {
	switch agentStream {
	case coreagentbinary.AgentStreamReleased:
		return AgentStreamReleased, nil
	case coreagentbinary.AgentStreamTesting:
		return AgentStreamTesting, nil
	case coreagentbinary.AgentStreamProposed:
		return AgentStreamProposed, nil
	case coreagentbinary.AgentStreamDevel:
		return AgentStreamDevel, nil
	}

	return Stream(-1), errors.Errorf(
		"agent stream %q is not recognised as a valid value", agentStream,
	).Add(coreerrors.NotValid)
}

// IsValid checks if the [Stream] is a valid value.
func (s Stream) IsValid() bool {
	switch s {
	case AgentStreamReleased, AgentStreamProposed, AgentStreamTesting, AgentStreamDevel:
		return true
	default:
		return false
	}
}

// String returns the primitive string values of [Stream].
// It is the client's responsibility to validate that [Stream]
// is indeed valid before calling [Stream.String].
func (s Stream) String() string {
	switch s {
	case AgentStreamReleased:
		return "released"
	case AgentStreamProposed:
		return "proposed"
	case AgentStreamTesting:
		return "testing"
	case AgentStreamDevel:
		return "devel"
	}

	return ""
}
