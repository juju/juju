// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"time"

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
	Users() []User
	Machines() []Machine

	AddUser(UserArgs)
}

type User interface {
	Name() names.UserTag
	DisplayName() string
	CreatedBy() names.UserTag
	DateCreated() time.Time
	LastConnection() time.Time
	ReadOnly() bool
}

type Machine interface {
	Id() names.MachineTag

	Containers() []Machine
}
