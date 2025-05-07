// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc"
)

type RestrictCAASModelSuite struct {
	testing.BaseSuite
	root rpc.Root
}

var _ = tc.Suite(&RestrictCAASModelSuite{})

func (s *RestrictCAASModelSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.root = apiserver.TestingCAASModelOnlyRoot()
}

func (s *RestrictCAASModelSuite) TestAllowed(c *tc.C) {
	s.assertMethod(c, "CAASUnitProvisioner", 2, "WatchApplicationsScale")
}

func (s *RestrictCAASModelSuite) TestNotAllowed(c *tc.C) {
	caller, err := s.root.FindMethod("Firewaller", 1, "WatchOpenedPorts")
	c.Assert(err, tc.ErrorMatches, `facade "Firewaller" not supported on container models`)
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
	c.Assert(caller, tc.IsNil)
}

func (s *RestrictCAASModelSuite) assertMethod(c *tc.C, facadeName string, version int, method string) {
	caller, err := s.root.FindMethod(facadeName, version, method)
	c.Check(err, jc.ErrorIsNil)
	c.Check(caller, tc.NotNil)
}
