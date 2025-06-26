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
