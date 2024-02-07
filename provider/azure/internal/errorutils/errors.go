// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errorutils

import (
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"

	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/provider/common"
)

var logger = loggo.GetLogger("juju.provider.azure")

type requestError struct {
	ServiceError *serviceError `json:"error"`
}

type serviceError struct {
	Code    string                `json:"code"`
	Message string                `json:"message"`
	Details []serviceErrorDetails `json:"details"`
}

type serviceErrorDetails struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// MaybeQuotaExceededError returns the relevant error message and true
// if the error is caused by a Quota Exceeded issue.
func MaybeQuotaExceededError(err error) (error, bool) {
	var respErr *azcore.ResponseError
	if !errors.As(err, &respErr) {
		return err, false
	}
	if respErr.StatusCode != http.StatusBadRequest {
		return respErr, false
	}
	var reqErr requestError
	if err = runtime.UnmarshalAsJSON(respErr.RawResponse, &reqErr); err != nil {
		return respErr, false
	}
	if reqErr.ServiceError == nil {
		return respErr, false
	}
	if reqErr.ServiceError.Code == "QuotaExceeded" {
		return errors.New(reqErr.ServiceError.Message), true
	}
	for _, d := range reqErr.ServiceError.Details {
		if d.Code == "QuotaExceeded" {
			return errors.New(d.Message), true
		}
	}
	return respErr, false
}

func hasErrorCode(resp *http.Response, code string) bool {
	if resp == nil {
		return false
	}
	var reqErr requestError
	if err := runtime.UnmarshalAsJSON(resp, &reqErr); err != nil {
		return false
	}
	if reqErr.ServiceError == nil {
		return false
	}
	if reqErr.ServiceError.Code == code {
		return true
	}
	for _, d := range reqErr.ServiceError.Details {
		if d.Code == code {
			return true
		}
	}
	return false
}

// IsNotFoundError returns true if the error is
// caused by a not found error.
func IsNotFoundError(err error) bool {
	var respErr *azcore.ResponseError
	if !errors.As(err, &respErr) {
		return false
	}
	return respErr.StatusCode == http.StatusNotFound ||
		hasErrorCode(respErr.RawResponse, "NotFound")
}

// IsConflictError returns true if the error is
// caused by a deployment Conflict.
func IsConflictError(err error) bool {
	var respErr *azcore.ResponseError
	if !errors.As(err, &respErr) {
		return false
	}
	return respErr.StatusCode == http.StatusConflict ||
		hasErrorCode(respErr.RawResponse, "Conflict")
}

// IsForbiddenError returns true if the error is
// caused by a forbidden error.
func IsForbiddenError(err error) bool {
	var respErr *azcore.ResponseError
	if !errors.As(err, &respErr) {
		return false
	}
	return respErr.StatusCode == http.StatusForbidden ||
		hasErrorCode(respErr.RawResponse, "Forbidden")
}

// ErrorCode returns the top level error code
// if the error is a ResponseError.
func ErrorCode(err error) string {
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		return respErr.ErrorCode
	}
	return ""
}

// StatusCode returns the top level status code
// if the error is a ResponseError.
func StatusCode(err error) int {
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		return respErr.StatusCode
	}
	return 0
}

// SimpleError returns an error with the "interesting"
// content from a ResponseError.
func SimpleError(err error) error {
	var respErr *azcore.ResponseError
	if !errors.As(err, &respErr) {
		return err
	}
	var reqErr requestError
	if err := runtime.UnmarshalAsJSON(respErr.RawResponse, &reqErr); err != nil {
		return respErr
	}
	if reqErr.ServiceError == nil {
		return respErr
	}
	msg := ""
	if len(reqErr.ServiceError.Details) > 0 {
		msg = reqErr.ServiceError.Details[0].Message
	}
	if msg == "" {
		msg = reqErr.ServiceError.Message
	}
	return errors.New(msg)
}

// HandleCredentialError determines if given error relates to invalid credential.
// If it is, the credential is invalidated.
// Original error is returned untouched.
func HandleCredentialError(err error, ctx envcontext.ProviderCallContext) error {
	MaybeInvalidateCredential(err, ctx)
	return err
}

// MaybeInvalidateCredential determines if given error is related to authentication/authorisation failures.
// If an error is related to an invalid credential, then this call will try to invalidate that credential as well.
func MaybeInvalidateCredential(err error, ctx envcontext.ProviderCallContext) bool {
	if !HasDenialStatusCode(err) {
		return false
	}

	converted := fmt.Errorf("azure cloud denied access: %w", common.CredentialNotValidError(err))
	invalidateErr := ctx.InvalidateCredential(converted.Error())
	if invalidateErr != nil {
		logger.Warningf("could not invalidate stored azure cloud credential on the controller: %v", invalidateErr)
	}
	return true
}

// HasDenialStatusCode returns true of the error has a status code
// meaning that the credential is invalid.
func HasDenialStatusCode(err error) bool {
	if err == nil {
		return false
	}
	return common.AuthorisationFailureStatusCodes.Contains(StatusCode(err))
}
