// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errorutils

import (
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/common"
)

var logger = loggo.GetLogger("juju.provider.azure")

// ServiceError returns the *azure.ServiceError underlying the
// supplied error, if any, and a bool indicating whether one
// was found.
func ServiceError(err error) (*azure.ServiceError, bool) {
	err = errors.Cause(err)
	if d, ok := err.(autorest.DetailedError); ok {
		err = d.Original
	}
	if r, ok := err.(*azure.RequestError); ok {
		return r.ServiceError, true
	}
	// The error Azure gives us back can also be a struct
	// not a pointer.
	if r, ok := err.(azure.RequestError); ok {
		return r.ServiceError, true
	}
	return nil, false
}

// HandleCredentialError determines if given error relates to invalid credential.
// If it is, the credential is invalidated.
// Original error is returned untouched.
func HandleCredentialError(err error, ctx context.ProviderCallContext) error {
	MaybeInvalidateCredential(err, ctx)
	return err
}

// MaybeInvalidateCredential determines if given error is related to authentication/authorisation failures.
// If an error is related to an invalid credential, then this call will try to invalidate that credential as well.
func MaybeInvalidateCredential(err error, ctx context.ProviderCallContext) bool {
	if ctx == nil {
		return false
	}
	if !hasDenialStatusCode(err) {
		return false
	}

	invalidateErr := ctx.InvalidateCredential("azure cloud denied access")
	if invalidateErr != nil {
		logger.Warningf("could not invalidate stored azure cloud credential on the controller: %v", invalidateErr)
	}
	return true
}

func hasDenialStatusCode(err error) bool {
	if err == nil {
		return false
	}

	if d, ok := errors.Cause(err).(autorest.DetailedError); ok {
		if d.Response != nil {
			return common.AuthorisationFailureStatusCodes.Contains(d.Response.StatusCode)
		}
		return common.AuthorisationFailureStatusCodes.Contains(d.StatusCode.(int))
	}
	return false
}
