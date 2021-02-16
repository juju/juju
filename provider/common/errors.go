// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"net/http"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/environs/context"
)

// ZoneIndependentError wraps the given error such that it
// satisfies environs.IsAvailabilityZoneIndependent.
func ZoneIndependentError(err error) error {
	if err == nil {
		return nil
	}
	wrapped := errors.Wrap(err, zoneIndependentError{err})
	wrapped.(*errors.Err).SetLocation(1)
	return wrapped
}

type zoneIndependentError struct {
	error
}

// AvailabilityZoneIndependent is part of the
// environs.AvailabilityZoneError interface.
func (zoneIndependentError) AvailabilityZoneIndependent() bool {
	return true
}

// credentialNotValid represents an error when a provider credential is not valid.
// Realistically, this is not a transient error. Without a valid credential we
// cannot do much on the provider. This is fatal.
type credentialNotValid struct {
	error
}

// CredentialNotValid returns an error which wraps err and satisfies
// IsCredentialNotValid().
func CredentialNotValid(err error) error {
	if err == nil {
		return nil
	}
	wrapped := errors.Wrap(err, &credentialNotValid{err})
	wrapped.(*errors.Err).SetLocation(1)
	return wrapped
}

// NewCredentialNotValid returns an error with given message and satisfies
// IsCredentialNotValid().
func NewCredentialNotValid(message string) error {
	err := errors.New("credential not valid: " + message)
	wrapped := errors.Wrap(err, &credentialNotValid{err})
	wrapped.(*errors.Err).SetLocation(1)
	return wrapped
}

// CredentialNotValidf returns a wrapped error with given message and satisfies
// IsCredentialNotValid().
func CredentialNotValidf(err error, message string) error {
	wrapped := errors.Wrapf(err, &credentialNotValid{err}, message)
	wrapped.(*errors.Err).SetLocation(1)
	return wrapped
}

// IsCredentialNotValid reports whether err was created with CredentialNotValid().
func IsCredentialNotValid(err error) bool {
	err = errors.Cause(err)
	_, ok := err.(*credentialNotValid)
	return ok
}

// AuthorisationFailureStatusCodes contains http status code that signify authorisation difficulties.
var AuthorisationFailureStatusCodes = set.NewInts(
	http.StatusUnauthorized,
	http.StatusPaymentRequired,
	http.StatusForbidden,
	http.StatusProxyAuthRequired,
)

// MaybeHandleCredentialError determines if a given error relates to an invalid credential.
// If it is, the credential is invalidated and the return bool is true.
func MaybeHandleCredentialError(isAuthError func(error) bool, err error, ctx context.ProviderCallContext) bool {
	denied := isAuthError(errors.Cause(err))
	if ctx != nil && denied {
		converted := CredentialNotValidf(err, "cloud denied access")
		invalidateErr := ctx.InvalidateCredential(converted.Error())
		if invalidateErr != nil {
			logger.Warningf("could not invalidate stored cloud credential on the controller: %v", invalidateErr)
		}
	}
	return denied
}

// HandleCredentialError determines if a given error relates to an invalid credential.
func HandleCredentialError(isAuthError func(error) bool, err error, ctx context.ProviderCallContext) {
	MaybeHandleCredentialError(isAuthError, err, ctx)
}
