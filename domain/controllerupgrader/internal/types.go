// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/agentbinary"
)

// ControllerNode represents a controller node in the cluster.
type ControllerNode struct {
	ID           string
	Version      semversion.Number
	Architecture agentbinary.Architecture
}
