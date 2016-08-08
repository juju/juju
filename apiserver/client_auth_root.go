// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/rpcreflect"
)

// clientAuthRoot restricts API calls for users of a model. Initially the
// authorisation checks are only for read only access to the model, but in the
// near future, full ACL support is desirable.
type clientAuthRoot struct {
	finder rpc.MethodFinder
	user   description.UserAccess
}

// newClientAuthRoot returns a new restrictedRoot.
func newClientAuthRoot(finder rpc.MethodFinder, user description.UserAccess) *clientAuthRoot {
	return &clientAuthRoot{finder, user}
}

// FindMethod returns a not supported error if the rootName is not one of the
// facades available at the server root when there is no active model.
func (r *clientAuthRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	// The lookup of the name is done first to return a not found error if the
	// user is looking for a method that we just don't have.
	caller, err := r.finder.FindMethod(rootName, version, methodName)
	if err != nil {
		return nil, err
	}
	// ReadOnly User
	if r.user.Access == description.ReadAccess {
		canCall := isCallAllowableByReadOnlyUser(rootName, methodName) ||
			isCallReadOnly(rootName, methodName)
		if !canCall {
			return nil, errors.Trace(common.ErrPerm)
		}
	}

	// Check if our call requires higher access than the user has.
	if doesCallRequireAdmin(rootName, methodName) && r.user.Access != description.AdminAccess {
		return nil, errors.Trace(common.ErrPerm)
	}

	return caller, nil
}

// isCallAllowableByReadOnlyUser returns whether or not the method on the facade
// can be called by a read only user.
func isCallAllowableByReadOnlyUser(facade, _ /*method*/ string) bool {
	// At this stage, any facade that is part of the restricted root (those
	// that are accessable outside of models) are OK because the user would
	// have access to those facades if they went through the controller API
	// endpoint rather than a model oriented one.
	return restrictedRootNames.Contains(facade)
}

var modelManagerMethods = set.NewStrings(
	"ModifyModelAccess",
	"CreateModel",
)

var controllerMethods = set.NewStrings(
	"DestroyController",
)

func doesCallRequireAdmin(facade, method string) bool {
	// TODO(perrito666) This should filter adding users to controllers.
	// TODO(perrito666) Add an exaustive list of facades/methods that are
	// admin only and put them in an authoritative source to be re-used.
	// TODO(perrito666) This is a stub, the idea is to maintain the current
	// status of permissions until we decide what goes to admin only.
	switch facade {
	case "ModelManager":
		return modelManagerMethods.Contains(method)
	case "Controller":
		return controllerMethods.Contains(method)
	}
	return false
}
