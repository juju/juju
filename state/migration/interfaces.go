// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/names"

	"github.com/juju/juju/version"
)

type Description interface {
	Model() Model
	// Add/Get binaries
}

type Model interface {
	Tag() names.EnvironTag
	Owner() names.UserTag
	Config() map[string]interface{}
	LatestToolsVersion() version.Number

	Machines() []Machine
}

type Machine interface {
	Id() names.MachineTag

	Containers() []Machine
}
