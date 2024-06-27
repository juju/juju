// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

// Name is a unique name given to a machine within a Juju controller.
type Name string

// Validate returns an error if the [Name] is invalid. The error returned
// satisfies [errors.NotValid].
//
// Currently no validation for name is performed. This allows us to add
// validation in future versions.
func (i Name) Validate() error {
	return nil
}

// String returns the [Name] as a string.
func (i Name) String() string {
	return string(i)
}
