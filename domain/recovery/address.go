// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package recovery

import "github.com/juju/juju/core/network"

// ControllerUnitAddress is a replacement observation persisted during recovery.
type ControllerUnitAddress struct {
	UUID  string
	Value string
	Type  network.AddressType
}

// ControllerUnitAddressPatch supplies identifiers before persistence starts.
// A device is created only when IP addresses need one and none is recorded.
type ControllerUnitAddressPatch struct {
	DeviceUUID string
	Addresses  []ControllerUnitAddress
}
