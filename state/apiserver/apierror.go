// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"errors"
	jujuerrors "launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
)

var (
	errBadId          = errors.New("id not found")
	errBadCreds       = errors.New("invalid entity name or password")
	errPerm           = errors.New("permission denied")
	errNotLoggedIn    = errors.New("not logged in")
	errUnknownWatcher = errors.New("unknown watcher id")
	errUnknownPinger  = errors.New("unknown pinger id")
	errStoppedWatcher = errors.New("watcher has been stopped")
)

var singletonErrorCodes = map[error]string{
	state.ErrCannotEnterScopeYet: api.CodeCannotEnterScopeYet,
	state.ErrCannotEnterScope:    api.CodeCannotEnterScope,
	state.ErrExcessiveContention: api.CodeExcessiveContention,
	state.ErrUnitHasSubordinates: api.CodeUnitHasSubordinates,
	errBadId:                     api.CodeNotFound,
	errBadCreds:                  api.CodeUnauthorized,
	errPerm:                      api.CodeUnauthorized,
	errNotLoggedIn:               api.CodeUnauthorized,
	errUnknownWatcher:            api.CodeNotFound,
	errStoppedWatcher:            api.CodeStopped,
}

func serverError(err error) error {
	code := singletonErrorCodes[err]
	switch {
	case code != "":
	case jujuerrors.IsUnauthorizedError(err):
		code = api.CodeUnauthorized
	case jujuerrors.IsNotFoundError(err):
		code = api.CodeNotFound
	case state.IsNotAssigned(err):
		code = api.CodeNotAssigned
	case state.IsHasAssignedUnitsError(err):
		code = api.CodeHasAssignedUnits
	}
	if code != "" {
		return &api.Error{
			Message: err.Error(),
			Code:    code,
		}
	}
	return err
}
