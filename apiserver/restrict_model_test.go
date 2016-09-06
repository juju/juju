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
	caller, err := s.root.FindMethod("Client", 1, "FullStatus")
	c.Check(err, jc.ErrorIsNil)
	c.Check(caller, gc.NotNil)
}

func (s *restrictModelSuite) TestBlocked(c *gc.C) {
	caller, err := s.root.FindMethod("ModelManager", 2, "ListModels")
	c.Assert(err, gc.ErrorMatches, `facade "ModelManager" not supported for model API connection`)
	c.Assert(errors.IsNotSupported(err), jc.IsTrue)
	c.Assert(caller, gc.IsNil)
}
