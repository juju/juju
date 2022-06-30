// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
)

type restrictUpgradesSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&restrictUpgradesSuite{})

func (r *restrictUpgradesSuite) TestAllowedMethods(c *gc.C) {
	root := apiserver.TestingUpgradingRoot()
	checkAllowed := func(facade, method string, version int) {
		caller, err := root.FindMethod(facade, version, method)
		c.Check(err, jc.ErrorIsNil)
		c.Check(caller, gc.NotNil)
	}
	checkAllowed("Client", "FullStatus", clientFacadeVersion)
	checkAllowed("SSHClient", "PublicAddress", sshClientFacadeVersion)
	checkAllowed("SSHClient", "Proxy", sshClientFacadeVersion)
	checkAllowed("Pinger", "Ping", pingerFacadeVersion)
}

func (r *restrictUpgradesSuite) TestFindDisallowedMethod(c *gc.C) {
	root := apiserver.TestingUpgradingRoot()
	caller, err := root.FindMethod("Client", clientFacadeVersion, "ModelSet")
	c.Assert(errors.Cause(err), gc.Equals, params.UpgradeInProgressError)
	c.Assert(caller, gc.IsNil)
}
