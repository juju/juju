// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/testing"
)

type restrictCAASModelSuite struct {
	testing.BaseSuite
	root rpc.Root
}

var _ = gc.Suite(&restrictCAASModelSuite{})

func (s *restrictCAASModelSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.SetFeatureFlags(feature.CAAS)
	s.root = apiserver.TestingCAASModelOnlyRoot()
}

func (s *restrictCAASModelSuite) TestAllowed(c *gc.C) {
	s.assertMethod(c, "CAASProvisioner", 1, "ConnectionConfig")
}

func (s *restrictCAASModelSuite) TestNotAllowed(c *gc.C) {
	caller, err := s.root.FindMethod("Firewaller", 1, "WatchOpenedPorts")
	c.Assert(err, gc.ErrorMatches, `facade "Firewaller" not supported for a CAAS model API connection`)
	c.Assert(errors.IsNotSupported(err), jc.IsTrue)
	c.Assert(caller, gc.IsNil)
}

func (s *restrictCAASModelSuite) assertMethod(c *gc.C, facadeName string, version int, method string) {
	caller, err := s.root.FindMethod(facadeName, version, method)
	c.Check(err, jc.ErrorIsNil)
	c.Check(caller, gc.NotNil)
}
