// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import "github.com/juju/errors"

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

// IsCredentialNotValid reports whether err was created with CredentialNotValid().
func IsCredentialNotValid(err error) bool {
	err = errors.Cause(err)
	_, ok := err.(*credentialNotValid)
	return ok
}
