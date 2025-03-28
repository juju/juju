// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/errors"
	"gopkg.in/macaroon.v2"

	modelerrors "github.com/juju/juju/domain/model/errors"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	interrors "github.com/juju/juju/internal/errors"
	internallogger "github.com/juju/juju/internal/logger"
)

const (
	UpgradeInProgressError = errors.ConstError(CodeUpgradeInProgress)
)

var logger = internallogger.GetLogger("juju.apiserver.params")

// MigrationInProgressError signifies a migration is in progress.
var MigrationInProgressError = errors.New(CodeMigrationInProgress)

// Error is the type of error returned by any call to the state API.
type Error struct {
	Message string         `json:"message"`
	Code    string         `json:"code"`
	Info    map[string]any `json:"info,omitempty"`
}

// WithInfo is responsible for setting the [Error.Info] information
func (e Error) WithInfo(info map[string]any) *Error {
	return &Error{
		Code:    e.Code,
		Message: e.Message,
		Info:    info,
	}
}

func (e Error) Error() string {
	return e.Message
}

func (e Error) ErrorCode() string {
	return e.Code
}

// ErrorInfo implements the rpc.ErrorInfoProvider interface which enables
// API error attachments to be returned as part of RPC error responses.
func (e Error) ErrorInfo() map[string]interface{} {
	return e.Info
}

// GoString implements fmt.GoStringer.  It means that a *Error shows its
// contents correctly when printed with %#v.
func (e Error) GoString() string {
	return fmt.Sprintf("&params.Error{Message: %q, Code: %q}", e.Message, e.Code)
}

// UnmarshalInfo attempts to unmarshal the information contained in the Info
// field of a RequestError into an AdditionalErrorInfo instance a pointer to
// which is passed via the to argument. The method will return an error if a
// non-pointer arg is provided.
func (e Error) UnmarshalInfo(to interface{}) error {
	if reflect.ValueOf(to).Kind() != reflect.Ptr {
		return errors.New("UnmarshalInfo expects a pointer as an argument")
	}

	data, err := json.Marshal(e.Info)
	if err != nil {
		return errors.Annotate(err, "could not marshal error information")
	}
	err = json.Unmarshal(data, to)
	if err != nil {
		return errors.Annotate(err, "could not unmarshal error information to provided target")
	}

	return nil
}

// DischargeRequiredErrorInfo provides additional macaroon information for
// DischargeRequired errors. Note that although these fields are compatible
// with the same fields in httpbakery.ErrorInfo, the Juju API server does not
// implement endpoints directly compatible with that protocol because the error
// response format varies according to the endpoint.
type DischargeRequiredErrorInfo struct {
	// Macaroon may hold a macaroon that, when
	// discharged, may allow access to the juju API.
	// This field is associated with the ErrDischargeRequired
	// error code.
	Macaroon *macaroon.Macaroon `json:"macaroon,omitempty"`

	// BakeryMacaroon may hold a macaroon that, when
	// discharged, may allow access to the juju API.
	// This field is associated with the ErrDischargeRequired
	// error code.
	// This is the macaroon emitted by newer Juju controllers using bakery.v2.
	BakeryMacaroon *bakery.Macaroon `json:"bakery-macaroon,omitempty"`

	// MacaroonPath holds the URL path to be associated
	// with the macaroon. The macaroon is potentially
	// valid for all URLs under the given path.
	// If it is empty, the macaroon will be associated with
	// the original URL from which the error was returned.
	MacaroonPath string `json:"macaroon-path,omitempty"`
}

// AsMap encodes the error info as a map that can be attached to an Error.
func (e DischargeRequiredErrorInfo) AsMap() map[string]interface{} {
	return serializeToMap(e)
}

// RedirectErrorInfo provides additional information for Redirect errors.
type RedirectErrorInfo struct {
	// Servers holds the sets of addresses of the redirected servers.
	Servers [][]HostPort `json:"servers"`

	// CACert holds the certificate of the remote server.
	CACert string `json:"ca-cert"`

	// ControllerTag uniquely identifies the controller being redirected to.
	ControllerTag string `json:"controller-tag,omitempty"`

	// An optional alias for the controller the model migrated to.
	ControllerAlias string `json:"controller-alias,omitempty"`
}

// AsMap encodes the error info as a map that can be attached to an Error.
func (e RedirectErrorInfo) AsMap() map[string]interface{} {
	return serializeToMap(e)
}

