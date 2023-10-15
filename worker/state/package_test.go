// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
