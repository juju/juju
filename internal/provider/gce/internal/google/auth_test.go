// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"context"
	stdtesting "testing"

	compute "cloud.google.com/go/compute/apiv1"
	"github.com/juju/tc"

	jujuhttp "github.com/juju/juju/internal/http"
)

type authSuite struct {
	Credentials *Credentials
}

func TestAuthSuite(t *stdtesting.T) {
	tc.Run(t, &authSuite{})
}

func (s *authSuite) SetUpTest(c *tc.C) {
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

func (s *authSuite) TestNewRESTClient(c *tc.C) {
	_, err := newRESTClient(context.TODO(), s.Credentials, jujuhttp.NewClient(), compute.NewNetworksRESTClient)
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
