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
	"github.com/juju/juju/environs/envcontext"
)

const (
	// ErrorCredentialNotValid represents an error when a provider credential is
	// not valid. Realistically, this is not a transient error. Without a valid
	// credential we cannot do much on the provider. This is fatal.
	ErrorCredentialNotValid = errors.ConstError("credential not valid")
)

// AuthErrorFunc is a function that determines if an error is an authentication
// error.
type AuthErrorFunc func(error) bool

// CredentialInvalidator is a provider of invalidation of credentials.
// credentialInvalidator is used to invalidate the credentials
// when necessary. This will cause the provider to be unable to
// perform any operations until the credentials are updated/fixed.
type CredentialInvalidator interface {
	// InvalidateCredentials invalidates the credentials.
	InvalidateCredentials(ctx context.Context, reason environs.CredentialInvalidReason) error

	// HandleCredentialError determines if a given error relates to an invalid
	// credential. If it is, the credential is invalidated and the returns the
	// origin error.
	HandleCredentialError(ctx context.Context, err error) error

	// MaybeInvalidateCredentialError determines if a given error relates to an
	// invalid credential. If it is, the credential is invalidated and the return
	// bool is true and the origin error.
	MaybeInvalidateCredentialError(ctx context.Context, err error) (bool, error)
}

type credentialInvalidator struct {
	invalidator environs.CredentialInvalidator
	authError   AuthErrorFunc
}

// NewCredentialInvalidator returns a new CredentialInvalidator.
func NewCredentialInvalidator(invalidator environs.CredentialInvalidator, authError AuthErrorFunc) CredentialInvalidator {
	return credentialInvalidator{
		invalidator: invalidator,
		authError:   authError,
	}
}

// InvalidateCredentials invalidates the credentials.
func (c credentialInvalidator) InvalidateCredentials(ctx context.Context, reason environs.CredentialInvalidReason) error {
	return c.invalidator.InvalidateCredentials(ctx, reason)
}

// HandleCredentialError determines if a given error relates to an invalid
// credential. If it is, the credential is invalidated and the returns the
// origin error.
func (c credentialInvalidator) HandleCredentialError(ctx context.Context, err error) error {
	_, invalidErr := HandleCredentialError(ctx, c.invalidator, c.authError, errors.Trace(err))
	return invalidErr
}

// MaybeInvalidateCredentialError determines if a given error relates to an
// invalid credential. If it is, the credential is invalidated and the return
// bool is true and the origin error.
func (c credentialInvalidator) MaybeInvalidateCredentialError(ctx context.Context, err error) (bool, error) {
	return HandleCredentialError(ctx, c.invalidator, c.authError, errors.Trace(err))
}

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

// HandleCredentialError determines if a given error relates to an invalid
// credential. If it is, the credential is invalidated and the return bool is
// true.
func HandleCredentialError(ctx context.Context, invalidator environs.CredentialInvalidator, isAuthError func(error) bool, err error) (bool, error) {
	if invalidator == nil {
		logger.Warningf(ctx, "no credential invalidator provided to handle error")
		return false, err
	}

	// TODO (stickupkid): We should remove the `errors.Cause` and let the caller
	// handle this, otherwise we could be dropping vital information when each
	// provider checks the error.
	if denied := isAuthError(errors.Cause(err)); denied {
		converted := fmt.Errorf("cloud denied access: %w", CredentialNotValidError(err))
		invalidateErr := invalidator.InvalidateCredentials(ctx, environs.CredentialInvalidReason(converted.Error()))
		if invalidateErr != nil {
			logger.Warningf(ctx, "could not invalidate stored cloud credential on the controller: %v", invalidateErr)
		}
		return true, err
	}
	return false, err
}

// LegacyHandleCredentialError determines if a given error relates to an invalid
// credential. If it is, the credential is invalidated and the return bool is
// true.
// Deprecated: use HandleCredentialError instead.
func LegacyHandleCredentialError(isAuthError func(error) bool, err error, ctx envcontext.ProviderCallContext) bool {
	denied := isAuthError(errors.Cause(err))
	if denied {
		converted := fmt.Errorf("cloud denied access: %w", CredentialNotValidError(err))
		invalidateErr := ctx.InvalidateCredential(converted.Error())
		if invalidateErr != nil {
			logger.Warningf(ctx, "could not invalidate stored cloud credential on the controller: %v", invalidateErr)
		}
	}
	return denied
}
