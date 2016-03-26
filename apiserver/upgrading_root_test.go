// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/testing"
)

type upgradingRootSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&upgradingRootSuite{})

func (r *upgradingRootSuite) TestClientMethods(c *gc.C) {
	root := apiserver.TestingUpgradingRoot(nil)

	for facadeName, methods := range apiserver.AllowedMethodsDuringUpgrades {
		for _, method := range methods.Values() {
			// for now all of the api calls of interest,
			// reside on version 1 of their respective facade.
			caller, err := root.FindMethod(facadeName, 1, method)
			c.Check(err, jc.ErrorIsNil)
			c.Check(caller, gc.NotNil)
		}
	}
}

func (r *upgradingRootSuite) TestFindDisallowedMethod(c *gc.C) {
	root := apiserver.TestingUpgradingRoot(nil)

	caller, err := root.FindMethod("Client", 1, "ModelSet")

	c.Assert(errors.Cause(err), gc.Equals, params.UpgradeInProgressError)
	c.Assert(caller, gc.IsNil)
}

func (r *upgradingRootSuite) TestFindNonExistentMethod(c *gc.C) {
	root := apiserver.TestingUpgradingRoot(nil)

	caller, err := root.FindMethod("Foo", 0, "Bar")

	c.Assert(err, gc.ErrorMatches, "unknown object type \"Foo\"")
	c.Assert(caller, gc.IsNil)
}

func (r *upgradingRootSuite) TestFindMethodNonExistentVersion(c *gc.C) {
	root := apiserver.TestingUpgradingRoot(nil)

	caller, err := root.FindMethod("Client", 99999999, "FullStatus")

	c.Assert(err, gc.ErrorMatches, "unknown version \\(99999999\\) of interface \"Client\"")
	c.Assert(caller, gc.IsNil)
}
