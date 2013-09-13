// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit_test

import (
	"testing"

	gc "launchpad.net/gocheck"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type suite struct{}

var _ = gc.Suite(suite{})
