// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	stdtesting "testing"

	"github.com/juju/juju/v2/testing"
)

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
