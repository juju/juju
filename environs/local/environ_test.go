// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	. "launchpad.net/gocheck"
)

type EnvironSuite struct {
	MockLxcSuite
}

var _ = Suite(new(EnvironSuite))

func (*EnvironSuite) TestName(c *C) {
}
