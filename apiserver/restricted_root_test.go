// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
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
	r.root = apiserver.TestingRestrictedApiHandler(nil)
}

func (r *restrictedRootSuite) assertMethodAllowed(c *gc.C, rootName string, version int, method string) {
	caller, err := r.root.FindMethod(rootName, version, method)
	c.Check(err, jc.ErrorIsNil)
	c.Check(caller, gc.NotNil)
}

func (r *restrictedRootSuite) TestFindAllowedMethod(c *gc.C) {
	r.assertMethodAllowed(c, "AllModelWatcher", 2, "Next")
	r.assertMethodAllowed(c, "AllModelWatcher", 2, "Stop")

	r.assertMethodAllowed(c, "ModelManager", 2, "CreateModel")
	r.assertMethodAllowed(c, "ModelManager", 2, "ListModels")

	r.assertMethodAllowed(c, "UserManager", 1, "AddUser")
	r.assertMethodAllowed(c, "UserManager", 1, "SetPassword")
	r.assertMethodAllowed(c, "UserManager", 1, "UserInfo")

	r.assertMethodAllowed(c, "Controller", 2, "AllModels")
	r.assertMethodAllowed(c, "Controller", 2, "DestroyController")
	r.assertMethodAllowed(c, "Controller", 2, "ModelConfig")
	r.assertMethodAllowed(c, "Controller", 2, "ListBlockedModels")
}

func (r *restrictedRootSuite) TestFindDisallowedMethod(c *gc.C) {
	caller, err := r.root.FindMethod("Client", 1, "Status")

	c.Assert(err, gc.ErrorMatches, `logged in to server, no model, "Client" not supported`)
	c.Assert(errors.IsNotSupported(err), jc.IsTrue)
	c.Assert(caller, gc.IsNil)
}

func (r *restrictedRootSuite) TestNonExistentFacade(c *gc.C) {
	caller, err := r.root.FindMethod("SomeFacade", 0, "Method")

	c.Assert(err, gc.ErrorMatches, `logged in to server, no model, "SomeFacade" not supported`)
	c.Assert(caller, gc.IsNil)
}

func (r *restrictedRootSuite) TestFindNonExistentMethod(c *gc.C) {
	caller, err := r.root.FindMethod("ModelManager", 2, "Bar")

	c.Assert(err, gc.ErrorMatches, `no such request - method ModelManager\(2\).Bar is not implemented`)
	c.Assert(caller, gc.IsNil)
}

func (r *restrictedRootSuite) TestFindMethodNonExistentVersion(c *gc.C) {
	caller, err := r.root.FindMethod("UserManager", 99999999, "AddUser")

	c.Assert(err, gc.ErrorMatches, `unknown version \(99999999\) of interface "UserManager"`)
	c.Assert(caller, gc.IsNil)
}
