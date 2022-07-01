// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	stdtesting "testing"

	"github.com/juju/juju/v2/testing"
)

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
