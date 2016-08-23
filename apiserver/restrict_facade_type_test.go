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

type restrictFacadeTypeSuite struct {
	testing.BaseSuite
	controllerRoot rpc.Root
	modelRoot      rpc.Root
}

var _ = gc.Suite(&restrictFacadeTypeSuite{})

func (r *restrictFacadeTypeSuite) SetUpSuite(c *gc.C) {
	r.BaseSuite.SetUpSuite(c)
	r.controllerRoot = apiserver.TestingControllerOnlyRoot(nil)
	r.modelRoot = apiserver.TestingModelOnlyRoot(nil)
}

func (r *restrictFacadeTypeSuite) TestAllowedControllerMethods(c *gc.C) {
	r.assertMethod(c, r.controllerRoot, "AllModelWatcher", 2, "Next")
	r.assertMethod(c, r.controllerRoot, "AllModelWatcher", 2, "Stop")
	r.assertMethod(c, r.controllerRoot, "ModelManager", 2, "CreateModel")
	r.assertMethod(c, r.controllerRoot, "ModelManager", 2, "ListModels")
	r.assertMethod(c, r.controllerRoot, "Pinger", 1, "Ping")
}

func (r *restrictFacadeTypeSuite) TestAllowedModelMethod(c *gc.C) {
	r.assertMethod(c, r.modelRoot, "Client", 1, "FullStatus")
}

func (r *restrictFacadeTypeSuite) TestBlockedControllerMethod(c *gc.C) {
	caller, err := r.controllerRoot.FindMethod("Client", 1, "FullStatus")
	c.Assert(err, gc.ErrorMatches, `facade "Client" not supported for controller API connection`)
	c.Assert(errors.IsNotSupported(err), jc.IsTrue)
	c.Assert(caller, gc.IsNil)
}

func (r *restrictFacadeTypeSuite) TestBlockedModelMethod(c *gc.C) {
	caller, err := r.modelRoot.FindMethod("ModelManager", 2, "ListModels")
	c.Assert(err, gc.ErrorMatches, `facade "ModelManager" not supported for model API connection`)
	c.Assert(errors.IsNotSupported(err), jc.IsTrue)
	c.Assert(caller, gc.IsNil)
}

func (r *restrictFacadeTypeSuite) assertMethod(c *gc.C, root rpc.Root, facadeName string, version int, method string) {
	caller, err := root.FindMethod(facadeName, version, method)
	c.Check(err, jc.ErrorIsNil)
	c.Check(caller, gc.NotNil)
}
