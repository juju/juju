// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/gce/google"
)

type configSuite struct {
	google.BaseSuite
}

var _ = gc.Suite(&configSuite{})

func (*configSuite) TestValidateAuth(c *gc.C) {
	auth := google.Auth{
		ClientID:    "spam",
		ClientEmail: "user@mail.com",
		PrivateKey:  []byte("non-empty"),
	}
	err := google.ValidateAuth(auth)

	c.Check(err, jc.ErrorIsNil)
}

func (*configSuite) TestValidateAuthMissingID(c *gc.C) {
	auth := google.Auth{
		ClientEmail: "user@mail.com",
		PrivateKey:  []byte("non-empty"),
	}
	err := google.ValidateAuth(auth)

	c.Assert(err, gc.FitsTypeOf, &config.InvalidConfigValue{})
	c.Check(err.(*config.InvalidConfigValue).Key, gc.Equals, "GCE_CLIENT_ID")
}

func (*configSuite) TestValidateAuthBadEmail(c *gc.C) {
	auth := google.Auth{
		ClientID:    "spam",
		ClientEmail: "bad_email",
		PrivateKey:  []byte("non-empty"),
	}
	err := google.ValidateAuth(auth)

	c.Assert(err, gc.FitsTypeOf, &config.InvalidConfigValue{})
	c.Check(err.(*config.InvalidConfigValue).Key, gc.Equals, "GCE_CLIENT_EMAIL")
}

func (*configSuite) TestValidateAuthMissingKey(c *gc.C) {
	auth := google.Auth{
		ClientID:    "spam",
		ClientEmail: "user@mail.com",
	}
	err := google.ValidateAuth(auth)

	c.Assert(err, gc.FitsTypeOf, &config.InvalidConfigValue{})
	c.Check(err.(*config.InvalidConfigValue).Key, gc.Equals, "GCE_PRIVATE_KEY")
}
