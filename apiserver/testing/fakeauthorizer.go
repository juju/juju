// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/permission"
)

// FakeAuthorizerOption defines a function that modifies the fake authorizer
type FakeAuthorizerOption func(*FakeAuthorizer)

// SetTagWithWriteAccess returns a FakeAuthorizerOption that modifies the fake
// authorizer so that it will grant read access to the specified user.
func SetTagWithReadAccess(ut names.UserTag) FakeAuthorizerOption {
	return func(a *FakeAuthorizer) {
		a.Tag = ut
		a.HasReadTag = ut
	}
}

// SetTagWithWriteAccess returns a FakeAuthorizerOption that modifies the fake
// authorizer so that it will grant read and write access to the specified user.
func SetTagWithWriteAccess(ut names.UserTag) FakeAuthorizerOption {
	return func(a *FakeAuthorizer) {
		a.Tag = ut
		a.HasWriteTag = ut
	}
}

// SetTagWithAdminAccess returns a FakeAuthorizerOption that modifies the fake
// authorizer so that it will grant admin access to the specified user.
func SetTagWithAdminAccess(ut names.UserTag) FakeAuthorizerOption {
	return func(a *FakeAuthorizer) {
		a.Tag = ut
		a.AdminTag = ut
	}
}

// SetTagWithConsumeAccess returns a FakeAuthorizerOption that modifies the fake
// authorizer so that it will grant consume access to the specified user.
func SetTagWithConsumeAccess(ut names.UserTag) FakeAuthorizerOption {
	return func(a *FakeAuthorizer) {
		a.Tag = ut
		a.HasConsumeTag = ut
	}
}

// FakeAuthorizer implements the facade.Authorizer interface.
type FakeAuthorizer struct {
	Tag           names.Tag
	Controller    bool
	ModelUUID     string
	AdminTag      names.UserTag
	HasConsumeTag names.UserTag
	HasWriteTag   names.UserTag
	HasReadTag    names.UserTag
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

// AuthModelAgent returns true if the authenticated entity is a model agent
func (fa FakeAuthorizer) AuthModelAgent() bool {
	_, isModel := fa.GetAuthTag().(names.ModelTag)
	return isModel
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
func (fa FakeAuthorizer) HasPermission(operation permission.Access, target names.Tag) error {
	if fa.Tag.Kind() == names.UserTagKind {
		ut := fa.Tag.(names.UserTag)
		emptyTag := names.UserTag{}
		if fa.AdminTag != emptyTag && ut == fa.AdminTag {
			return nil
		}
		if fa.HasWriteTag != emptyTag && ut == fa.HasWriteTag && (operation == permission.WriteAccess || operation == permission.ReadAccess) {
			return nil
		}

		if fa.HasReadTag != emptyTag && ut == fa.HasReadTag && operation == permission.ReadAccess {
			return nil
		}

		uTag := fa.Tag.(names.UserTag)
		var err error
		res := nameBasedHasPermission(uTag.Name(), operation, target)
		if !res {
			err = errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission)
		}
		return err
	}
	return errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission)
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

// EntityHasPermission returns true if the passed entity is admin or has a name equal to
// the pre-set admin tag.
func (fa FakeAuthorizer) EntityHasPermission(entity names.Tag, operation permission.Access, _ names.Tag) error {
	if entity.Kind() == names.UserTagKind && entity.Id() == "admin" {
		return nil
	}
	emptyTag := names.UserTag{}
	if fa.AdminTag != emptyTag && entity == fa.AdminTag {
		return nil
	}
	if operation == permission.ConsumeAccess && fa.HasConsumeTag != emptyTag && entity == fa.HasConsumeTag {
		return nil
	}
	return errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission)
}
