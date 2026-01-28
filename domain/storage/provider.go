// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"regexp"
)

// ProviderType is a unique identification type to represent individual storage
// providers within a controller by common value.
type ProviderType string

const (
	// maxProviderTypeLegnth is the maximum length a provider type can be. This
	// conforms to RFC1035 label rules. See [validProviderTypeRegex].
	maxProviderTypeLegnth = 63

	// minProviderTypeLegnth is the minimum length a provider type can be. This
	// conforms to RFC1035 label rules. See [validProviderTypeRegex].
	minProviderTypeLegnth = 1
)

var (
	// validProviderTypeRegex is a regular expression that checks if a provider
	// type value is valid. This may seem odd as provider types are controlled
	// within source code. However we ask users to supply these values via
	// multiple interfaces. We need a safe way of validating what they have
	// supplied before using it any further.
	//
	// The rules for a valid provider type must follow RFC1035 label rules.
	validProviderTypeRegex = regexp.MustCompile("^[[:alpha:]][[:alnum:]-]{0,61}[[:alnum:]]$|^[[:alpha:]]$")
)

// IsValid exists to validate a supplied provider type value from an untrusted
// source before using any further.
//
// The rules for a valid provider type must follow RFC1035 label rules.
func (p ProviderType) IsValid() bool {
	str := p.String()
	// Check length first before regex as it is a quicker failure path.
	return len(str) >= minProviderTypeLegnth &&
		len(str) <= maxProviderTypeLegnth &&
		validProviderTypeRegex.MatchString(str)
}

// String returns the string representation of [ProviderType]. String implements
// the [fmt.Stringer] interface.
func (p ProviderType) String() string {
	return string(p)
}
