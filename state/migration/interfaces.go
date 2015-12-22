// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/names"
)

type Description interface {
	Model() Model
	// Add/Get binaries
}

type Model interface {
	Id() names.EnvironTag
	Name() string
	Owner() names.UserTag

	Machines() []Machine
}

type Machine interface {
	Id() names.MachineTag

	Containers() []Machine
}
