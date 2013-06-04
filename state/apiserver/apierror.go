// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	stderrors "errors"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
)

var (
	errBadId          = stderrors.New("id not found")
	errBadVersion     = stderrors.New("API version not available")
	errBadCreds       = stderrors.New("invalid entity name or password")
	errPerm           = stderrors.New("permission denied")
	errNotLoggedIn    = stderrors.New("not logged in")
	errUnknownWatcher = stderrors.New("unknown watcher id")
	errUnknownPinger  = stderrors.New("unknown pinger id")
	errStoppedWatcher = stderrors.New("watcher has been stopped")
)

var singletonErrorCodes = map[error]string{
	state.ErrCannotEnterScopeYet: api.CodeCannotEnterScopeYet,
	state.ErrCannotEnterScope:    api.CodeCannotEnterScope,
	state.ErrExcessiveContention: api.CodeExcessiveContention,
	state.ErrUnitHasSubordinates: api.CodeUnitHasSubordinates,
	errBadId:                     api.CodeNotFound,
	errBadVersion:                api.CodeBadVersion,
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
	case errors.IsUnauthorizedError(err):
		code = api.CodeUnauthorized
	case errors.IsNotFoundError(err):
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

func serverErrorToParams(err error) *params.Error {
	if err != nil {
		err = serverError(err)
		return &params.Error{
			Message: err.Error(),
			Code:    api.ErrCode(err),
		}
	}
	return nil
}
