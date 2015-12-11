// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/rpcreflect"
	"github.com/juju/juju/state"
)

var ErrUserReadOnly = errors.New("user has only read only access")

// clientAuthRoot restricts API calls for users of an environment. Initially
// the authorisation checks are only for read only access to the environment,
// but in the near future, full ACL support is desirable.
type clientAuthRoot struct {
	rpc.MethodFinder
	user *state.EnvironmentUser
}

// newClientAuthRoot returns a new restrictedRoot.
func newClientAuthRoot(finder rpc.MethodFinder, user *state.EnvironmentUser) *newClientAuthRoot {
	return &newClientAuthRoot{finder, user}
}

// FindMethod returns a not supported error if the rootName is not one
// of the facades available at the server root when there is no active
// environment.
func (r *clientAuthRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	// The lookup of the name is done first to return a not found error if the
	// user is looking for a method that we just don't have.
	caller, err := r.MethodFinder.FindMethod(rootName, version, methodName)
	if err != nil {
		return nil, err
	}
	if r.user.ReadOnly() {
		// check the white list
		writeOperation := true // go look
		// if there
		if writeOperation {
			return nil, errors.Trace(ErrUserReadOnly)
		}
	}

	return caller, nil
}
