// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/txn"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/network"
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
	return &noAddressSetError{unitTag: unitTag, addressName: addressName}
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
	Cause          error
	LegacyMacaroon *macaroon.Macaroon
	Macaroon       *bakery.Macaroon
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

// IsUpgradeInProgress returns true if this error is caused
// by an upgrade in progress.
func IsUpgradeInProgressError(err error) bool {
	if state.IsUpgradeInProgressError(err) {
		return true
	}
	return errors.Cause(err) == params.UpgradeInProgressError
}

// RedirectError is the error returned when a model (previously accessible by
// the user) has been migrated to a different controller.
type RedirectError struct {
	// Servers holds the sets of addresses of the redirected servers.
	// TODO (manadart 2019-11-08): Change this to be either MachineHostPorts
	// or the HostPorts indirection. We don't care about space info here.
	// We can then delete the API params helpers for conversion for this type
	// as it will no longer be used.
	Servers []network.ProviderHostPorts `json:"servers"`

	// CACert holds the certificate of the remote server.
	CACert string `json:"ca-cert"`

	// ControllerTag uniquely identifies the controller being redirected to.
	ControllerTag names.ControllerTag `json:"controller-tag,omitempty"`

	// An optional alias for the controller where the model got redirected to.
	ControllerAlias string `json:"controller-alias,omitempty"`
}

// Error implements the error interface.
func (e *RedirectError) Error() string {
	return fmt.Sprintf("redirection to alternative server required")
}

// IsRedirectError returns true if err is caused by a RedirectError.
func IsRedirectError(err error) bool {
	_, ok := errors.Cause(err).(*RedirectError)
	return ok
}

var (
	ErrBadId              = errors.New("id not found")
	ErrBadCreds           = errors.New("invalid entity name or password")
	ErrNoCreds            = errors.New("no credentials provided")
	ErrLoginExpired       = errors.New("login expired")
	ErrPerm               = errors.New("permission denied")
	ErrNotLoggedIn        = errors.New("not logged in")
	ErrUnknownWatcher     = errors.New("unknown watcher id")
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
	ErrNoCreds:                   params.CodeNoCreds,
	ErrLoginExpired:              params.CodeLoginExpired,
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
	case params.CodeNotFound,
		params.CodeUserNotFound,
		params.CodeModelNotFound:
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
	case params.CodeRetry:
		status = http.StatusServiceUnavailable
	case params.CodeRedirect:
		status = http.StatusMovedPermanently
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
	logger.Tracef("server RPC error %v", errors.Details(err))

	var (
		info map[string]interface{}
		msg  = err.Error()
	)

	// Skip past annotations when looking for the code.
	err = errors.Cause(err)
	code, ok := singletonCode(err)
	switch {
	case ok:
	case errors.IsUnauthorized(err):
		code = params.CodeUnauthorized
	case errors.IsNotFound(err):
		code = params.CodeNotFound
	case errors.IsUserNotFound(err):
		code = params.CodeUserNotFound
	case errors.IsAlreadyExists(err):
		code = params.CodeAlreadyExists
	case errors.IsNotAssigned(err):
		code = params.CodeNotAssigned
	case state.IsHasAssignedUnitsError(err):
		code = params.CodeHasAssignedUnits
	case state.IsHasHostedModelsError(err):
		code = params.CodeHasHostedModels
	case state.IsHasPersistentStorageError(err):
		code = params.CodeHasPersistentStorage
	case state.IsModelNotEmptyError(err):
		code = params.CodeModelNotEmpty
	case isNoAddressSetError(err):
		code = params.CodeNoAddressSet
	case errors.IsNotProvisioned(err):
		code = params.CodeNotProvisioned
	case IsUpgradeInProgressError(err):
		code = params.CodeUpgradeInProgress
	case state.IsHasAttachmentsError(err):
		code = params.CodeMachineHasAttachedStorage
	case state.IsStorageAttachedError(err):
		code = params.CodeStorageAttached
	case isUnknownModelError(err):
		code = params.CodeModelNotFound
	case errors.IsNotSupported(err):
		code = params.CodeNotSupported
	case errors.IsBadRequest(err):
		code = params.CodeBadRequest
	case errors.IsMethodNotAllowed(err):
		code = params.CodeMethodNotAllowed
	case errors.IsNotImplemented(err):
		code = params.CodeNotImplemented
	case errors.IsForbidden(err):
		code = params.CodeForbidden
	case state.IsIncompatibleSeriesError(err):
		code = params.CodeIncompatibleSeries
	case IsDischargeRequiredError(err):
		dischErr := errors.Cause(err).(*DischargeRequiredError)
		code = params.CodeDischargeRequired
		info = params.DischargeRequiredErrorInfo{
			Macaroon:       dischErr.LegacyMacaroon,
			BakeryMacaroon: dischErr.Macaroon,
			// One macaroon fits all.
			MacaroonPath: "/",
		}.AsMap()
	case IsRedirectError(err):
		redirErr := errors.Cause(err).(*RedirectError)
		code = params.CodeRedirect

		// Check for a zero-value tag. We don't send it over the wire if it is.
		controllerTag := ""
		if redirErr.ControllerTag.Id() != "" {
			controllerTag = redirErr.ControllerTag.String()
		}

		info = params.RedirectErrorInfo{
			Servers:         params.FromProviderHostsPorts(redirErr.Servers),
			CACert:          redirErr.CACert,
			ControllerTag:   controllerTag,
			ControllerAlias: redirErr.ControllerAlias,
		}.AsMap()
	case errors.IsQuotaLimitExceeded(err):
		code = params.CodeQuotaLimitExceeded
	default:
		code = params.ErrCode(err)
	}

	return &params.Error{
		Message: msg,
		Code:    code,
		Info:    info,
	}
}

func DestroyErr(desc string, ids []string, errs []error) error {
	// TODO(waigani) refactor DestroyErr to take a map of ids to errors.
	if len(errs) == 0 {
		return nil
	}
	msg := "some %s were not destroyed"
	if len(errs) == len(ids) {
		msg = "no %s were destroyed"
	}
	msg = fmt.Sprintf(msg, desc)
	errStrings := make([]string, len(errs))
	for i, err := range errs {
		errStrings[i] = err.Error()
	}
	return errors.Errorf("%s: %s", msg, strings.Join(errStrings, "; "))
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
	case params.IsCodeUserNotFound(err):
		return errors.NewUserNotFound(nil, msg)
	case params.IsCodeAlreadyExists(err):
		return errors.NewAlreadyExists(nil, msg)
	case params.IsCodeNotAssigned(err):
		return errors.NewNotAssigned(nil, msg)
	case params.IsCodeHasAssignedUnits(err):
		// TODO(ericsnow) Handle state.HasAssignedUnitsError here.
		// ...by parsing msg?
		return err
	case params.IsCodeHasHostedModels(err):
		return err
	case params.IsCodeHasPersistentStorage(err):
		return err
	case params.IsCodeModelNotEmpty(err):
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
	case params.IsCodeStorageAttached(err):
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
	case params.IsCodeQuotaLimitExceeded(err):
		return errors.NewQuotaLimitExceeded(nil, msg)
	default:
		return err
	}
}
