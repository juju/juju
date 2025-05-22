// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	"bytes"
	"encoding/json"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/provider/gce/google"
)

type credentialsSuite struct {
	google.BaseSuite
}

func TestCredentialsSuite(t *stdtesting.T) {
	tc.Run(t, &credentialsSuite{})
}

func (s *credentialsSuite) TestNewCredentials(c *tc.C) {
	values := map[string]string{
		google.OSEnvClientID:    "abc",
		google.OSEnvClientEmail: "xyz@g.com",
		google.OSEnvPrivateKey:  "<some-key>",
		google.OSEnvProjectID:   "yup",
	}
	creds, err := google.NewCredentials(values)
	c.Assert(err, tc.ErrorIsNil)

	jsonKey := creds.JSONKey
	creds.JSONKey = nil
	c.Check(creds, tc.DeepEquals, &google.Credentials{
		ClientID:    "abc",
		ClientEmail: "xyz@g.com",
		PrivateKey:  []byte("<some-key>"),
		ProjectID:   "yup",
	})
	data := make(map[string]string)
	err = json.Unmarshal(jsonKey, &data)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(data, tc.DeepEquals, map[string]string{
		"type":         "service_account",
		"client_id":    "abc",
		"client_email": "xyz@g.com",
		"private_key":  "<some-key>",
	})
}

func (s *credentialsSuite) TestNewCredentialsUnrecognized(c *tc.C) {
	values := map[string]string{
		"spam": "eggs",
	}
	_, err := google.NewCredentials(values)

	c.Check(err, tc.ErrorIs, errors.NotSupported)
}

func (s *credentialsSuite) TestNewCredentialsValidates(c *tc.C) {
	values := map[string]string{
		google.OSEnvClientEmail: "xyz@g.com",
		google.OSEnvPrivateKey:  "<some-key>",
		google.OSEnvProjectID:   "yup",
	}
	_, err := google.NewCredentials(values)
	// This error comes from Credentials.Validate so by implication
	// if we're getting this error, validation is being performed.
	c.Check(err, tc.ErrorMatches, `invalid config value \(\) for "GCE_CLIENT_ID": missing ClientID`)
	c.Assert(err, tc.Satisfies, google.IsInvalidConfigValueError)
}

func (s *credentialsSuite) TestParseJSONKey(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)

	jsonKey := creds.JSONKey
	creds.JSONKey = nil
	c.Check(creds, tc.DeepEquals, &google.Credentials{
		ClientID:    "abc",
		ClientEmail: "xyz@g.com",
		PrivateKey:  []byte("<some-key>"),
		ProjectID:   "yup",
	})
	c.Check(string(jsonKey), tc.Equals, original)
}

func (s *credentialsSuite) TestCredentialsValues(c *tc.C) {
	original := map[string]string{
		google.OSEnvClientID:    "abc",
		google.OSEnvClientEmail: "xyz@g.com",
		google.OSEnvPrivateKey:  "<some-key>",
		google.OSEnvProjectID:   "yup",
	}
	creds, err := google.NewCredentials(original)
	c.Assert(err, tc.ErrorIsNil)
	values := creds.Values()

	c.Check(values, tc.DeepEquals, original)
}

func (*credentialsSuite) TestValidateValid(c *tc.C) {
	creds := &google.Credentials{
		ClientID:    "spam",
		ClientEmail: "user@mail.com",
		PrivateKey:  []byte("non-empty"),
	}
	err := creds.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (*credentialsSuite) TestValidateMissingID(c *tc.C) {
	creds := &google.Credentials{
		ClientEmail: "user@mail.com",
		PrivateKey:  []byte("non-empty"),
	}
	err := creds.Validate()

	c.Assert(err, tc.Satisfies, google.IsInvalidConfigValueError)
	c.Check(err.(*google.InvalidConfigValueError).Key, tc.Equals, "GCE_CLIENT_ID")
}

func (*credentialsSuite) TestValidateBadEmail(c *tc.C) {
	creds := &google.Credentials{
		ClientID:    "spam",
		ClientEmail: "bad_email",
		PrivateKey:  []byte("non-empty"),
	}
	err := creds.Validate()

	c.Assert(err, tc.Satisfies, google.IsInvalidConfigValueError)
	c.Check(err.(*google.InvalidConfigValueError).Key, tc.Equals, "GCE_CLIENT_EMAIL")
}

func (*credentialsSuite) TestValidateMissingKey(c *tc.C) {
	creds := &google.Credentials{
		ClientID:    "spam",
		ClientEmail: "user@mail.com",
	}
	err := creds.Validate()

	c.Assert(err, tc.Satisfies, google.IsInvalidConfigValueError)
	c.Check(err.(*google.InvalidConfigValueError).Key, tc.Equals, "GCE_PRIVATE_KEY")
}
