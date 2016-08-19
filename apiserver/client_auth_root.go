// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/rpcreflect"
)

// clientAuthRoot restricts API calls for users of a model. Initially the
// authorisation checks are only for read only access to the model, but in the
// near future, full ACL support is desirable.
type clientAuthRoot struct {
	rpc.Root
	modelUser      description.UserAccess
	controllerUser description.UserAccess
}

// newClientAuthRoot returns a new API root that
// restricts RPC calls to those appropriate for
// the given user access.
//
// This is not appropriate for use on controller-only API connections.
func newClientAuthRoot(root rpc.Root, modelUser description.UserAccess, controllerUser description.UserAccess) *clientAuthRoot {
	return &clientAuthRoot{root, modelUser, controllerUser}
}

// FindMethod implements rpc.Root.FindMethod.
// It returns a permission-denied error if the call is not appropriate
// for the user access rights.
func (r *clientAuthRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	// The lookup of the name is done first to return a
	// rpcreflect.CallNotImplementedError error if the
	// user is looking for a method that we just don't have.
	return r.Root.FindMethod(rootName, version, methodName)
}
