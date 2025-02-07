// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package virtualhostkeys provides unique identifiers
// for unit and machine host keys. Used for setting
// and getting virtual host keys from state.
package virtualhostkeys

type UnitLookup struct {
	ID string
}

type MachineLookup struct {
	ID string
}

// UnitHostKeyID provides the virtual host key
// lookup value for a unit based on the unit name.
func UnitHostKeyID(unitName string) UnitLookup {
	s := "unit" + "-" + unitName + "-" + "hostkey"
	return UnitLookup{ID: s}
}

// MachineHostKeyID provides the virtual host key
// lookup value for a machine based on the machine ID.
func MachineHostKeyID(machineID string) MachineLookup {
	s := "machine" + "-" + machineID + "-" + "hostkey"
	return MachineLookup{ID: s}
}
