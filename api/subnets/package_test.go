// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

// TestAll is the main test function for this package
func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
