// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
)

func MakeEnvironForTest(c *gc.C) environs.Environ {
	return makeEnviron(c)
}
