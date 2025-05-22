// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type restrictUpgradesSuite struct {
	testing.BaseSuite
}

func TestRestrictUpgradesSuite(t *stdtesting.T) {
	tc.Run(t, &restrictUpgradesSuite{})
}

func (r *restrictUpgradesSuite) TestAllowedMethods(c *tc.C) {
	root := apiserver.TestingUpgradingRoot()
	checkAllowed := func(facade, method string, version int) {
		caller, err := root.FindMethod(facade, version, method)
		c.Check(err, tc.ErrorIsNil)
		c.Check(caller, tc.NotNil)
	}
	checkAllowed("Client", "FullStatus", clientFacadeVersion)
	checkAllowed("SSHClient", "PublicAddress", sshClientFacadeVersion)
	checkAllowed("SSHClient", "Proxy", sshClientFacadeVersion)
	checkAllowed("Pinger", "Ping", pingerFacadeVersion)
}

func (r *restrictUpgradesSuite) TestFindDisallowedMethod(c *tc.C) {
	root := apiserver.TestingUpgradingRoot()
	caller, err := root.FindMethod("Client", clientFacadeVersion, "ModelSet")
	c.Assert(errors.Cause(err), tc.Equals, params.UpgradeInProgressError)
	c.Assert(caller, tc.IsNil)
}
