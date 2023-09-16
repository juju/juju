// Copyright 2017 Canonical Ltd.
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

type RestrictCAASModelSuite struct {
	testing.BaseSuite
	root rpc.Root
}

var _ = gc.Suite(&RestrictCAASModelSuite{})

func (s *RestrictCAASModelSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.root = apiserver.TestingCAASModelOnlyRoot()
}

func (s *RestrictCAASModelSuite) TestAllowed(c *gc.C) {
	s.assertMethod(c, "CAASUnitProvisioner", 2, "WatchApplicationsScale")
}

func (s *RestrictCAASModelSuite) TestNotAllowed(c *gc.C) {
	caller, err := s.root.FindMethod("Firewaller", 1, "WatchOpenedPorts")
	c.Assert(err, gc.ErrorMatches, `facade "Firewaller" not supported on container models`)
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
	c.Assert(caller, gc.IsNil)
}

func (s *RestrictCAASModelSuite) assertMethod(c *gc.C, facadeName string, version int, method string) {
	caller, err := s.root.FindMethod(facadeName, version, method)
	c.Check(err, jc.ErrorIsNil)
	c.Check(caller, gc.NotNil)
}
