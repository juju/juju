// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	stdtesting "testing"

	"github.com/juju/testing"

	gc "gopkg.in/check.v1"
)

func TestPackage(t *stdtesting.T) {
	if testing.RaceEnabled {
		t.Skip("skipping package under -race, see LP 1518806")
	}
	gc.TestingT(t)
}
