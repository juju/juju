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

type restrictAnonymousSuite struct {
	testing.BaseSuite
	root rpc.Root
}

func TestRestrictAnonymousSuite(t *stdtesting.T) {
	tc.Run(t, &restrictAnonymousSuite{})
}

func (s *restrictAnonymousSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.root = apiserver.TestingAnonymousRoot()
}

func (s *restrictAnonymousSuite) TestNotAllowed(c *tc.C) {
	caller, err := s.root.FindMethod("Client", clientFacadeVersion, "FullStatus")
	c.Assert(err, tc.ErrorMatches, `facade "Client" not supported for anonymous API connections`)
	c.Assert(err, tc.ErrorIs, errors.NotSupported)
	c.Assert(caller, tc.IsNil)
}
