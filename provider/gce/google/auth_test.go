// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"context"

	jujuhttp "github.com/juju/http/v2"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type authSuite struct {
	Credentials *Credentials
}

var _ = gc.Suite(&authSuite{})

func (s *authSuite) SetUpTest(c *gc.C) {
	s.Credentials = &Credentials{
		ClientID:    "spam",
		ClientEmail: "user@mail.com",
		PrivateKey:  []byte("<some-key>"),
		JSONKey: []byte(`
{
    "private_key_id": "mnopq",
    "private_key": "<some-key>",
    "client_email": "user@mail.com",
    "client_id": "spam",
    "type": "service_account"
}`[1:]),
	}
}

func (s *authSuite) TestNewComputeService(c *gc.C) {
	_, err := newComputeService(context.TODO(), s.Credentials, jujuhttp.NewClient())
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
