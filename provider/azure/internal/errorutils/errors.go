// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errorutils

import (
	"net/http"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/set"

	"github.com/juju/juju/environs/context"
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
	return nil, false
}

// AuthorisationFailureStatusCodes contains http status code that signify authorisation difficulties.
var AuthorisationFailureStatusCodes = set.NewInts(
	http.StatusUnauthorized,
	http.StatusPaymentRequired,
	http.StatusForbidden,
	http.StatusProxyAuthRequired,
)

// HandleCredentialError determines if given error relates to invalid credential.
// If it is, the credential is invalidated.
// Original error is returned untouched.
func HandleCredentialError(err error, ctx context.ProviderCallContext) error {
	MaybeHandleCredentialError(err, ctx)
	return err
}

// MaybeHandleCredentialError determines if given error has authorisation denial codes embedded.
// If a code related to an invalid credential is found, the credential is invalidated as well.
func MaybeHandleCredentialError(err error, ctx context.ProviderCallContext) bool {
	if ctx == nil {
		return false
	}
	if !hasDenialStatusCode(err) {
		return false
	}

	invalidateErr := ctx.InvalidateCredential("azure cloud denied access")
	if invalidateErr != nil {
		logger.Infof("could not invalidate stored azure cloud credential on the controller")
	}
	return true
}

func hasDenialStatusCode(err error) bool {
	if err == nil {
		return false
	}

	if d, ok := errors.Cause(err).(autorest.DetailedError); ok {
		if d.Response != nil {
			return AuthorisationFailureStatusCodes.Contains(d.Response.StatusCode)
		}
		return AuthorisationFailureStatusCodes.Contains(d.StatusCode.(int))
	}
	return false
}
