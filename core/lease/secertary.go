// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"time"
)

// Secretary is responsible for validating the sanity of lease and holder names
// before bothering the manager with them.
type Secretary interface {

	// CheckLease returns an error if the supplied lease name is not valid.
	CheckLease(key Key) error

	// CheckHolder returns an error if the supplied holder name is not valid.
	CheckHolder(name string) error

	// CheckDuration returns an error if the supplied duration is not valid.
	CheckDuration(duration time.Duration) error
}

// SecretaryFinder is responsible for returning the correct Secretary for a
// given namespace.
type SecretaryFinder interface {
	// SecretaryFor returns the Secretary for the given namespace.
	// Returns an error if the namespace is not valid.
	SecretaryFor(namespace string) (Secretary, error)
}
