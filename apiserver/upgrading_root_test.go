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

func (r *upgradingRootSuite) TestAllowedMethods(c *gc.C) {
	root := apiserver.TestingUpgradingRoot(nil)
	checkAllowed := func(facade, method string) {
		caller, err := root.FindMethod(facade, 1, method)
		c.Check(err, jc.ErrorIsNil)
		c.Check(caller, gc.NotNil)
	}
	checkAllowed("Client", "FullStatus")
	checkAllowed("Client", "AbortCurrentUpgrade")
	checkAllowed("SSHClient", "PublicAddress")
	checkAllowed("SSHClient", "Proxy")
	checkAllowed("Pinger", "Ping")
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
