// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"context"

	jujuhttp "github.com/juju/juju/internal/http"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type authSuite struct {
	BaseSuite
}

var _ = gc.Suite(&authSuite{})

func (s *authSuite) TestNewComputeService(c *gc.C) {
	_, err := newComputeService(context.Background(), s.Credentials, jujuhttp.NewClient())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *authSuite) TestCreateJWTConfig(c *gc.C) {
	cfg, err := newJWTConfig(s.Credentials)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Scopes, jc.DeepEquals, scopes)
}

func (s *authSuite) TestCreateJWTConfigWithNoJSONKey(c *gc.C) {
	cfg, err := newJWTConfig(&Credentials{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Scopes, jc.DeepEquals, scopes)
}
