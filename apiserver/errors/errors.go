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

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/rpc/params"
	stateerrors "github.com/juju/juju/state/errors"
)

var logger = loggo.GetLogger("juju.apiserver.common.errors")

const (
	ErrBadId              = errors.ConstError("id not found")
	ErrBadCreds           = errors.ConstError("invalid entity name or password")
	ErrNoCreds            = errors.ConstError("no credentials provided")
	ErrLoginExpired       = errors.ConstError("login expired")
	ErrPerm               = errors.ConstError("permission denied")
	ErrNotLoggedIn        = errors.ConstError("not logged in")
	ErrUnknownWatcher     = errors.ConstError("unknown watcher id")
	ErrStoppedWatcher     = errors.ConstError("watcher has been stopped")
	ErrBadRequest         = errors.ConstError("invalid request")
	ErrTryAgain           = errors.ConstError("try again")
	ErrActionNotAvailable = errors.ConstError("action no longer available")
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
	case params.CodeNotYetAvailable:
		// The request could not be completed due to a conflict with
		// the current state of the resource. This code is only allowed
		// in situations where it is expected that the user might be
		// able to resolve the conflict and resubmit the request.
		//
		// See https://www.w3.org/Protocols/rfc2616/rfc2616-sec10.html#sec10.4.10
		status = http.StatusConflict
	case params.CodeNotLeader:
		status = http.StatusTemporaryRedirect
	case params.CodeLeaseError:
		status = leaseStatusCode(err1)
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

	var (
		dischargeRequiredError       *DischargeRequiredError
		incompatibleClientError      *params.IncompatibleClientError
		notLeaderError               *NotLeaderError
		redirectError                *RedirectError
		upgradeSeriesValidationError *UpgradeSeriesValidationError
	)
	// Skip past annotations when looking for the code.
	err = errors.Cause(err)
	code, ok := singletonCode(err)
	switch {
	case ok:
	case errors.Is(err, errors.Unauthorized):
		code = params.CodeUnauthorized
	case errors.Is(err, errors.NotFound):
		code = params.CodeNotFound
	case errors.Is(err, errors.UserNotFound):
		code = params.CodeUserNotFound
	case errors.Is(err, errors.AlreadyExists):
		code = params.CodeAlreadyExists
	case errors.Is(err, errors.NotAssigned):
		code = params.CodeNotAssigned
	case errors.Is(err, stateerrors.HasAssignedUnitsError):
		code = params.CodeHasAssignedUnits
	case errors.Is(err, stateerrors.HasHostedModelsError):
		code = params.CodeHasHostedModels
	case errors.Is(err, stateerrors.PersistentStorageError):
		code = params.CodeHasPersistentStorage
	case errors.Is(err, stateerrors.ModelNotEmptyError):
		code = params.CodeModelNotEmpty
	case errors.Is(err, NoAddressSetError):
		code = params.CodeNoAddressSet
	case errors.Is(err, errors.NotProvisioned):
		code = params.CodeNotProvisioned
	case errors.Is(err, params.UpgradeInProgressError),
		errors.Is(err, stateerrors.ErrUpgradeInProgress):
		code = params.CodeUpgradeInProgress
	case errors.Is(err, stateerrors.HasAttachmentsError):
		code = params.CodeMachineHasAttachedStorage
	case errors.Is(err, stateerrors.HasContainersError):
		code = params.CodeMachineHasContainers
	case errors.Is(err, stateerrors.StorageAttachedError):
		code = params.CodeStorageAttached
	case errors.Is(err, UnknownModelError):
		code = params.CodeModelNotFound
	case errors.Is(err, errors.NotSupported):
		code = params.CodeNotSupported
	case errors.Is(err, errors.BadRequest):
		code = params.CodeBadRequest
	case errors.Is(err, errors.MethodNotAllowed):
		code = params.CodeMethodNotAllowed
	case errors.Is(err, errors.NotImplemented):
		code = params.CodeNotImplemented
	case errors.Is(err, errors.Forbidden):
		code = params.CodeForbidden
	case errors.Is(err, errors.NotValid):
		code = params.CodeNotValid
	case errors.Is(err, IncompatibleSeriesError), errors.Is(err, stateerrors.IncompatibleSeriesError):
		code = params.CodeIncompatibleSeries
	case errors.As(err, &dischargeRequiredError):
		code = params.CodeDischargeRequired
		info = params.DischargeRequiredErrorInfo{
			Macaroon:       dischargeRequiredError.LegacyMacaroon,
			BakeryMacaroon: dischargeRequiredError.Macaroon,
			// One macaroon fits all.
			MacaroonPath: "/",
		}.AsMap()
	case errors.As(err, &upgradeSeriesValidationError):
		info = params.UpgradeSeriesValidationErrorInfo{
			Status: upgradeSeriesValidationError.Status,
		}.AsMap()
	case errors.As(err, &redirectError):
		code = params.CodeRedirect

		// Check for a zero-value tag. We don't send it over the wire if it is.
		controllerTag := ""
		if redirectError.ControllerTag.Id() != "" {
			controllerTag = redirectError.ControllerTag.String()
		}

		info = params.RedirectErrorInfo{
			Servers:         params.FromProviderHostsPorts(redirectError.Servers),
			CACert:          redirectError.CACert,
			ControllerTag:   controllerTag,
			ControllerAlias: redirectError.ControllerAlias,
		}.AsMap()
	case errors.Is(err, errors.QuotaLimitExceeded):
		code = params.CodeQuotaLimitExceeded
	case errors.Is(err, errors.NotYetAvailable):
		code = params.CodeNotYetAvailable
	case errors.Is(err, ErrTryAgain):
		code = params.CodeTryAgain
	case errors.As(err, &incompatibleClientError):
		code = params.CodeIncompatibleClient
		info = incompatibleClientError.AsMap()
	case errors.As(err, &notLeaderError):
		code = params.CodeNotLeader
		info = notLeaderError.AsMap()
	case errors.Is(err, DeadlineExceededError):
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
	if err == nil {
		return nil
	}
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
		return errors.NewNotFound(nil, msg)
	case params.IsCodeModelNotFound(err):
		return fmt.Errorf("%s%w", msg, errors.Hide(UnknownModelError))
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
	case params.IsCodeNotYetAvailable(err):
		return errors.NewNotYetAvailable(nil, msg)
	case params.IsCodeNotLeader(err):
		e, ok := err.(*params.Error)
		if !ok {
			return err
		}
		serverAddress, _ := e.Info["server-address"].(string)
		serverID, _ := e.Info["server-id"].(string)
		return NewNotLeaderError(serverAddress, serverID)
	case params.IsCodeDeadlineExceeded(err):
		return fmt.Errorf(msg+"%w", errors.Hide(DeadlineExceededError))
	case params.IsLeaseError(err):
		return rehydrateLeaseError(err)
	case params.IsCodeTryAgain(err):
		return ErrTryAgain
	case params.IsCodeNotValid(err):
		return errors.NewNotValid(nil, msg)
	default:
		return err
	}
}
