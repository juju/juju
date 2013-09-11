// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"launchpad.net/juju-core/state/api/params"
)

// This module implements a subset of the interface provided by
// state.Environment, as needed by the uniter API.

// Environment represents the state of an environment.
type Environment struct {
	st *State
}

// UUID returns the universally unique identifier of the environment.
//
// NOTE: This differs from state.Environment.UUID() by returning an
// error as well, because it needs to make an API call
//
// TODO(dimitern): 2013-09-06 bug 1221834
// Cache this after getting it once - it's immutable.
func (e Environment) UUID() (string, error) {
	var result params.StringResult
	err := e.st.caller.Call("Uniter", "", "CurrentEnvironUUID", nil, &result)
	if err != nil {
		return "", err
	}
	return result.Result, nil
}
