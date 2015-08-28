// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/testing"
)

type restrictedRootSuite struct {
	testing.BaseSuite

	root rpc.MethodFinder
}

var _ = gc.Suite(&restrictedRootSuite{})

func (r *restrictedRootSuite) SetUpTest(c *gc.C) {
	r.BaseSuite.SetUpTest(c)
	r.SetFeatureFlags(feature.JES)
	r.root = apiserver.TestingRestrictedApiHandler(nil)
}

func (r *restrictedRootSuite) assertMethodAllowed(c *gc.C, rootName string, version int, method string) {
	caller, err := r.root.FindMethod(rootName, version, method)
	c.Check(err, jc.ErrorIsNil)
	c.Check(caller, gc.NotNil)
}

func (r *restrictedRootSuite) TestFindAllowedMethod(c *gc.C) {
	r.assertMethodAllowed(c, "AllEnvWatcher", 1, "Next")
	r.assertMethodAllowed(c, "AllEnvWatcher", 1, "Stop")

	r.assertMethodAllowed(c, "EnvironmentManager", 1, "CreateEnvironment")
	r.assertMethodAllowed(c, "EnvironmentManager", 1, "ListEnvironments")

	r.assertMethodAllowed(c, "UserManager", 0, "AddUser")
	r.assertMethodAllowed(c, "UserManager", 0, "SetPassword")
	r.assertMethodAllowed(c, "UserManager", 0, "UserInfo")

	r.assertMethodAllowed(c, "SystemManager", 1, "AllEnvironments")
	r.assertMethodAllowed(c, "SystemManager", 1, "DestroySystem")
	r.assertMethodAllowed(c, "SystemManager", 1, "EnvironmentConfig")
	r.assertMethodAllowed(c, "SystemManager", 1, "ListBlockedEnvironments")
}

func (r *restrictedRootSuite) TestFindDisallowedMethod(c *gc.C) {
	caller, err := r.root.FindMethod("Client", 0, "Status")

	c.Assert(err, gc.ErrorMatches, `logged in to server, no environment, "Client" not supported`)
	c.Assert(errors.IsNotSupported(err), jc.IsTrue)
	c.Assert(caller, gc.IsNil)
}

func (r *restrictedRootSuite) TestNonExistentFacade(c *gc.C) {
	caller, err := r.root.FindMethod("NonExistent", 0, "Method")

	c.Assert(err, gc.ErrorMatches, `unknown object type "NonExistent"`)
	c.Assert(caller, gc.IsNil)
}

func (r *restrictedRootSuite) TestFindNonExistentMethod(c *gc.C) {
	caller, err := r.root.FindMethod("EnvironmentManager", 1, "Bar")

	c.Assert(err, gc.ErrorMatches, `no such request - method EnvironmentManager\(1\).Bar is not implemented`)
	c.Assert(caller, gc.IsNil)
}

func (r *restrictedRootSuite) TestFindMethodNonExistentVersion(c *gc.C) {
	caller, err := r.root.FindMethod("UserManager", 99999999, "AddUser")

	c.Assert(err, gc.ErrorMatches, `unknown version \(99999999\) of interface "UserManager"`)
	c.Assert(caller, gc.IsNil)
}
