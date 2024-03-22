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

type restrictAnonymousSuite struct {
	testing.BaseSuite
	root rpc.Root
}

var _ = gc.Suite(&restrictAnonymousSuite{})

func (s *restrictAnonymousSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.root = apiserver.TestingAnonymousRoot()
}

func (s *restrictAnonymousSuite) TestNotAllowed(c *gc.C) {
	caller, err := s.root.FindMethod("Client", clientFacadeVersion, "FullStatus")
	c.Assert(err, gc.ErrorMatches, `facade "Client" not supported for anonymous API connections`)
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
	c.Assert(caller, gc.IsNil)
}
