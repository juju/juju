// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package factory_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

// TestPackage integrates the tests into gotest.
func Test(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
