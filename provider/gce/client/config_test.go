// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/gce/client"
)

type configSuite struct {
	client.BaseSuite
}

var _ = gc.Suite(&configSuite{})

func (*configSuite) TestValidateAuth(c *gc.C) {
	auth := client.Auth{
		ClientID:    "spam",
		ClientEmail: "user@mail.com",
		PrivateKey:  []byte("non-empty"),
	}
	err := client.ValidateAuth(auth)

	c.Check(err, jc.ErrorIsNil)
}

func (*configSuite) TestValidateAuthMissingID(c *gc.C) {
	auth := client.Auth{
		ClientEmail: "user@mail.com",
		PrivateKey:  []byte("non-empty"),
	}
	err := client.ValidateAuth(auth)

	c.Assert(err, gc.FitsTypeOf, &config.InvalidConfigValue{})
	c.Check(err.(*config.InvalidConfigValue).Key, gc.Equals, "GCE_CLIENT_ID")
}

func (*configSuite) TestValidateAuthBadEmail(c *gc.C) {
	auth := client.Auth{
		ClientID:    "spam",
		ClientEmail: "bad_email",
		PrivateKey:  []byte("non-empty"),
	}
	err := client.ValidateAuth(auth)

	c.Assert(err, gc.FitsTypeOf, &config.InvalidConfigValue{})
	c.Check(err.(*config.InvalidConfigValue).Key, gc.Equals, "GCE_CLIENT_EMAIL")
}

func (*configSuite) TestValidateAuthMissingKey(c *gc.C) {
	auth := client.Auth{
		ClientID:    "spam",
		ClientEmail: "user@mail.com",
	}
	err := client.ValidateAuth(auth)

	c.Assert(err, gc.FitsTypeOf, &config.InvalidConfigValue{})
	c.Check(err.(*config.InvalidConfigValue).Key, gc.Equals, "GCE_PRIVATE_KEY")
}
