// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	stdtesting "testing"

	"github.com/juju/juju/v3/testing"
)

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
