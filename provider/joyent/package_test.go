// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	stdtesting "testing"

	"github.com/juju/testing"

	gc "gopkg.in/check.v1"
)

func TestPackage(t *stdtesting.T) {
	if testing.RaceEnabled {
		t.Skip("skipping package under -race, see LP 1497801")
	}
	registerLocalTests()
	gc.TestingT(t)
}
