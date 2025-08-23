// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/juju/errors"
	"google.golang.org/api/googleapi"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/common"
)

// InvalidConfigValueError indicates that one of the config values failed validation.
type InvalidConfigValueError struct {
	errors.Err

	// Key is the OS env var corresponding to the field with the bad value.
	Key string

	// Value is the invalid value.
	Value interface{}
}

// IsInvalidConfigValueError returns whether or not the cause of
// the provided error is a *InvalidConfigValueError.
func IsInvalidConfigValueError(err error) bool {
	_, ok := errors.Cause(err).(*InvalidConfigValueError)
	return ok
}

// NewInvalidConfigValueError returns a new InvalidConfigValueError for the given
// info. If the provided reason is an error then Reason is set to that
// error. Otherwise a non-nil value is treated as a string and Reason is
// set to a non-nil value that wraps it.
func NewInvalidConfigValueError(key, value string, reason error) error {
	err := &InvalidConfigValueError{
		Err:   *errors.Mask(reason).(*errors.Err),
		Key:   key,
		Value: value,
	}
	err.Err.SetLocation(1)
	return err
}

// Cause implements errors.Causer.Cause.
func (err *InvalidConfigValueError) Cause() error {
	return err
}

// NewMissingConfigValue returns a new error for a missing config field.
func NewMissingConfigValue(key, field string) error {
	return NewInvalidConfigValueError(key, "", errors.New("missing "+field))
}

// Error implements error.
func (err InvalidConfigValueError) Error() string {
	return fmt.Sprintf("invalid config value (%s) for %q: %v", err.Value, err.Key, &err.Err)
}

// HandleCredentialError determines if a given error relates to an invalid credential.
// If it is, the credential is invalidated. Original error is returned untouched.
func HandleCredentialError(err error, ctx context.ProviderCallContext) error {
	maybeInvalidateCredential(err, ctx)
	return err
}

func maybeInvalidateCredential(err error, ctx context.ProviderCallContext) bool {
	if ctx == nil {
		return false
	}
	if !HasDenialStatusCode(err) {
		return false
	}

	converted := fmt.Errorf("google cloud denied access: %w", common.CredentialNotValidError(err))
	invalidateErr := ctx.InvalidateCredential(converted.Error())
	if invalidateErr != nil {
		logger.Warningf("could not invalidate stored google cloud credential on the controller: %v", invalidateErr)
	}
	return true
}

// HasDenialStatusCode determines if the given error was caused by an invalid credential, i.e. whether it contains a
// response status code that indicates an authentication failure.
func HasDenialStatusCode(err error) bool {
	if err == nil {
		return false
	}

	var cause error
	switch e := errors.Cause(err).(type) {
	case *url.Error:
		cause = e
	case *googleapi.Error:
		cause = e
	default:
		return false
	}

	for code, descs := range AuthorisationFailureStatusCodes {
		for _, desc := range descs {
			if strings.Contains(cause.Error(), fmt.Sprintf(": %v %v", code, desc)) {
				return true
			}
		}
	}
	return false
}

// AuthorisationFailureStatusCodes contains http status code and
// description that signify authorisation difficulties.
//
// Google does not always use standard HTTP descriptions, which
// is why a single status code can map to multiple descriptions.
var AuthorisationFailureStatusCodes = map[int][]string{
	http.StatusUnauthorized:      {"Unauthorized"},
	http.StatusPaymentRequired:   {"Payment Required"},
	http.StatusForbidden:         {"Forbidden", "Access Not Configured"},
	http.StatusProxyAuthRequired: {"Proxy Auth Required"},
	// OAuth 2.0 also implements RFC#6749, so we need to cater for specific BadRequest errors.
	// https://tools.ietf.org/html/rfc6749#section-5.2
	http.StatusBadRequest: {"Bad Request"},
}

// IsNotFound reports if given error is of 'not found' type.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var gerr *googleapi.Error
	if ok := errors.As(err, &gerr); ok {
		return gerr.Code == http.StatusNotFound
	}
	return errors.Is(err, errors.NotFound)
}
