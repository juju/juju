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
	AdminTag       names.UserTag
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

// HasPermission returns true if the logged in user is admin or has a name equal to
// the pre-set admin tag.
func (fa FakeAuthorizer) HasPermission(operation description.Access, target names.Tag) (bool, error) {
	if fa.Tag.Kind() == names.UserTagKind {
		ut := fa.Tag.(names.UserTag)
		if ut.Name() == "admin" {
			return true, nil
		}
		emptyTag := names.UserTag{}
		if fa.AdminTag != emptyTag && ut == fa.AdminTag {
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

// HasPermission returns true if the passed user is admin or has a name equal to
// the pre-set admin tag.
func (fa FakeAuthorizer) UserHasPermission(user names.UserTag, operation description.Access, target names.Tag) (bool, error) {
	if user.Name() == "admin" {
		return true, nil
	}
	emptyTag := names.UserTag{}
	if fa.AdminTag != emptyTag && user == fa.AdminTag {
		return true, nil
	}
	ut := fa.Tag.(names.UserTag)
	if ut == user {
		return true, nil
	}
	return false, nil
}
