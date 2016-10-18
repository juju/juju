// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"github.com/juju/errors"
)

var (
	ErrNotBootstrapped  = errors.New("model is not bootstrapped")
	ErrNoInstances      = errors.NotFoundf("instances")
	ErrPartialInstances = errors.New("only some instances were found")
)
