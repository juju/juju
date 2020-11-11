// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import "github.com/juju/names/v4"

func StartMachine(fw *Firewaller, tag names.MachineTag) error {
	// Use channels to synchronize access to unexported startMachine method
	result := make(chan error)
	fw.startMachineEvent <- startMachineEventInfo{tag, result}
	return <-result
}

func GetMachineds(fw *Firewaller) map[names.MachineTag]*machineData {
	// Use channels to synchronize access to unexported getMachineds method
	result := make(chan map[names.MachineTag]*machineData)
	fw.getMachinedsEvent <- getMachinedsEventInfo{result}
	return <-result
}
