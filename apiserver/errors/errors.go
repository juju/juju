// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/txn/v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
	stateerrors "github.com/juju/juju/state/errors"
)

var logger = loggo.GetLogger("juju.apiserver.common.errors")

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
	stateerrors.ErrCannotEnterScopeYet: params.CodeCannotEnterScopeYet,
	stateerrors.ErrCannotEnterScope:    params.CodeCannotEnterScope,
	stateerrors.ErrUnitHasSubordinates: params.CodeUnitHasSubordinates,
	stateerrors.ErrDead:                params.CodeDead,
	txn.ErrExcessiveContention:         params.CodeExcessiveContention,
	leadership.ErrClaimDenied:          params.CodeLeadershipClaimDenied,
	lease.ErrClaimDenied:               params.CodeLeaseClaimDenied,
	ErrBadId:                           params.CodeNotFound,
	ErrBadCreds:                        params.CodeUnauthorized,
	ErrNoCreds:                         params.CodeNoCreds,
	ErrLoginExpired:                    params.CodeLoginExpired,
	ErrPerm:                            params.CodeUnauthorized,
	ErrNotLoggedIn:                     params.CodeUnauthorized,
	ErrUnknownWatcher:                  params.CodeNotFound,
	ErrStoppedWatcher:                  params.CodeStopped,
	ErrTryAgain:                        params.CodeTryAgain,
	ErrActionNotAvailable:              params.CodeActionNotAvailable,
}

func singletonCode(err error) (string, bool) {
	// All error types may not be hashable; deal with
	// that by catching the panic if we try to look up
	// a non-hashable type.
	defer func() {
		_ = recover()
	}()
	code, ok := singletonErrorCodes[err]
	return code, ok
}

func singletonError(err error) (bool, error) {
	errCode := params.ErrCode(err)
	for singleton, code := range singletonErrorCodes {
		if errCode == code && singleton.Error() == err.Error() {
			return true, singleton
		}
	}
	return false, nil
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
	case params.CodeRedirect:
		status = http.StatusMovedPermanently
	case params.CodeNotLeader:
		status = http.StatusTemporaryRedirect
	case params.CodeLeaseError:
		status = leaseStatusCode(err1)
	case params.CodeNotYetAvailable:
		// The request could not be completed due to a conflict with
		// the current state of the resource. This code is only allowed
		// in situations where it is expected that the user might be
		// able to resolve the conflict and resubmit the request.
		//
		// See https://www.w3.org/Protocols/rfc2616/rfc2616-sec10.html#sec10.4.10
		status = http.StatusConflict
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
	if logger.IsTraceEnabled() {
		logger.Tracef("server RPC error %v", errors.Details(err))
	}

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
	case stateerrors.IsHasAssignedUnitsError(err):
		code = params.CodeHasAssignedUnits
	case stateerrors.IsHasHostedModelsError(err):
		code = params.CodeHasHostedModels
	case stateerrors.IsHasPersistentStorageError(err):
		code = params.CodeHasPersistentStorage
	case stateerrors.IsModelNotEmptyError(err):
		code = params.CodeModelNotEmpty
	case isNoAddressSetError(err):
		code = params.CodeNoAddressSet
	case errors.IsNotProvisioned(err):
		code = params.CodeNotProvisioned
	case IsUpgradeInProgressError(err):
		code = params.CodeUpgradeInProgress
	case stateerrors.IsHasAttachmentsError(err):
		code = params.CodeMachineHasAttachedStorage
	case stateerrors.IsHasContainersError(err):
		code = params.CodeMachineHasContainers
	case stateerrors.IsStorageAttachedError(err):
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
	case IsIncompatibleSeriesError(err), stateerrors.IsIncompatibleSeriesError(err):
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
	case IsUpgradeSeriesValidationError(err):
		rawErr := errors.Cause(err).(*UpgradeSeriesValidationError)
		info = params.UpgradeSeriesValidationErrorInfo{
			Status: rawErr.Status,
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
	case errors.IsNotYetAvailable(err):
		code = params.CodeNotYetAvailable
	case params.IsIncompatibleClientError(err):
		code = params.CodeIncompatibleClient
		rawErr := errors.Cause(err).(*params.IncompatibleClientError)
		info = rawErr.AsMap()
	case IsNotLeaderError(err):
		code = params.CodeNotLeader
		rawErr := errors.Cause(err).(*NotLeaderError)
		info = rawErr.AsMap()
	case IsDeadlineExceededError(err):
		code = params.CodeDeadlineExceeded
	case lease.IsLeaseError(err):
		code = params.CodeLeaseError
		info = leaseErrorInfoMap(err)
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

	if ok, singleton := singletonError(err); ok {
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
		// TODO(ericsnow) Handle stateerrors.HasAssignedUnitsError here.
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
		// TODO(ericsnow) Handle stateerrors.UpgradeInProgressError here.
		// ...by parsing msg?
		return err
	case params.IsCodeMachineHasAttachedStorage(err):
		// TODO(ericsnow) Handle stateerrors.HasAttachmentsError here.
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
	case params.IsCodeNotLeader(err):
		e, ok := err.(*params.Error)
		if !ok {
			return err
		}
		serverAddress, _ := e.Info["server-address"].(string)
		serverID, _ := e.Info["server-id"].(string)
		return NewNotLeaderError(serverAddress, serverID)
	case params.IsCodeDeadlineExceeded(err):
		return NewDeadlineExceededError(msg)
	case params.IsLeaseError(err):
		return rehydrateLeaseError(err)
	case params.IsCodeNotYetAvailable(err):
		return errors.NewNotYetAvailable(nil, msg)
	default:
		return err
	}
}
