// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce/google"
)

type credentialsSuite struct {
	google.BaseSuite
}

var _ = gc.Suite(&credentialsSuite{})

func (*credentialsSuite) TestValidateValid(c *gc.C) {
	creds := &google.Credentials{
		ClientID:    "spam",
		ClientEmail: "user@mail.com",
		PrivateKey:  []byte("non-empty"),
	}
	err := creds.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (*credentialsSuite) TestValidateMissingID(c *gc.C) {
	creds := &google.Credentials{
		ClientEmail: "user@mail.com",
		PrivateKey:  []byte("non-empty"),
	}
	err := creds.Validate()

	c.Assert(err, gc.FitsTypeOf, &google.InvalidCredential{})
	c.Check(err.(*google.InvalidCredential).Key, gc.Equals, "GCE_CLIENT_ID")
}

func (*credentialsSuite) TestValidateBadEmail(c *gc.C) {
	creds := &google.Credentials{
		ClientID:    "spam",
		ClientEmail: "bad_email",
		PrivateKey:  []byte("non-empty"),
	}
	err := creds.Validate()

	c.Assert(err, gc.FitsTypeOf, &google.InvalidCredential{})
	c.Check(err.(*google.InvalidCredential).Key, gc.Equals, "GCE_CLIENT_EMAIL")
}

func (*credentialsSuite) TestValidateMissingKey(c *gc.C) {
	creds := &google.Credentials{
		ClientID:    "spam",
		ClientEmail: "user@mail.com",
	}
	err := creds.Validate()

	c.Assert(err, gc.FitsTypeOf, &google.InvalidCredential{})
	c.Check(err.(*google.InvalidCredential).Key, gc.Equals, "GCE_PRIVATE_KEY")
}
