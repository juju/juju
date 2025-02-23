// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"
	"fmt"
	"net/http"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
)

const (
	// ErrorCredentialNotValid represents an error when a provider credential is
	// not valid. Realistically, this is not a transient error. Without a valid
	// credential we cannot do much on the provider. This is fatal.
	ErrorCredentialNotValid = errors.ConstError("credential not valid")
)

// CredentialNotValidError returns an error that satisfy both
// Is(err, ErrorCredentialNotValid) and the errors.Locationer interface.
func CredentialNotValidError(err error) error {
	return errors.SetLocation(
		errors.WithType(err, ErrorCredentialNotValid),
		1,
	)
}

// AuthorisationFailureStatusCodes contains http status code that signify authorisation difficulties.
var AuthorisationFailureStatusCodes = set.NewInts(
	http.StatusUnauthorized,
	http.StatusPaymentRequired,
	http.StatusForbidden,
	http.StatusProxyAuthRequired,
)

// HandleCredentialError determines if a given error relates to an invalid credential.
// If it is, the credential is invalidated and the return bool is true.
func HandleCredentialError(ctx context.Context, invalidator environs.CredentialInvalidator, isAuthError func(error) bool, err error) bool {
	if invalidator == nil {
		logger.Warningf(ctx, "no credential invalidator provided to handle error")
		return false
	}

	if denied := isAuthError(errors.Cause(err)); denied {
		converted := fmt.Errorf("cloud denied access: %w", CredentialNotValidError(err))
		invalidateErr := invalidator.InvalidateCredentials(ctx, environs.CredentialInvalidReason(converted.Error()))
		if invalidateErr != nil {
			logger.Warningf(ctx, "could not invalidate stored cloud credential on the controller: %v", invalidateErr)
		}
		return true
	}
	return false
}
