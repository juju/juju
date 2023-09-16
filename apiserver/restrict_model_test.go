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

type restrictModelSuite struct {
	testing.BaseSuite
	root rpc.Root
}

var _ = gc.Suite(&restrictModelSuite{})

func (s *restrictModelSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.root = apiserver.TestingModelOnlyRoot()
}

func (s *restrictModelSuite) TestAllowed(c *gc.C) {
	s.assertMethod(c, "Client", clientFacadeVersion, "FullStatus")
	s.assertMethod(c, "Pinger", pingerFacadeVersion, "Ping")
	s.assertMethod(c, "HighAvailability", highAvailabilityFacadeVersion, "EnableHA")
}

func (s *restrictModelSuite) TestBlocked(c *gc.C) {
	caller, err := s.root.FindMethod("ModelManager", modelManagerFacadeVersion, "ListModels")
	c.Assert(err, gc.ErrorMatches, `facade "ModelManager" not supported for model API connection`)
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
	c.Assert(caller, gc.IsNil)
}

func (s *restrictModelSuite) assertMethod(c *gc.C, facadeName string, version int, method string) {
	caller, err := s.root.FindMethod(facadeName, version, method)
	c.Check(err, jc.ErrorIsNil)
	c.Check(caller, gc.NotNil)
}
