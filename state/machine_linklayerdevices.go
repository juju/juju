// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// AllDeviceAddresses returns all known addresses assigned to
// link-layer devices on the machine.
func (m *Machine) AllDeviceAddresses() ([]*Address, error) {
	var allAddresses []*Address
	return allAddresses, nil
}
