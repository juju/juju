// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

// StartInstanceError is an error type that may be returned from providers'
// StartInstance methods to include more details of the error.
type StartInstanceError struct {
	// Err is the underlying error that caused StartInstance to fail.
	Err error

	// ZoneIndependent, if true, indicates that the error is due to some
	// condition that holds regardless of the availability zone specified.
	ZoneIndependent bool
}

// Error is part of the error interface.
func (err *StartInstanceError) Error() string {
	return err.Err.Error()
}

// AvailabilityZoneIndependent is part of the environs.AvailabilityZoneError
// interface.
func (err *StartInstanceError) AvailabilityZoneIndependent() bool {
	return err.ZoneIndependent
}
