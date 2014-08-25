// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/names"
)

// FakeAuthorizer implements the common.Authorizer interface.
type FakeAuthorizer struct {
	Tag            names.Tag
	EnvironManager bool
}

func (fa FakeAuthorizer) AuthOwner(tag names.Tag) bool {
	return fa.Tag == tag
}

func (fa FakeAuthorizer) AuthEnvironManager() bool {
	return fa.EnvironManager
}

// AuthMachineAgent returns whether the current client is a machine agent.
func (fa FakeAuthorizer) AuthMachineAgent() bool {
	_, isMachine := fa.GetAuthTag().(names.MachineTag)
	return isMachine
}

// AuthUnitAgent returns whether the current client is a unit agent.
func (fa FakeAuthorizer) AuthUnitAgent() bool {
	_, isUnit := fa.GetAuthTag().(names.UnitTag)
	return isUnit
}

// AuthClient returns whether the authenticated entity is a client
// user.
func (fa FakeAuthorizer) AuthClient() bool {
	_, isUser := fa.GetAuthTag().(names.UserTag)
	return isUser
}

func (fa FakeAuthorizer) GetAuthTag() names.Tag {
	return fa.Tag
}
