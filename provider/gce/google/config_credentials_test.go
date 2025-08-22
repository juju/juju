// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	"bytes"
	"encoding/json"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce/google"
	"github.com/juju/juju/testing"
)

type credentialsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&credentialsSuite{})

func (s *credentialsSuite) TestNewCredentials(c *gc.C) {
	values := map[string]string{
		google.OSEnvClientID:    "abc",
		google.OSEnvClientEmail: "xyz@g.com",
		google.OSEnvPrivateKey:  "<some-key>",
		google.OSEnvProjectID:   "yup",
	}
	creds, err := google.NewCredentials(values)
	c.Assert(err, jc.ErrorIsNil)

	jsonKey := creds.JSONKey
	creds.JSONKey = nil
	c.Check(creds, jc.DeepEquals, &google.Credentials{
		ClientID:    "abc",
		ClientEmail: "xyz@g.com",
		PrivateKey:  []byte("<some-key>"),
		ProjectID:   "yup",
	})
	data := make(map[string]string)
	err = json.Unmarshal(jsonKey, &data)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(data, jc.DeepEquals, map[string]string{
		"type":         "service_account",
		"client_id":    "abc",
		"client_email": "xyz@g.com",
		"private_key":  "<some-key>",
	})
}

func (s *credentialsSuite) TestNewCredentialsUnrecognized(c *gc.C) {
	values := map[string]string{
		"spam": "eggs",
	}
	_, err := google.NewCredentials(values)

	c.Check(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *credentialsSuite) TestNewCredentialsValidates(c *gc.C) {
	values := map[string]string{
		google.OSEnvClientEmail: "xyz@g.com",
		google.OSEnvPrivateKey:  "<some-key>",
		google.OSEnvProjectID:   "yup",
	}
	_, err := google.NewCredentials(values)
	// This error comes from Credentials.Validate so by implication
	// if we're getting this error, validation is being performed.
	c.Check(err, gc.ErrorMatches, `invalid config value \(\) for "GCE_CLIENT_ID": missing ClientID`)
	c.Assert(err, jc.Satisfies, google.IsInvalidConfigValueError)
}

func (s *credentialsSuite) TestParseJSONKey(c *gc.C) {
	original := `
{
    "private_key_id": "mnopq",
    "private_key": "<some-key>",
    "client_email": "xyz@g.com",
    "client_id": "abc",
    "project_id": "yup",
    "type": "service_account"
}`[1:]
	creds, err := google.ParseJSONKey(bytes.NewBufferString(original))
	c.Assert(err, jc.ErrorIsNil)

	jsonKey := creds.JSONKey
	creds.JSONKey = nil
	c.Check(creds, jc.DeepEquals, &google.Credentials{
		ClientID:    "abc",
		ClientEmail: "xyz@g.com",
		PrivateKey:  []byte("<some-key>"),
		ProjectID:   "yup",
	})
	c.Check(string(jsonKey), gc.Equals, original)
}

func (s *credentialsSuite) TestCredentialsValues(c *gc.C) {
	original := map[string]string{
		google.OSEnvClientID:    "abc",
		google.OSEnvClientEmail: "xyz@g.com",
		google.OSEnvPrivateKey:  "<some-key>",
		google.OSEnvProjectID:   "yup",
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

	c.Assert(err, jc.Satisfies, google.IsInvalidConfigValueError)
	c.Check(err.(*google.InvalidConfigValueError).Key, gc.Equals, "GCE_CLIENT_ID")
}

func (*credentialsSuite) TestValidateBadEmail(c *gc.C) {
	creds := &google.Credentials{
		ClientID:    "spam",
		ClientEmail: "bad_email",
		PrivateKey:  []byte("non-empty"),
	}
	err := creds.Validate()

	c.Assert(err, jc.Satisfies, google.IsInvalidConfigValueError)
	c.Check(err.(*google.InvalidConfigValueError).Key, gc.Equals, "GCE_CLIENT_EMAIL")
}

func (*credentialsSuite) TestValidateMissingKey(c *gc.C) {
	creds := &google.Credentials{
		ClientID:    "spam",
		ClientEmail: "user@mail.com",
	}
	err := creds.Validate()

	c.Assert(err, jc.Satisfies, google.IsInvalidConfigValueError)
	c.Check(err.(*google.InvalidConfigValueError).Key, gc.Equals, "GCE_PRIVATE_KEY")
}
