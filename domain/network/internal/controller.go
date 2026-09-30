// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/network"
)

// ControllerUnitNetwork contains the network facts for a controller unit.
// Addresses belong to the unit's net node, never its application's Service.
type ControllerUnitNetwork struct {
	ModelType model.ModelType
	Addresses network.ControllerAPIAddresses
}
