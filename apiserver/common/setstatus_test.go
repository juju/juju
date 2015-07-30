// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type statusSetterSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&statusSetterSuite{})

func (s *statusSetterSuite) TestFatal(c *gc.C) {
	c.Fatalf("not done")
}
