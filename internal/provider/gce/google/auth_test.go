// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"github.com/juju/tc"

	jujuhttp "github.com/juju/juju/internal/http"
)

type authSuite struct {
	BaseSuite
}

var _ = tc.Suite(&authSuite{})

func (s *authSuite) TestNewComputeService(c *tc.C) {
	_, err := newComputeService(c.Context(), s.Credentials, jujuhttp.NewClient())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *authSuite) TestCreateJWTConfig(c *tc.C) {
	cfg, err := newJWTConfig(s.Credentials)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.Scopes, tc.DeepEquals, scopes)
}

func (s *authSuite) TestCreateJWTConfigWithNoJSONKey(c *tc.C) {
	cfg, err := newJWTConfig(&Credentials{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.Scopes, tc.DeepEquals, scopes)
}
