// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	coreaction "github.com/juju/juju/core/action"
)

// Action represents a domain action.
type Action struct {
	// UUID is the action unique identifier.
	UUID coreaction.UUID
	// Receiver is the action receiver (unit / machine).
	Receiver string
}
