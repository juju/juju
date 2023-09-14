// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"os"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type CredentialsFileSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&CredentialsFileSuite{})

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

func (s *CredentialsFileSuite) TestWriteFile(c *gc.C) {
	writeTestCredentialsFile(c)
	data, err := os.ReadFile(osenv.JujuXDGDataHomePath("credentials.yaml"))
	c.Assert(err, jc.ErrorIsNil)

	var original map[string]interface{}
	err = yaml.Unmarshal([]byte(testCredentialsYAML), &original)
	c.Assert(err, jc.ErrorIsNil)
	var written map[string]interface{}
	err = yaml.Unmarshal(data, &written)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(written, gc.DeepEquals, original)
}

func (s *CredentialsFileSuite) TestReadNoFile(c *gc.C) {
	credentials, err := jujuclient.ReadCredentialsFile("nohere.yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credentials, gc.NotNil)
}

func (s *CredentialsFileSuite) TestReadEmptyFile(c *gc.C) {
	err := os.WriteFile(osenv.JujuXDGDataHomePath("credentials.yaml"), []byte(""), 0600)
	c.Assert(err, jc.ErrorIsNil)

	credentialstore := jujuclient.NewFileCredentialStore()
	_, err = credentialstore.CredentialForCloud("foo")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func parseCredentials(c *gc.C) *cloud.CredentialCollection {
	credentials, err := cloud.ParseCredentialCollection([]byte(testCredentialsYAML))
	c.Assert(err, jc.ErrorIsNil)
	return credentials
}

func writeTestCredentialsFile(c *gc.C) map[string]cloud.CloudCredential {
	credentials := parseCredentials(c)
	err := jujuclient.WriteCredentialsFile(credentials)
	c.Assert(err, jc.ErrorIsNil)
	allCredentials := make(map[string]cloud.CloudCredential)
	names := credentials.CloudNames()
	for _, cloudName := range names {
		cred, err := credentials.CloudCredential(cloudName)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(cred, gc.NotNil)
		allCredentials[cloudName] = *cred
	}
	return allCredentials
}
