// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/core/description"
)

// FakeAuthorizer implements the facade.Authorizer interface.
type FakeAuthorizer struct {
	Tag            names.Tag
	EnvironManager bool
	ModelUUID      string
}

func (fa FakeAuthorizer) AuthOwner(tag names.Tag) bool {
	return fa.Tag == tag
}

func (fa FakeAuthorizer) AuthModelManager() bool {
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

func (fa FakeAuthorizer) HasPermission(operation description.Access, target names.Tag) (bool, error) {
	if fa.Tag.Kind() == names.UserTagKind {
		ut := fa.Tag.(names.UserTag)
		if ut.Name() == "admin" {
			return true, nil
		}
		return false, nil
	}
	return true, nil
}

// ConnectedModel returns the UUID of the model the current client is
// connected to.
func (fa FakeAuthorizer) ConnectedModel() string {
	return fa.ModelUUID
}
