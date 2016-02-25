// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"io/ioutil"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/errors"
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
	data, err := ioutil.ReadFile(osenv.JujuXDGDataHomePath("credentials.yaml"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, testCredentialsYAML[1:])
}

func (s *CredentialsFileSuite) TestReadNoFile(c *gc.C) {
	credentials, err := jujuclient.ReadCredentialsFile("nohere.yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credentials, gc.IsNil)
}

func (s *CredentialsFileSuite) TestReadEmptyFile(c *gc.C) {
	err := ioutil.WriteFile(osenv.JujuXDGDataHomePath("credentials.yaml"), []byte(""), 0600)
	c.Assert(err, jc.ErrorIsNil)

	credentialstore := jujuclient.NewFileCredentialStore()
	_, err = credentialstore.CredentialForCloud("foo")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func parseCredentials(c *gc.C) map[string]cloud.CloudCredential {
	credentials, err := cloud.ParseCredentials([]byte(testCredentialsYAML))
	c.Assert(err, jc.ErrorIsNil)
	return credentials
}

func writeTestCredentialsFile(c *gc.C) map[string]cloud.CloudCredential {
	credentials := parseCredentials(c)
	err := jujuclient.WriteCredentialsFile(credentials)
	c.Assert(err, jc.ErrorIsNil)
	return credentials
}
