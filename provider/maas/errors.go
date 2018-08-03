// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"github.com/juju/errors"
	"github.com/juju/gomaasapi"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/common"
)

// MaybeHandleCredentialError determines if a given error relates to an invalid credential.
//  If it is, the credential is invalidated and the return bool is true.
// Original error is returned untouched.
func MaybeHandleCredentialError(err error, ctx context.ProviderCallContext) (error, bool) {
	denied := IsAuthorisationFailure(errors.Cause(err))
	if ctx != nil && denied {
		invalidateErr := ctx.InvalidateCredential("maas cloud denied access")
		if invalidateErr != nil {
			logger.Warningf("could not invalidate stored maas cloud credential on the controller: %v", invalidateErr)
		}
	}
	return err, denied
}

// HandleCredentialError determines if a given error relates to an invalid credential.
// If it is, the credential is invalidated. Original error is returned untouched.
func HandleCredentialError(err error, ctx context.ProviderCallContext) error {
	MaybeHandleCredentialError(err, ctx)
	return err
}

// IsAuthorisationFailure determines if the given error has an authorisation failure.
func IsAuthorisationFailure(err error) bool {
	// This should cover most cases.
	if gomaasapi.IsPermissionError(err) {
		return true
	}

	// This should cover exceptional circumstances.
	if maasErr, ok := err.(gomaasapi.ServerError); ok {
		return common.AuthorisationFailureStatusCodes.Contains(maasErr.StatusCode)
	}
	return false
}
