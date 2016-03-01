// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type FacadeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FacadeSuite{})

func (s *FacadeSuite) TestFatal(c *gc.C) {
	c.Fatalf("xxx")
}
