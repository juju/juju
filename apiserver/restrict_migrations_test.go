// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type restrictMigrationsSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&restrictMigrationsSuite{})

func (r *restrictMigrationsSuite) TestAllowedMethods(c *tc.C) {
	root := apiserver.TestingMigratingRoot()
	checkAllowed := func(facade, method string, version int) {
		caller, err := root.FindMethod(facade, version, method)
		c.Check(err, jc.ErrorIsNil)
		c.Check(caller, tc.NotNil)
	}
	checkAllowed("Client", "FullStatus", clientFacadeVersion)
	checkAllowed("SSHClient", "PublicAddress", sshClientFacadeVersion)
	checkAllowed("SSHClient", "Proxy", sshClientFacadeVersion)
	checkAllowed("Pinger", "Ping", pingerFacadeVersion)
}

func (r *restrictMigrationsSuite) TestFindDisallowedMethod(c *tc.C) {
	root := apiserver.TestingMigratingRoot()
	caller, err := root.FindMethod("Client", clientFacadeVersion, "ModelSet")
	c.Assert(errors.Cause(err), tc.Equals, params.MigrationInProgressError)
	c.Assert(caller, tc.IsNil)
}
