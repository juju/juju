// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/txn"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/state"
)

func NotSupportedError(tag names.Tag, operation string) error {
	return errors.Errorf("entity %q does not support %s", tag, operation)
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

func isNoAddressSetError(err error) bool {
	_, ok := err.(*noAddressSetError)
	return ok
}

type unknownModelError struct {
	uuid string
}

func (e *unknownModelError) Error() string {
	return fmt.Sprintf("unknown model: %q", e.uuid)
}

func UnknownModelError(uuid string) error {
	return &unknownModelError{uuid: uuid}
}

func isUnknownModelError(err error) bool {
	_, ok := err.(*unknownModelError)
	return ok
}

// DischargeRequiredError is the error returned when a macaroon requires discharging
// to complete authentication.
type DischargeRequiredError struct {
	Cause    error
	Macaroon *macaroon.Macaroon
}

// Error implements the error interface.
func (e *DischargeRequiredError) Error() string {
	return e.Cause.Error()
}

// IsDischargeRequiredError reports whether the cause
// of the error is a *DischargeRequiredError.
func IsDischargeRequiredError(err error) bool {
	_, ok := errors.Cause(err).(*DischargeRequiredError)
	return ok
}

var (
	ErrBadId              = errors.New("id not found")
	ErrBadCreds           = errors.New("invalid entity name or password")
	ErrPerm               = errors.New("permission denied")
	ErrNotLoggedIn        = errors.New("not logged in")
	ErrUnknownWatcher     = errors.New("unknown watcher id")
	ErrUnknownPinger      = errors.New("unknown pinger id")
	ErrStoppedWatcher     = errors.New("watcher has been stopped")
	ErrBadRequest         = errors.New("invalid request")
	ErrTryAgain           = errors.New("try again")
	ErrActionNotAvailable = errors.New("action no longer available")
)

// OperationBlockedError returns an error which signifies that
// an operation has been blocked; the message should describe
// what has been blocked.
func OperationBlockedError(msg string) error {
	if msg == "" {
		msg = "the operation has been blocked"
	}
	return &params.Error{
		Message: msg,
		Code:    params.CodeOperationBlocked,
	}
}

var singletonErrorCodes = map[error]string{
	state.ErrCannotEnterScopeYet: params.CodeCannotEnterScopeYet,
	state.ErrCannotEnterScope:    params.CodeCannotEnterScope,
	state.ErrUnitHasSubordinates: params.CodeUnitHasSubordinates,
	state.ErrDead:                params.CodeDead,
	txn.ErrExcessiveContention:   params.CodeExcessiveContention,
	leadership.ErrClaimDenied:    params.CodeLeadershipClaimDenied,
	lease.ErrClaimDenied:         params.CodeLeaseClaimDenied,
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

func singletonError(err error) (error, bool) {
	errCode := params.ErrCode(err)
	for singleton, code := range singletonErrorCodes {
		if errCode == code && singleton.Error() == err.Error() {
			return singleton, true
		}
	}
	return nil, false
}

// ServerErrorAndStatus is like ServerError but also
// returns an HTTP status code appropriate for using
// in a response holding the given error.
func ServerErrorAndStatus(err error) (*params.Error, int) {
	err1 := ServerError(err)
	if err1 == nil {
		return nil, http.StatusOK
	}
	status := http.StatusInternalServerError
	switch err1.Code {
	case params.CodeUnauthorized:
		status = http.StatusUnauthorized
	case params.CodeNotFound:
		status = http.StatusNotFound
	case params.CodeBadRequest:
		status = http.StatusBadRequest
	case params.CodeMethodNotAllowed:
		status = http.StatusMethodNotAllowed
	case params.CodeOperationBlocked:
		// This should really be http.StatusForbidden but earlier versions
		// of juju clients rely on the 400 status, so we leave it like that.
		status = http.StatusBadRequest
	case params.CodeForbidden:
		status = http.StatusForbidden
	case params.CodeDischargeRequired:
		status = http.StatusUnauthorized
	}
	return err1, status
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
	var info *params.ErrorInfo
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
	case isNoAddressSetError(err):
		code = params.CodeNoAddressSet
	case errors.IsNotProvisioned(err):
		code = params.CodeNotProvisioned
	case state.IsUpgradeInProgressError(err):
		code = params.CodeUpgradeInProgress
	case state.IsHasAttachmentsError(err):
		code = params.CodeMachineHasAttachedStorage
	case isUnknownModelError(err):
		code = params.CodeNotFound
	case errors.IsNotSupported(err):
		code = params.CodeNotSupported
	case errors.IsBadRequest(err):
		code = params.CodeBadRequest
	case errors.IsMethodNotAllowed(err):
		code = params.CodeMethodNotAllowed
	default:
		if err, ok := err.(*DischargeRequiredError); ok {
			code = params.CodeDischargeRequired
			info = &params.ErrorInfo{
				Macaroon: err.Macaroon,
				// One macaroon fits all.
				MacaroonPath: "/",
			}
			break
		}
		code = params.ErrCode(err)
	}
	return &params.Error{
		Message: msg,
		Code:    code,
		Info:    info,
	}
}

func DestroyErr(desc string, ids, errs []string) error {
	// TODO(waigani) refactor DestroyErr to take a map of ids to errors.
	if len(errs) == 0 {
		return nil
	}
	msg := "some %s were not destroyed"
	if len(errs) == len(ids) {
		msg = "no %s were destroyed"
	}
	msg = fmt.Sprintf(msg, desc)
	return errors.Errorf("%s: %s", msg, strings.Join(errs, "; "))
}

// RestoreError makes a best effort at converting the given error
// back into an error originally converted by ServerError().
func RestoreError(err error) error {
	err = errors.Cause(err)

	if apiErr, ok := err.(*params.Error); !ok {
		return err
	} else if apiErr == nil {
		return nil
	}
	if params.ErrCode(err) == "" {
		return err
	}
	msg := err.Error()

	if singleton, ok := singletonError(err); ok {
		return singleton
	}

	// TODO(ericsnow) Support the other error types handled by ServerError().
	switch {
	case params.IsCodeUnauthorized(err):
		return errors.NewUnauthorized(nil, msg)
	case params.IsCodeNotFound(err):
		// TODO(ericsnow) UnknownModelError should be handled here too.
		// ...by parsing msg?
		return errors.NewNotFound(nil, msg)
	case params.IsCodeAlreadyExists(err):
		return errors.NewAlreadyExists(nil, msg)
	case params.IsCodeNotAssigned(err):
		return errors.NewNotAssigned(nil, msg)
	case params.IsCodeHasAssignedUnits(err):
		// TODO(ericsnow) Handle state.HasAssignedUnitsError here.
		// ...by parsing msg?
		return err
	case params.IsCodeNoAddressSet(err):
		// TODO(ericsnow) Handle isNoAddressSetError here.
		// ...by parsing msg?
		return err
	case params.IsCodeNotProvisioned(err):
		return errors.NewNotProvisioned(nil, msg)
	case params.IsCodeUpgradeInProgress(err):
		// TODO(ericsnow) Handle state.UpgradeInProgressError here.
		// ...by parsing msg?
		return err
	case params.IsCodeMachineHasAttachedStorage(err):
		// TODO(ericsnow) Handle state.HasAttachmentsError here.
		// ...by parsing msg?
		return err
	case params.IsCodeNotSupported(err):
		return errors.NewNotSupported(nil, msg)
	case params.IsBadRequest(err):
		return errors.NewBadRequest(nil, msg)
	case params.IsMethodNotAllowed(err):
		return errors.NewMethodNotAllowed(nil, msg)
	case params.ErrCode(err) == params.CodeDischargeRequired:
		// TODO(ericsnow) Handle DischargeRequiredError here.
		return err
	default:
		return err
	}
}
