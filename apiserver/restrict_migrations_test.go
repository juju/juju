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

type restrictMigrationsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&restrictMigrationsSuite{})

func (r *restrictMigrationsSuite) TestAllowedMethods(c *gc.C) {
	root := apiserver.TestingMigratingRoot()
	checkAllowed := func(facade, method string) {
		caller, err := root.FindMethod(facade, 1, method)
		c.Check(err, jc.ErrorIsNil)
		c.Check(caller, gc.NotNil)
	}
	checkAllowed("Client", "FullStatus")
	checkAllowed("SSHClient", "PublicAddress")
	checkAllowed("SSHClient", "Proxy")
	checkAllowed("Pinger", "Ping")
}

func (r *restrictMigrationsSuite) TestFindDisallowedMethod(c *gc.C) {
	root := apiserver.TestingMigratingRoot()
	caller, err := root.FindMethod("Client", 1, "ModelSet")
	c.Assert(errors.Cause(err), gc.Equals, params.MigrationInProgressError)
	c.Assert(caller, gc.IsNil)
}
