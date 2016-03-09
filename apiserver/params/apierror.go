// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/macaroon.v1"
)

// Error is the type of error returned by any call to the state API.
type Error struct {
	Message string
	Code    string
	Info    *ErrorInfo `json:",omitempty"`
}

// ErrorInfo holds additional information provided by an error.
// Note that although these fields are compatible with the
// same fields in httpbakery.ErrorInfo, the Juju API server does
// not implement endpoints directly compatible with that protocol
// because the error response format varies according to
// the endpoint.
type ErrorInfo struct {
	// Macaroon may hold a macaroon that, when
	// discharged, may allow access to the juju API.
	// This field is associated with the ErrDischargeRequired
	// error code.
	Macaroon *macaroon.Macaroon `json:",omitempty"`

	// MacaroonPath holds the URL path to be associated
	// with the macaroon. The macaroon is potentially
	// valid for all URLs under the given path.
	// If it is empty, the macaroon will be associated with
	// the original URL from which the error was returned.
	MacaroonPath string `json:",omitempty"`
}

func (e Error) Error() string {
	return e.Message
}

func (e Error) ErrorCode() string {
	return e.Code
}

// GoString implements fmt.GoStringer.  It means that a *Error shows its
// contents correctly when printed with %#v.
func (e Error) GoString() string {
	return fmt.Sprintf("&params.Error{Message: %q, Code: %q}", e.Message, e.Code)
}

// The Code constants hold error codes for some kinds of error.
const (
	CodeNotFound                  = "not found"
	CodeUnauthorized              = "unauthorized access"
	CodeCannotEnterScope          = "cannot enter scope"
	CodeCannotEnterScopeYet       = "cannot enter scope yet"
	CodeExcessiveContention       = "excessive contention"
	CodeUnitHasSubordinates       = "unit has subordinates"
	CodeNotAssigned               = "not assigned"
	CodeStopped                   = "stopped"
	CodeDead                      = "dead"
	CodeHasAssignedUnits          = "machine has assigned units"
	CodeMachineHasAttachedStorage = "machine has attached storage"
	CodeNotProvisioned            = "not provisioned"
	CodeNoAddressSet              = "no address set"
	CodeTryAgain                  = "try again"
	CodeNotImplemented            = "not implemented" // asserted to match rpc.codeNotImplemented in rpc/rpc_test.go
	CodeAlreadyExists             = "already exists"
	CodeUpgradeInProgress         = "upgrade in progress"
	CodeActionNotAvailable        = "action no longer available"
	CodeOperationBlocked          = "operation is blocked"
	CodeLeadershipClaimDenied     = "leadership claim denied"
	CodeLeaseClaimDenied          = "lease claim denied"
	CodeNotSupported              = "not supported"
	CodeBadRequest                = "bad request"
	CodeMethodNotAllowed          = "method not allowed"
	CodeForbidden                 = "forbidden"
	CodeDischargeRequired         = "macaroon discharge required"
)

// ErrCode returns the error code associated with
// the given error, or the empty string if there
// is none.
func ErrCode(err error) string {
	type ErrorCoder interface {
		ErrorCode() string
	}
	switch err := errors.Cause(err).(type) {
	case ErrorCoder:
		return err.ErrorCode()
	default:
		return ""
	}
}

func IsCodeActionNotAvailable(err error) bool {
	return ErrCode(err) == CodeActionNotAvailable
}

func IsCodeNotFound(err error) bool {
	return ErrCode(err) == CodeNotFound
}

func IsCodeUnauthorized(err error) bool {
	return ErrCode(err) == CodeUnauthorized
}

// IsCodeNotFoundOrCodeUnauthorized is used in API clients which,
// pre-API, used errors.IsNotFound; this is because an API client is
// not necessarily privileged to know about the existence or otherwise
// of a particular entity, and the server may hence convert NotFound
// to Unauthorized at its discretion.
func IsCodeNotFoundOrCodeUnauthorized(err error) bool {
	return IsCodeNotFound(err) || IsCodeUnauthorized(err)
}

func IsCodeCannotEnterScope(err error) bool {
	return ErrCode(err) == CodeCannotEnterScope
}

func IsCodeCannotEnterScopeYet(err error) bool {
	return ErrCode(err) == CodeCannotEnterScopeYet
}

func IsCodeExcessiveContention(err error) bool {
	return ErrCode(err) == CodeExcessiveContention
}

func IsCodeUnitHasSubordinates(err error) bool {
	return ErrCode(err) == CodeUnitHasSubordinates
}

func IsCodeNotAssigned(err error) bool {
	return ErrCode(err) == CodeNotAssigned
}

func IsCodeStopped(err error) bool {
	return ErrCode(err) == CodeStopped
}

func IsCodeDead(err error) bool {
	return ErrCode(err) == CodeDead
}

func IsCodeHasAssignedUnits(err error) bool {
	return ErrCode(err) == CodeHasAssignedUnits
}

func IsCodeMachineHasAttachedStorage(err error) bool {
	return ErrCode(err) == CodeMachineHasAttachedStorage
}

func IsCodeNotProvisioned(err error) bool {
	return ErrCode(err) == CodeNotProvisioned
}

func IsCodeNoAddressSet(err error) bool {
	return ErrCode(err) == CodeNoAddressSet
}

func IsCodeTryAgain(err error) bool {
	return ErrCode(err) == CodeTryAgain
}

func IsCodeNotImplemented(err error) bool {
	return ErrCode(err) == CodeNotImplemented
}

func IsCodeAlreadyExists(err error) bool {
	return ErrCode(err) == CodeAlreadyExists
}

func IsCodeUpgradeInProgress(err error) bool {
	return ErrCode(err) == CodeUpgradeInProgress
}

func IsCodeOperationBlocked(err error) bool {
	return ErrCode(err) == CodeOperationBlocked
}

func IsCodeLeadershipClaimDenied(err error) bool {
	return ErrCode(err) == CodeLeadershipClaimDenied
}

func IsCodeLeaseClaimDenied(err error) bool {
	return ErrCode(err) == CodeLeaseClaimDenied
}

func IsCodeNotSupported(err error) bool {
	return ErrCode(err) == CodeNotSupported
}

func IsBadRequest(err error) bool {
	return ErrCode(err) == CodeBadRequest
}

func IsMethodNotAllowed(err error) bool {
	return ErrCode(err) == CodeMethodNotAllowed
}