// serializeToMap is a convenience function for marshaling v into a
// map[string]interface{}. It works by marshalling v into json and then
// unmarshaling back to a map.
func serializeToMap(v interface{}) map[string]interface{} {
	data, err := json.Marshal(v)
	if err != nil {
		logger.Criticalf(context.TODO(), "serializeToMap: marshal to json failed: %v", err)
		return nil
	}

	var asMap map[string]interface{}
	err = json.Unmarshal(data, &asMap)
	if err != nil {
		logger.Criticalf(context.TODO(), "serializeToMap: unmarshal to map failed: %v", err)
		return nil
	}

	return asMap
}

// The Code constants hold error codes for well known errors.
const (
	CodeNotFound                   = "not found"
	CodeModelNotFound              = "model not found"
	CodeSecretNotFound             = "secret not found"
	CodeSecretRevisionNotFound     = "secret revision not found"
	CodeSecretBackendNotFound      = "secret backend not found"
	CodeSecretConsumerNotFound     = "secret consumer not found"
	CodeUnauthorized               = "unauthorized access"
	CodeSessionTokenInvalid        = "session token invalid"
	CodeLoginExpired               = "login expired"
	CodeNoCreds                    = "no credentials provided"
	CodeCannotEnterScope           = "cannot enter scope"
	CodeCannotEnterScopeYet        = "cannot enter scope yet"
	CodeExcessiveContention        = "excessive contention"
	CodeUnitHasSubordinates        = "unit has subordinates"
	CodeNotAssigned                = "not assigned"
	CodeStopped                    = "stopped"
	CodeDead                       = "dead"
	CodeHasAssignedUnits           = "machine has assigned units"
	CodeHasHostedModels            = "controller has hosted models"
	CodeHasPersistentStorage       = "controller/model has persistent storage"
	CodeModelNotEmpty              = "model not empty"
	CodeMachineHasAttachedStorage  = "machine has attached storage"
	CodeMachineHasContainers       = "machine is hosting containers"
	CodeStorageAttached            = "storage is attached"
	CodeNotProvisioned             = "not provisioned"
	CodeNoAddressSet               = "no address set"
	CodeTryAgain                   = "try again"
	CodeNotImplemented             = "not implemented" // asserted to match rpc.codeNotImplemented in rpc/rpc_test.go
	CodeAlreadyExists              = "already exists"
	CodeSecretBackendAlreadyExists = "secret backend already exists"
	CodeUpgradeInProgress          = "upgrade in progress"
	CodeMigrationInProgress        = "model migration in progress"
	CodeActionNotAvailable         = "action no longer available"
	CodeOperationBlocked           = "operation is blocked"
	CodeLeadershipClaimDenied      = "leadership claim denied"
	CodeLeaseClaimDenied           = "lease claim denied"
	CodeNotSupported               = "not supported"
	CodeSecretBackendNotSupported  = "secret backend not supported"
	CodeBadRequest                 = "bad request"
	CodeMethodNotAllowed           = "method not allowed"
	CodeForbidden                  = "forbidden"
	CodeSecretBackendForbidden     = "secret backend forbidden"
	CodeDischargeRequired          = "macaroon discharge required"
	CodeRedirect                   = "redirection required"
	CodeIncompatibleBase           = "incompatible base"
	CodeCloudRegionRequired        = "cloud region required"
	CodeIncompatibleClouds         = "incompatible clouds"
	CodeQuotaLimitExceeded         = "quota limit exceeded"
	CodeNotLeader                  = "not leader"
	CodeDeadlineExceeded           = "deadline exceeded"
	CodeNotYetAvailable            = "not yet available; try again later"
	CodeNotValid                   = "not valid"
	CodeSecretBackendNotValid      = "secret backend not valid"
	CodeAccessRequired             = "access required"
	CodeAppShouldNotHaveUnits      = "application should not have units"

	//
	// Tag based error
	//

	// CodeTagInvalid represents an error code when the tag supplied by the
	// caller is not parsable.
	CodeTagInvalid = "invalid tag"

	// CodeTagKindNotSupport represents an error code when a tag has been
	// provided to a facade call and the tags kind is unsupported by the facade.
	CodeTagKindNotSupported = "tag kind not supported"

	//
	// Machine based errors
	//

	// CodeMachineInvalidID represents an error code that indicates a supplied
	// machine id is invalid.
	CodeMachineInvalidID = "invalid machine id"

	// CodeMachineNotFound represents an error code that indicates the machine
	// requested does not exist.
	CodeMachineNotFound = "machine not found"

	//
	// User based errors
	//

	// CodeUserInvalidName represents an error that happens when a user name
	// has been supplied that is invalid.
	CodeUserInvalidName = "invalid user name"

	// CodeUserNotFound represents an error that happens when a user requested
	// does not exist.
	CodeUserNotFound = "user not found"

	//
	// User ssh key errors
	//

	// CodeUserKeyInvalidComment represents an error where a requested key to be
	// added by a user violates the Juju comment restrictions.
	CodeUserKeyInvalidComment = "invalid public key comment"

	// CodeUserKeyInvalidKey represents an error where a requested key to be
	// added is not considered valid.
	CodeUserKeyInvalidKey = "invalid public key"

	// CodeUserKeyAlreadyExists represents an error where a requested key to be
	// added already exists for the user.
	CodeUserKeyAlreadyExists = "public key already exists"

	// CodeUserKeyInvalidKeySource represents an error where by a public key
	// ssh import source is not valid.
	CodeUserKeyInvalidKeySource = "invalid user public key source"

	// CodeUserKeyUnknownKeySource represents an error where the public key
	// source being asked to import for is unknown and not supported.
	CodeUserKeyUnknownKeySource = "unknown user public key source"

	// CodeUserKeySourceSubjectNotFound represents an error where the key source
	// has told us the subject being imported does not exist.
	CodeUserKeySourceSubjectNotFound = "key source subject not found"
)

