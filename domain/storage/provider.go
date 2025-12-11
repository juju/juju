// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"regexp"
)

// ProviderType is a unique identification type to represent individual storage
// providers within a controller by common value.
type ProviderType string

var (
	// validProviderTypeRegex is a regular expression that checks if a provider
	// type value is valid. This may seem odd as provider types are controlled
	// within source code. However we ask users to supply these values via
	// multiple interfaces. We need a safe way of validating what they have
	// supplied before using it any further.
	//
	// The rules for a valid provider type are:
	// - MUST start with an upper case or lower case ASCII character.
	// - MUST be at least one character long.
	// - After the first character, ASCII alphasnumeric characters are allowed
	// including hyphens.
	// - MUST not be greater than 128 characters.
	validProviderTypeRegex = regexp.MustCompile("^[[:alpha:]][-[:alpha:][:digit:]]{0,127}$")
)

// IsValid exists to validate a supplied provider type value from an untrusted
// source before using any further.
//
// The rules for a valid provider type are:
// - MUST start with an upper case or lower case ASCII character.
// - MUST be at least one character long.
// - After the first character, ASCII alphasnumeric characters are allowed
// including hyphens.
// - MUST not be greater than 128 characters.
func (p ProviderType) IsValid() bool {
	return validProviderTypeRegex.MatchString(p.String())
}

// String returns the string representation of [ProviderType]. String implements
// the [fmt.Stringer] interface.
func (p ProviderType) String() string {
	return string(p)
}
