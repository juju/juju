// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package life_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type LifeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&LifeSuite{})

func (*LifeSuite) TestFatal(c *gc.C) {
	c.Fatalf("xxx")
}