// TranslateWellKnownError translates well known wire error codes into a github.com/juju/errors error
// that matches the error code.
func TranslateWellKnownError(err error) error {
	code := ErrCode(err)
	switch code {
	// TODO: add more error cases including DeadlineExceeded
	// case CodeDeadlineExceeded:
	// 	return errors.NewTimeout(err, "")
	case CodeNotFound:
		return errors.NewNotFound(err, "")
	case CodeUserNotFound:
		return errors.NewUserNotFound(err, "")
	case CodeSecretNotFound:
		return fmt.Errorf("%s%w", err.Error(), errors.Hide(secreterrors.SecretNotFound))
	case CodeSecretRevisionNotFound:
		return fmt.Errorf("%s%w", err.Error(), errors.Hide(secreterrors.SecretRevisionNotFound))
	case CodeSecretConsumerNotFound:
		return fmt.Errorf("%s%w", err.Error(), errors.Hide(secreterrors.SecretConsumerNotFound))
	case CodeSecretBackendNotFound:
		return fmt.Errorf("%s%w", err.Error(), errors.Hide(secretbackenderrors.NotFound))
	case CodeUnauthorized:
		return errors.NewUnauthorized(err, "")
	case CodeNotImplemented:
		return errors.NewNotImplemented(err, "")
	case CodeAlreadyExists:
		return errors.NewAlreadyExists(err, "")
	case CodeSecretBackendAlreadyExists:
		return fmt.Errorf("%s%w", err.Error(), errors.Hide(secretbackenderrors.AlreadyExists))
	case CodeNotSupported:
		return errors.NewNotSupported(err, "")
	case CodeNotValid:
		return errors.NewNotValid(err, "")
	case CodeSecretBackendNotSupported:
		return fmt.Errorf("%s%w", err.Error(), errors.Hide(secretbackenderrors.NotSupported))
	case CodeSecretBackendNotValid:
		return fmt.Errorf("%s%w", err.Error(), errors.Hide(secretbackenderrors.NotValid))
	case CodeNotProvisioned:
		return errors.NewNotProvisioned(err, "")
	case CodeNotAssigned:
		return errors.NewNotAssigned(err, "")
	case CodeBadRequest:
		return errors.NewBadRequest(err, "")
	case CodeMethodNotAllowed:
		return errors.NewMethodNotAllowed(err, "")
	case CodeForbidden:
		return errors.NewForbidden(err, "")
	case CodeSecretBackendForbidden:
		return fmt.Errorf("%s%w", err.Error(), errors.Hide(secretbackenderrors.Forbidden))
	case CodeQuotaLimitExceeded:
		return errors.NewQuotaLimitExceeded(err, "")
	case CodeNotYetAvailable:
		return errors.NewNotYetAvailable(err, "")
	case CodeModelNotFound:
		return fmt.Errorf("%s%w", err.Error(), errors.Hide(modelerrors.NotFound))
	}
	return err
}

