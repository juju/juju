// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testing_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

func Test(t *stdtesting.T) {
	tc.TestingT(t)
}
