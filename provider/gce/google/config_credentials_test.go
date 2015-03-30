// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce/google"
)

type credentialsSuite struct {
	google.BaseSuite
}

var _ = gc.Suite(&credentialsSuite{})

func (s *credentialsSuite) TestNewCredentials(c *gc.C) {
	values := map[string]string{
		google.OSEnvClientID:    "abc",
		google.OSEnvClientEmail: "xyz@g.com",
		google.OSEnvPrivateKey:  "<some-key>",
	}
	creds, err := google.NewCredentials(values)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(creds, jc.DeepEquals, &google.Credentials{
		ClientID:    "abc",
		ClientEmail: "xyz@g.com",
		PrivateKey:  []byte("<some-key>"),
	})
}

func (s *credentialsSuite) TestNewCredentialsEmpty(c *gc.C) {
	creds, err := google.NewCredentials(nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(creds, jc.DeepEquals, &google.Credentials{})
}

func (s *credentialsSuite) TestNewCredentialsUnrecognized(c *gc.C) {
	values := map[string]string{
		"spam": "eggs",
	}
	_, err := google.NewCredentials(values)

	c.Check(err, gc.FitsTypeOf, errors.NotSupportedf(""))
}

func (s *credentialsSuite) TestCredentialsValues(c *gc.C) {
	original := map[string]string{
		google.OSEnvClientID:    "abc",
		google.OSEnvClientEmail: "xyz@g.com",
		google.OSEnvPrivateKey:  "<some-key>",
	}
	creds, err := google.NewCredentials(original)
	c.Assert(err, jc.ErrorIsNil)
	values := creds.Values()

	c.Check(values, jc.DeepEquals, original)
}

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

	c.Assert(err, gc.FitsTypeOf, &google.InvalidConfigValue{})
	c.Check(err.(*google.InvalidConfigValue).Key, gc.Equals, "GCE_CLIENT_ID")
}

func (*credentialsSuite) TestValidateBadEmail(c *gc.C) {
	creds := &google.Credentials{
		ClientID:    "spam",
		ClientEmail: "bad_email",
		PrivateKey:  []byte("non-empty"),
	}
	err := creds.Validate()

	c.Assert(err, gc.FitsTypeOf, &google.InvalidConfigValue{})
	c.Check(err.(*google.InvalidConfigValue).Key, gc.Equals, "GCE_CLIENT_EMAIL")
}

func (*credentialsSuite) TestValidateMissingKey(c *gc.C) {
	creds := &google.Credentials{
		ClientID:    "spam",
		ClientEmail: "user@mail.com",
	}
	err := creds.Validate()

	c.Assert(err, gc.FitsTypeOf, &google.InvalidConfigValue{})
	c.Check(err.(*google.InvalidConfigValue).Key, gc.Equals, "GCE_PRIVATE_KEY")
}