// ErrCode returns the error code associated with
// the given error, or the empty string if there
// is none.
func ErrCode(err error) string {
	type ErrorCoder interface {
		error
		ErrorCode() string
	}

	// NOTE (tlm):
	// Don't remove this line!!!!
	// Because we use a very outdated http request library it still wraps some
	// of it's errors with errgo pkg. We need Cause here to potentially pull
	// out errors from this library.
	//
	// The soon we remove httprequest from Juju the better life will be.
	err = errors.Cause(err)

	coder, is := interrors.AsType[ErrorCoder](err)
	if is {
		return coder.ErrorCode()
	}
	return ""
}

func IsCodeActionNotAvailable(err error) bool {
	return ErrCode(err) == CodeActionNotAvailable
}

func IsCodeNotFound(err error) bool {
	return ErrCode(err) == CodeNotFound
}

func IsCodeNotValid(err error) bool {
	return ErrCode(err) == CodeNotValid
}

func IsCodeUserNotFound(err error) bool {
	return ErrCode(err) == CodeUserNotFound
}

func IsCodeModelNotFound(err error) bool {
	return ErrCode(err) == CodeModelNotFound
}

func IsCodeSecretNotFound(err error) bool {
	return ErrCode(err) == CodeSecretNotFound
}

func IsCodeSecretRevisionNotFound(err error) bool {
	return ErrCode(err) == CodeSecretRevisionNotFound
}

func IsCodeSecretConsumerNotFound(err error) bool {
	return ErrCode(err) == CodeSecretConsumerNotFound
}

func IsCodeSecretBackendNotFound(err error) bool {
	return ErrCode(err) == CodeSecretBackendNotFound
}

func IsCodeSecretBackendForbidden(err error) bool {
	return ErrCode(err) == CodeSecretBackendForbidden
}

func IsCodeUnauthorized(err error) bool {
	return ErrCode(err) == CodeUnauthorized
}

// IsCodeSessionTokenInvalid returns true if err includes a SessionTokenInvalid
// error code.
func IsCodeSessionTokenInvalid(err error) bool {
	return ErrCode(err) == CodeSessionTokenInvalid
}

func IsCodeNoCreds(err error) bool {
	// When we receive this error from an rpc call, rpc.RequestError
	// is populated with a CodeUnauthorized code and a message that
	// is formatted as "$CodeNoCreds ($CodeUnauthorized)".
	ec := ErrCode(err)
	return ec == CodeNoCreds || (ec == CodeUnauthorized && strings.HasPrefix(errors.Cause(err).Error(), CodeNoCreds))
}

func IsCodeNotYetAvailable(err error) bool {
	return ErrCode(err) == CodeNotYetAvailable
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

func IsCodeHasHostedModels(err error) bool {
	return ErrCode(err) == CodeHasHostedModels
}

func IsCodeHasPersistentStorage(err error) bool {
	return ErrCode(err) == CodeHasPersistentStorage
}

func IsCodeModelNotEmpty(err error) bool {
	return ErrCode(err) == CodeModelNotEmpty
}

func IsCodeMachineHasAttachedStorage(err error) bool {
	return ErrCode(err) == CodeMachineHasAttachedStorage
}

func IsCodeMachineHasContainers(err error) bool {
	return ErrCode(err) == CodeMachineHasContainers
}

func IsCodeStorageAttached(err error) bool {
	return ErrCode(err) == CodeStorageAttached
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

func IsRedirect(err error) bool {
	return ErrCode(err) == CodeRedirect
}

func IsCodeForbidden(err error) bool {
	return ErrCode(err) == CodeForbidden
}

// IsCodeQuotaLimitExceeded returns true if err includes a QuotaLimitExceeded
// error code.
func IsCodeQuotaLimitExceeded(err error) bool {
	return ErrCode(err) == CodeQuotaLimitExceeded
}

func IsCodeNotLeader(err error) bool {
	return ErrCode(err) == CodeNotLeader
}

func IsCodeDeadlineExceeded(err error) bool {
	return ErrCode(err) == CodeDeadlineExceeded
}

func IsCodeAppShouldNotHaveUnits(err error) bool {
	return ErrCode(err) == CodeAppShouldNotHaveUnits
}
