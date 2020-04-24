// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"strings"

	"github.com/juju/juju/core/permission"
	"github.com/juju/names/v4"
)

// FakeAuthorizer implements the facade.Authorizer interface.
type FakeAuthorizer struct {
	Tag         names.Tag
	Controller  bool
	ModelUUID   string
	AdminTag    names.UserTag
	HasWriteTag names.UserTag
}

func (fa FakeAuthorizer) AuthOwner(tag names.Tag) bool {
	return fa.Tag == tag
}

func (fa FakeAuthorizer) AuthController() bool {
	return fa.Controller
}

// AuthMachineAgent returns whether the current client is a machine agent.
func (fa FakeAuthorizer) AuthMachineAgent() bool {
	// TODO(controlleragent) - add AuthControllerAgent function
	_, isMachine := fa.GetAuthTag().(names.MachineTag)
	_, isController := fa.GetAuthTag().(names.ControllerAgentTag)
	return isMachine || isController
}

// AuthApplicationAgent returns whether the current client is an application operator.
func (fa FakeAuthorizer) AuthApplicationAgent() bool {
	_, isApp := fa.GetAuthTag().(names.ApplicationTag)
	return isApp
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
func (fa FakeAuthorizer) HasPermission(operation permission.Access, target names.Tag) (bool, error) {
	if fa.Tag.Kind() == names.UserTagKind {
		ut := fa.Tag.(names.UserTag)
		emptyTag := names.UserTag{}
		if fa.AdminTag != emptyTag && ut == fa.AdminTag {
			return true, nil
		}
		if ut == fa.HasWriteTag && (operation == permission.WriteAccess || operation == permission.ReadAccess) {
			return true, nil
		}

		uTag := fa.Tag.(names.UserTag)
		return nameBasedHasPermission(uTag.Name(), operation, target), nil
	}
	return false, nil
}

// nameBasedHasPermission provides a way for tests to fake the expected outcomes of the
// authentication.
// setting permissionname as the name that user will always have the given permission.
// setting permissionnamemodeltagstring as the name will make that user have the given
// permission only in that model.
func nameBasedHasPermission(name string, operation permission.Access, target names.Tag) bool {
	var perm permission.Access
	switch {
	case strings.HasPrefix(name, string(permission.SuperuserAccess)):
		return operation == permission.SuperuserAccess
	case strings.HasPrefix(name, string(permission.AddModelAccess)):
		return operation == permission.AddModelAccess
	case strings.HasPrefix(name, string(permission.LoginAccess)):
		return operation == permission.LoginAccess
	case strings.HasPrefix(name, string(permission.AdminAccess)):
		perm = permission.AdminAccess
	case strings.HasPrefix(name, string(permission.WriteAccess)):
		perm = permission.WriteAccess
	case strings.HasPrefix(name, string(permission.ConsumeAccess)):
		perm = permission.ConsumeAccess
	case strings.HasPrefix(name, string(permission.ReadAccess)):
		perm = permission.ReadAccess
	default:
		return false
	}
	name = name[len(perm):]
	if len(name) == 0 && perm == permission.AdminAccess {
		return true
	}
	if len(name) == 0 {
		return operation == perm
	}
	if name[0] == '-' {
		name = name[1:]
	}
	targetTag, err := names.ParseTag(name)
	if err != nil {
		return false
	}
	return operation == perm && targetTag.String() == target.String()
}

// ConnectedModel returns the UUID of the model the current client is
// connected to.
func (fa FakeAuthorizer) ConnectedModel() string {
	return fa.ModelUUID
}

// UserHasPermission returns true if the passed user is admin or has a name equal to
// the pre-set admin tag.
func (fa FakeAuthorizer) UserHasPermission(user names.UserTag, operation permission.Access, target names.Tag) (bool, error) {
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
