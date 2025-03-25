// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	coreagentbinary "github.com/juju/juju/core/agentbinary"
)

// Metadata holds the metadata of the agent binary.
type Metadata struct {
	// Version is the version of the agent binary.
	coreagentbinary.Version
	// Size is the size of the agent binary.
	Size int64
	// SHA256 is the SHA256 hash of the agent binary.
	SHA256 string
	// SHA384 is the SHA384 hash of the agent binary.
	SHA384 string
}
