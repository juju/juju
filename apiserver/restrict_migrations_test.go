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

type restrictMigrationsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&restrictMigrationsSuite{})

func (r *restrictMigrationsSuite) TestAllowedMethods(c *gc.C) {
	root := apiserver.TestingMigratingRoot()
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

func (r *restrictMigrationsSuite) TestFindDisallowedMethod(c *gc.C) {
	root := apiserver.TestingMigratingRoot()
	caller, err := root.FindMethod("Client", clientFacadeVersion, "ModelSet")
	c.Assert(errors.Cause(err), gc.Equals, params.MigrationInProgressError)
	c.Assert(caller, gc.IsNil)
}
