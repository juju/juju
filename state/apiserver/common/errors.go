// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	stderrors "errors"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
)

var (
	ErrBadId          = stderrors.New("id not found")
	ErrBadCreds       = stderrors.New("invalid entity name or password")
	ErrPerm           = stderrors.New("permission denied")
	ErrNotLoggedIn    = stderrors.New("not logged in")
	ErrUnknownWatcher = stderrors.New("unknown watcher id")
	ErrUnknownPinger  = stderrors.New("unknown pinger id")
	ErrStoppedWatcher = stderrors.New("watcher has been stopped")
	ErrBadRequest     = stderrors.New("invalid request")
)

var singletonErrorCodes = map[error]string{
	state.ErrCannotEnterScopeYet: api.CodeCannotEnterScopeYet,
	state.ErrCannotEnterScope:    api.CodeCannotEnterScope,
	state.ErrExcessiveContention: api.CodeExcessiveContention,
	state.ErrUnitHasSubordinates: api.CodeUnitHasSubordinates,
	ErrBadId:                     api.CodeNotFound,
	ErrBadCreds:                  api.CodeUnauthorized,
	ErrPerm:                      api.CodeUnauthorized,
	ErrNotLoggedIn:               api.CodeUnauthorized,
	ErrUnknownWatcher:            api.CodeNotFound,
	ErrStoppedWatcher:            api.CodeStopped,
}

// ServerError returns an error suitable for returning to an API
// client, with an error code suitable for various kinds of errors
// generated in packages outside the API.
func ServerError(err error) *params.Error {
	if err == nil {
		return nil
	}
	code := singletonErrorCodes[err]
	switch {
	case code != "":
	case errors.IsUnauthorizedError(err):
		code = api.CodeUnauthorized
	case errors.IsNotFoundError(err):
		code = api.CodeNotFound
	case state.IsNotAssigned(err):
		code = api.CodeNotAssigned
	case state.IsHasAssignedUnitsError(err):
		code = api.CodeHasAssignedUnits
	default:
		code = api.ErrCode(err)
	}
	return &params.Error{
		Message: err.Error(),
		Code:    code,
	}
}
