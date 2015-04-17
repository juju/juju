// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type authSuite struct {
	BaseSuite
}

var _ = gc.Suite(&authSuite{})

func (s *authSuite) TestNewConnection(c *gc.C) {
	_, err := newConnection(s.Credentials)
	c.Assert(err, jc.ErrorIsNil)
}
