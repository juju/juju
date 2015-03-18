// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/rpcreflect"
)

// restrictedRoot restricts API calls to the environment manager and
// user manager when accessed through the root path on the API server.
type restrictedRoot struct {
	rpc.MethodFinder
}

// newRestrictedRoot returns a new restrictedRoot.
func newRestrictedRoot(finder rpc.MethodFinder) *restrictedRoot {
	return &restrictedRoot{finder}
}

var restrictedRootNames = set.NewStrings(
	"EnvironmentManager",
	"UserManager",
)

// FindMethod returns a not supported error if the rootName is not one
// of the facades available at the server root when there is no active
// environment.
func (r *restrictedRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	if !restrictedRootNames.Contains(rootName) {
		return nil, errors.NotSupportedf("logged in to server, no environment, %q", rootName)
	}
	return r.MethodFinder.FindMethod(rootName, version, methodName)
}
