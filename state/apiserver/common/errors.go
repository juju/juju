// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	stderrors "errors"
	"fmt"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

type notSupportedError struct {
	entity    string
	operation string
}

func (e *notSupportedError) Error() string {
	return fmt.Sprintf("entity %q does not support %s", e.entity, e.operation)
}

func NotSupportedError(entity, operation string) error {
	return &notSupportedError{entity, operation}
}

type noAddressSetError struct {
	unitTag     string
	addressName string
}

func (e *noAddressSetError) Error() string {
	return fmt.Sprintf("%q has no %s address set", e.unitTag, e.addressName)
}

func NoAddressSetError(unitTag, addressName string) error {
	return &noAddressSetError{unitTag, addressName}
}

func IsNoAddressSetError(err error) bool {
	_, ok := err.(*noAddressSetError)
	return ok
}

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
	state.ErrCannotEnterScopeYet: params.CodeCannotEnterScopeYet,
	state.ErrCannotEnterScope:    params.CodeCannotEnterScope,
	state.ErrExcessiveContention: params.CodeExcessiveContention,
	state.ErrUnitHasSubordinates: params.CodeUnitHasSubordinates,
	ErrBadId:                     params.CodeNotFound,
	ErrBadCreds:                  params.CodeUnauthorized,
	ErrPerm:                      params.CodeUnauthorized,
	ErrNotLoggedIn:               params.CodeUnauthorized,
	ErrUnknownWatcher:            params.CodeNotFound,
	ErrStoppedWatcher:            params.CodeStopped,
}

func singletonCode(err error) (string, bool) {
	// All error types may not be hashable; deal with
	// that by catching the panic if we try to look up
	// a non-hashable type.
	defer func() {
		recover()
	}()
	code, ok := singletonErrorCodes[err]
	return code, ok
}

// ServerError returns an error suitable for returning to an API
// client, with an error code suitable for various kinds of errors
// generated in packages outside the API.
func ServerError(err error) *params.Error {
	if err == nil {
		return nil
	}
	code, ok := singletonCode(err)
	switch {
	case ok:
	case errors.IsUnauthorizedError(err):
		code = params.CodeUnauthorized
	case errors.IsNotFoundError(err):
		code = params.CodeNotFound
	case state.IsNotAssigned(err):
		code = params.CodeNotAssigned
	case state.IsHasAssignedUnitsError(err):
		code = params.CodeHasAssignedUnits
	case IsNoAddressSetError(err):
		code = params.CodeNoAddressSet
	case state.IsNotProvisionedError(err):
		code = params.CodeNotProvisioned
	default:
		code = params.ErrCode(err)
	}
	return &params.Error{
		Message: err.Error(),
		Code:    code,
	}
}
