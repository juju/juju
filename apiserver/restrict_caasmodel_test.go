// Copyright 2017 Canonical Ltd.
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

type RestrictCAASModelSuite struct {
	testing.BaseSuite
	root rpc.Root
}

func TestRestrictCAASModelSuite(t *stdtesting.T) {
	tc.Run(t, &RestrictCAASModelSuite{})
}

func (s *RestrictCAASModelSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.root = apiserver.TestingCAASModelOnlyRoot()
}

func (s *RestrictCAASModelSuite) TestAllowed(c *tc.C) {
	s.assertMethod(c, "CAASModelOperator", 1, "ModelOperatorProvisioningInfo")
}

func (s *RestrictCAASModelSuite) TestSubnetsAllowed(c *tc.C) {
	s.assertMethod(c, "Subnets", 5, "ListSubnets")
}

func (s *RestrictCAASModelSuite) TestSpacesReloadAllowed(c *tc.C) {
	s.assertMethod(c, "Spaces", 6, "ReloadSpaces")
}

func (s *RestrictCAASModelSuite) TestSpacesReadMethodsAllowed(c *tc.C) {
	s.assertMethod(c, "Spaces", 6, "ListSpaces")
	s.assertMethod(c, "Spaces", 6, "ShowSpace")
}

func (s *RestrictCAASModelSuite) TestSpacesMutationMethodsNotAllowed(c *tc.C) {
	for _, method := range []string{
		"CreateSpaces",
		"MoveSubnets",
		"RemoveSpace",
		"RenameSpace",
	} {
		caller, err := s.root.FindMethod("Spaces", 6, method)
		c.Check(err, tc.ErrorMatches, `facade method "Spaces\.`+method+`" not supported on container models`)
		c.Check(err, tc.ErrorIs, errors.NotSupported)
		c.Check(caller, tc.IsNil)
	}
}

func (s *RestrictCAASModelSuite) TestNotAllowed(c *tc.C) {
	caller, err := s.root.FindMethod("Firewaller", 1, "WatchOpenedPorts")
	c.Assert(err, tc.ErrorMatches, `facade "Firewaller" not supported on container models`)
	c.Assert(err, tc.ErrorIs, errors.NotSupported)
	c.Assert(caller, tc.IsNil)
}

func (s *RestrictCAASModelSuite) assertMethod(c *tc.C, facadeName string, version int, method string) {
	caller, err := s.root.FindMethod(facadeName, version, method)
	c.Check(err, tc.ErrorIsNil)
	c.Check(caller, tc.NotNil)
}
