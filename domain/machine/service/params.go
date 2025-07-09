// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import "github.com/juju/juju/core/machine"

// AddMachineResults contains the results of adding a machine, i.e. the
// machine's name along with a (optional) child machine name.
type AddMachineResults struct {
	MachineName      machine.Name
	ChildMachineName *machine.Name
}
