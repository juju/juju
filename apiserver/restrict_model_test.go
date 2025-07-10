// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc"
)

type restrictModelSuite struct {
	testing.BaseSuite
	root rpc.Root
}

func TestRestrictModelSuite(t *stdtesting.T) {
	tc.Run(t, &restrictModelSuite{})
}

func (s *restrictModelSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.root = apiserver.TestingModelOnlyRoot()
}

func (s *restrictModelSuite) TestAllowed(c *tc.C) {
	s.assertMethod(c, "Client", clientFacadeVersion, "FullStatus")
	s.assertMethod(c, "Pinger", pingerFacadeVersion, "Ping")
}

func (s *restrictModelSuite) TestBlocked(c *tc.C) {
	caller, err := s.root.FindMethod("ModelManager", modelManagerFacadeVersion, "ListModels")
	c.Assert(err, tc.ErrorMatches, `facade "ModelManager" not supported for model API connection`)
	c.Assert(err, tc.ErrorIs, errors.NotSupported)
	c.Assert(caller, tc.IsNil)
}

func (s *restrictModelSuite) assertMethod(c *tc.C, facadeName string, version int, method string) {
	caller, err := s.root.FindMethod(facadeName, version, method)
	c.Check(err, tc.ErrorIsNil)
	c.Check(caller, tc.NotNil)
}
