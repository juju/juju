// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/apiserver"
	"github.com/juju/juju/v2/rpc"
	"github.com/juju/juju/v2/testing"
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
	s.assertMethod(c, "Client", 1, "FullStatus")
	s.assertMethod(c, "Pinger", 1, "Ping")
	s.assertMethod(c, "HighAvailability", 2, "EnableHA")
}

func (s *restrictModelSuite) TestBlocked(c *gc.C) {
	caller, err := s.root.FindMethod("ModelManager", 2, "ListModels")
	c.Assert(err, gc.ErrorMatches, `facade "ModelManager" not supported for model API connection`)
	c.Assert(errors.IsNotSupported(err), jc.IsTrue)
	c.Assert(caller, gc.IsNil)
}

func (s *restrictModelSuite) assertMethod(c *gc.C, facadeName string, version int, method string) {
	caller, err := s.root.FindMethod(facadeName, version, method)
	c.Check(err, jc.ErrorIsNil)
	c.Check(caller, gc.NotNil)
}
