// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"github.com/juju/errors"
)

var (
	ErrNotBootstrapped     = errors.New("environment is not bootstrapped")
	ErrAlreadyBootstrapped = errors.New("environment is already bootstrapped")
	ErrNoInstances         = errors.NotFoundf("instances")
	ErrPartialInstances    = errors.New("only some instances were found")

	// Errors indicating that the provider can't allocate an IP address to an
	// instance.
	ErrIPAddressesExhausted = errors.New("can't allocate a new IP address")
	ErrIPAddressUnavailable = errors.New("the requested IP address is unavailable")
)
