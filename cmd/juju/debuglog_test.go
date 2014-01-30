// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
)

type DebugLogSuite struct {
}

var _ = gc.Suite(&DebugLogSuite{})

func (s *DebugLogSuite) TestDebugLog(c *gc.C) {
}
