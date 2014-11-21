// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type DeployTest struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&DeployTest{})

func (s *DeployTest) TestFatal(c *gc.C) {
	c.Fatalf("XXX")
}
