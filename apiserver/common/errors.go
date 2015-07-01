// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	stderrors "errors"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/txn"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/leadership"
	"github.com/juju/juju/state"
)

type notSupportedError struct {
	tag       names.Tag
	operation string
}

func (e *notSupportedError) Error() string {
	return fmt.Sprintf("entity %q does not support %s", e.tag, e.operation)
}

func NotSupportedError(tag names.Tag, operation string) error {
	return &notSupportedError{tag, operation}
}

type noAddressSetError struct {
	unitTag     names.UnitTag
	addressName string
}

func (e *noAddressSetError) Error() string {
	return fmt.Sprintf("%q has no %s address set", e.unitTag, e.addressName)
}

func NoAddressSetError(unitTag names.UnitTag, addressName string) error {
	return &noAddressSetError{unitTag, addressName}
}

func IsNoAddressSetError(err error) bool {
	_, ok := err.(*noAddressSetError)
	return ok
}

type unknownEnvironmentError struct {
	uuid string
}

func (e *unknownEnvironmentError) Error() string {
	return fmt.Sprintf("unknown environment: %q", e.uuid)
}

func UnknownEnvironmentError(uuid string) error {
	return &unknownEnvironmentError{uuid: uuid}
}

func IsUnknownEnviromentError(err error) bool {
	_, ok := err.(*unknownEnvironmentError)
	return ok
}

var (
	ErrBadId              = stderrors.New("id not found")
	ErrBadCreds           = stderrors.New("invalid entity name or password")
	ErrPerm               = stderrors.New("permission denied")
	ErrNotLoggedIn        = stderrors.New("not logged in")
	ErrUnknownWatcher     = stderrors.New("unknown watcher id")
	ErrUnknownPinger      = stderrors.New("unknown pinger id")
	ErrStoppedWatcher     = stderrors.New("watcher has been stopped")
	ErrBadRequest         = stderrors.New("invalid request")
	ErrTryAgain           = stderrors.New("try again")
	ErrActionNotAvailable = stderrors.New("action no longer available")

	ErrOperationBlocked = func(msg string) *params.Error {
		if msg == "" {
			msg = "The operation has been blocked."
		}
		return &params.Error{
			Code:    params.CodeOperationBlocked,
			Message: msg,
		}
	}
)

var singletonErrorCodes = map[error]string{
	state.ErrCannotEnterScopeYet: params.CodeCannotEnterScopeYet,
	state.ErrCannotEnterScope:    params.CodeCannotEnterScope,
	state.ErrUnitHasSubordinates: params.CodeUnitHasSubordinates,
	state.ErrDead:                params.CodeDead,
	txn.ErrExcessiveContention:   params.CodeExcessiveContention,
	leadership.ErrClaimDenied:    params.CodeLeadershipClaimDenied,
	ErrBadId:                     params.CodeNotFound,
	ErrBadCreds:                  params.CodeUnauthorized,
	ErrPerm:                      params.CodeUnauthorized,
	ErrNotLoggedIn:               params.CodeUnauthorized,
	ErrUnknownWatcher:            params.CodeNotFound,
	ErrStoppedWatcher:            params.CodeStopped,
	ErrTryAgain:                  params.CodeTryAgain,
	ErrActionNotAvailable:        params.CodeActionNotAvailable,
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
	msg := err.Error()
	// Skip past annotations when looking for the code.
	err = errors.Cause(err)
	code, ok := singletonCode(err)
	switch {
	case ok:
	case errors.IsUnauthorized(err):
		code = params.CodeUnauthorized
	case errors.IsNotFound(err):
		code = params.CodeNotFound
	case errors.IsAlreadyExists(err):
		code = params.CodeAlreadyExists
	case errors.IsNotAssigned(err):
		code = params.CodeNotAssigned
	case state.IsHasAssignedUnitsError(err):
		code = params.CodeHasAssignedUnits
	case IsNoAddressSetError(err):
		code = params.CodeNoAddressSet
	case errors.IsNotProvisioned(err):
		code = params.CodeNotProvisioned
	case state.IsUpgradeInProgressError(err):
		code = params.CodeUpgradeInProgress
	case state.IsHasAttachmentsError(err):
		code = params.CodeMachineHasAttachedStorage
	case IsUnknownEnviromentError(err):
		code = params.CodeNotFound
	default:
		code = params.ErrCode(err)
	}
	return &params.Error{
		Message: msg,
		Code:    code,
	}
}
