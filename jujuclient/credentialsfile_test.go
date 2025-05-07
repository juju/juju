// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"os"

	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
)

type CredentialsFileSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = tc.Suite(&CredentialsFileSuite{})

const testCredentialsYAML = `
credentials:
  aws:
    default-credential: peter
    default-region: us-west-2
    paul:
      auth-type: access-key
      access-key: paul-key
      secret-key: paul-secret
    peter:
      auth-type: access-key
      access-key: key
      secret-key: secret
  aws-gov:
    fbi:
      auth-type: access-key
      access-key: key
      secret-key: secret
`

func (s *CredentialsFileSuite) TestWriteFile(c *tc.C) {
	writeTestCredentialsFile(c)
	data, err := os.ReadFile(osenv.JujuXDGDataHomePath("credentials.yaml"))
	c.Assert(err, jc.ErrorIsNil)

	var original map[string]interface{}
	err = yaml.Unmarshal([]byte(testCredentialsYAML), &original)
	c.Assert(err, jc.ErrorIsNil)
	var written map[string]interface{}
	err = yaml.Unmarshal(data, &written)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(written, tc.DeepEquals, original)
}

func (s *CredentialsFileSuite) TestReadNoFile(c *tc.C) {
	credentials, err := jujuclient.ReadCredentialsFile("nohere.yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credentials, tc.NotNil)
}

func (s *CredentialsFileSuite) TestReadEmptyFile(c *tc.C) {
	err := os.WriteFile(osenv.JujuXDGDataHomePath("credentials.yaml"), []byte(""), 0600)
	c.Assert(err, jc.ErrorIsNil)

	credentialstore := jujuclient.NewFileCredentialStore()
	_, err = credentialstore.CredentialForCloud("foo")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func parseCredentials(c *tc.C) *cloud.CredentialCollection {
	credentials, err := cloud.ParseCredentialCollection([]byte(testCredentialsYAML))
	c.Assert(err, jc.ErrorIsNil)
	return credentials
}

func writeTestCredentialsFile(c *tc.C) map[string]cloud.CloudCredential {
	credentials := parseCredentials(c)
	err := jujuclient.WriteCredentialsFile(credentials)
	c.Assert(err, jc.ErrorIsNil)
	allCredentials := make(map[string]cloud.CloudCredential)
	names := credentials.CloudNames()
	for _, cloudName := range names {
		cred, err := credentials.CloudCredential(cloudName)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(cred, tc.NotNil)
		allCredentials[cloudName] = *cred
	}
	return allCredentials
}
