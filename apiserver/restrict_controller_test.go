// Copyright 2016 Canonical Ltd.
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

type restrictControllerSuite struct {
	testing.BaseSuite
	root rpc.Root
}

var _ = gc.Suite(&restrictControllerSuite{})

func (s *restrictControllerSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.root = apiserver.TestingControllerOnlyRoot()
}

func (s *restrictControllerSuite) TestAllowed(c *gc.C) {
	s.assertMethod(c, "AllModelWatcher", 2, "Next")
	s.assertMethod(c, "AllModelWatcher", 2, "Stop")
	s.assertMethod(c, "ModelManager", 2, "CreateModel")
	s.assertMethod(c, "ModelManager", 2, "ListModels")
	s.assertMethod(c, "Pinger", 1, "Ping")
	s.assertMethod(c, "Bundle", 1, "GetChanges")
	s.assertMethod(c, "HighAvailability", 2, "EnableHA")
	s.assertMethod(c, "ApplicationOffers", 1, "ApplicationOffers")
}

func (s *restrictControllerSuite) TestNotAllowed(c *gc.C) {
	caller, err := s.root.FindMethod("Client", 1, "FullStatus")
	c.Assert(err, gc.ErrorMatches, `facade "Client" not supported for controller API connection`)
	c.Assert(errors.IsNotSupported(err), jc.IsTrue)
	c.Assert(caller, gc.IsNil)
}

func (s *restrictControllerSuite) assertMethod(c *gc.C, facadeName string, version int, method string) {
	caller, err := s.root.FindMethod(facadeName, version, method)
	c.Check(err, jc.ErrorIsNil)
	c.Check(caller, gc.NotNil)
}
