// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelagent

import (
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// AgentStream represents the agent stream that is currently being used by a
// model agent.
type AgentStream int

const (
	// agentStreamZero represents the zero value for AgentStream. This is used
	// to check if a value has been set/selected.
	agentStreamZero AgentStream = 0

	// AgentStreamReleased represents the released stream for agent binaries.
	AgentStreamReleased AgentStream = iota
	// AgentStreamProposed represents the proposed stream for agent binaries.
	AgentStreamProposed
	// AgentStreamTesting represents the testing stream for agent binaries.
	AgentStreamTesting
	// AgentStreamDevel represents the devel stream for agent binaries.
	AgentStreamDevel
)

// AgentStreamFromCoreAgentStream converts a [coreagentbinary.AgentStream] to a
// corresponding [AgentStream]. It returns an error if the value is not
// recognised or supported satisfying [coreerrors.NotValid].
func AgentStreamFromCoreAgentStream(
	agentStream coreagentbinary.AgentStream,
) (AgentStream, error) {
	var rval AgentStream

	switch agentStream {
	case coreagentbinary.AgentStreamReleased:
		rval = AgentStreamReleased
	case coreagentbinary.AgentStreamTesting:
		rval = AgentStreamTesting
	case coreagentbinary.AgentStreamProposed:
		rval = AgentStreamProposed
	case coreagentbinary.AgentStreamDevel:
		rval = AgentStreamDevel
	}

	if rval == agentStreamZero {
		return rval, errors.Errorf(
			"agent stream %q is not recognised as a valid value", agentStream,
		).Add(coreerrors.NotValid)
	}

	return rval, nil
}
