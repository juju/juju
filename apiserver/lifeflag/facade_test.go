// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type FacadeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FacadeSuite{})

func (*FacadeSuite) TestFatal(c *gc.C) {
	c.Fatalf("xxx")
}
