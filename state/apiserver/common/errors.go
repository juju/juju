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
	ErrBadVersion     = stderrors.New("API version not supported")
	ErrBadCreds       = stderrors.New("invalid entity name or password")
	ErrPerm           = stderrors.New("permission denied")
	ErrNotLoggedIn    = stderrors.New("not logged in")
	ErrUnknownWatcher = stderrors.New("unknown watcher id")
	ErrUnknownPinger  = stderrors.New("unknown pinger id")
	ErrStoppedWatcher = stderrors.New("watcher has been stopped")
)

var singletonErrorCodes = map[error]string{
	state.ErrCannotEnterScopeYet: api.CodeCannotEnterScopeYet,
	state.ErrCannotEnterScope:    api.CodeCannotEnterScope,
	state.ErrExcessiveContention: api.CodeExcessiveContention,
	state.ErrUnitHasSubordinates: api.CodeUnitHasSubordinates,
	ErrBadId:                     api.CodeNotFound,
	ErrBadVersion:                api.CodeBadVersion,
	ErrBadCreds:                  api.CodeUnauthorized,
	ErrPerm:                      api.CodeUnauthorized,
	ErrNotLoggedIn:               api.CodeUnauthorized,
	ErrUnknownWatcher:            api.CodeNotFound,
	ErrStoppedWatcher:            api.CodeStopped,
}

func ServerError(err error) error {
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

func ServerErrorToParams(err error) *params.Error {
	if err != nil {
		err = ServerError(err)
		return &params.Error{
			Message: err.Error(),
			Code:    api.ErrCode(err),
		}
	}
	return nil
}
