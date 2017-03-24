// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"github.com/juju/juju/apiserver"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type AllFacadesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&AllFacadesSuite{})

func (s *AllFacadesSuite) TestNoPanic(c *gc.C) {
	// AllFacades will panic on error so check it by calling it.
	r := apiserver.AllFacades()
	c.Assert(r, gc.NotNil)
}
