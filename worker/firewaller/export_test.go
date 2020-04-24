// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import "github.com/juju/names/v4"

func StartMachine(fw *Firewaller, tag names.MachineTag) error {
	return fw.startMachine(tag)
}

func GetMachineds(fw *Firewaller) map[names.MachineTag]*machineData {
	return fw.machineds
}
