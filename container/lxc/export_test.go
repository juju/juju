// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import "launchpad.net/juju-core/container"

func GetMachineId(container container.Container) (machineId string, ok bool) {
	lxc, ok := container.(*lxcContainer)
	if ok {
		machineId = lxc.machineId
	}
	return
}
