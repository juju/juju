// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package util

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

func Test(t *stdtesting.T) {
	tc.TestingT(t)
}
