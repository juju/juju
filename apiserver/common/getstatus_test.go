// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type statusGetterSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&statusGetterSuite{})

func (s *statusGetterSuite) TestFatal(c *gc.C) {
	c.Fatalf("not done")
}
