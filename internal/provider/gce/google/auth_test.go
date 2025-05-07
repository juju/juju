// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"context"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	jujuhttp "github.com/juju/juju/internal/http"
)

type authSuite struct {
	BaseSuite
}

var _ = tc.Suite(&authSuite{})

func (s *authSuite) TestNewComputeService(c *tc.C) {
	_, err := newComputeService(context.Background(), s.Credentials, jujuhttp.NewClient())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *authSuite) TestCreateJWTConfig(c *tc.C) {
	cfg, err := newJWTConfig(s.Credentials)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Scopes, jc.DeepEquals, scopes)
}

func (s *authSuite) TestCreateJWTConfigWithNoJSONKey(c *tc.C) {
	cfg, err := newJWTConfig(&Credentials{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Scopes, jc.DeepEquals, scopes)
}
