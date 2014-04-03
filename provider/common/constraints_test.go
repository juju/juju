// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
)

type ConstraintsSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&ConstraintsSuite{})
