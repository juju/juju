// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"errors"
)

var (
	ErrNotBootstrapped  = errors.New("environment is not bootstrapped")
	ErrNoInstances      = errors.New("no instances found")
	ErrPartialInstances = errors.New("only some instances were found")
)
