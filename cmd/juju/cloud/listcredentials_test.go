// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type listCredentialsSuite struct {
	testing.BaseSuite

	jujuXDGDataHome string
}

var _ = gc.Suite(&listCredentialsSuite{})

var sampleCredentials = map[string]jujucloud.CloudCredential{
	"aws": {
		DefaultRegion:     "ap-southeast-2",
		DefaultCredential: "down",
		AuthCredentials: map[string]jujucloud.Credential{
			"bob": jujucloud.NewCredential(
				jujucloud.AccessKeyAuthType,
				map[string]string{
					"access-key": "key",
					"secret-key": "secret",
				},
			),
			"down": jujucloud.NewCredential(
				jujucloud.OAuth2AuthType,
				map[string]string{
					"client-id":    "id",
					"client-email": "email",
					"private-key":  "key",
				},
			),
		},
	},
	"azure": {
		AuthCredentials: map[string]jujucloud.Credential{
			"azhja": jujucloud.NewCredential(
				jujucloud.UserPassAuthType,
				map[string]string{
					"application-id":       "app-id",
					"application-password": "app-secret",
					"subscription-id":      "subscription-id",
					"tenant-id":            "tenant-id",
				},
			),
		},
	},
}

func (s *listCredentialsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.jujuXDGDataHome = c.MkDir()
	oldJujuXDGDataHome := osenv.SetJujuXDGDataHome(s.jujuXDGDataHome)
	s.AddCleanup(func(c *gc.C) {
		osenv.SetJujuXDGDataHome(oldJujuXDGDataHome)
	})

	// Write $XDG_DATA_HOME/juju/credentials.yaml.
	data, err := jujucloud.MarshalCredentials(sampleCredentials)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filepath.Join(s.jujuXDGDataHome, "credentials.yaml"), data, 0600)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *listCredentialsSuite) TestListCredentialsTabular(c *gc.C) {
	out := s.listCredentials(c)
	c.Assert(out, gc.Equals, `
CLOUD  CREDENTIALS
aws    down*, bob
azure  azhja

`[1:])
}

func (s *listCredentialsSuite) TestListCredentialsTabularFiltered(c *gc.C) {
	out := s.listCredentials(c, "aws")
	c.Assert(out, gc.Equals, `
CLOUD  CREDENTIALS
aws    down*, bob

`[1:])
}

func (s *listCredentialsSuite) TestListCredentialsYAML(c *gc.C) {
	out := s.listCredentials(c, "--format", "yaml")
	c.Assert(out, gc.Equals, `
credentials:
  aws:
    default-credential: down
    default-region: ap-southeast-2
    bob:
      auth-type: access-key
      access-key: key
      secret-key: secret
    down:
      auth-type: oauth2
      client-email: email
      client-id: id
      private-key: key
  azure:
    azhja:
      auth-type: userpass
      application-id: app-id
      application-password: app-secret
      subscription-id: subscription-id
      tenant-id: tenant-id
`[1:])
}

func (s *listCredentialsSuite) TestListCredentialsYAMLFiltered(c *gc.C) {
	out := s.listCredentials(c, "--format", "yaml", "azure")
	c.Assert(out, gc.Equals, `
credentials:
  azure:
    azhja:
      auth-type: userpass
      application-id: app-id
      application-password: app-secret
      subscription-id: subscription-id
      tenant-id: tenant-id
`[1:])
}

func (s *listCredentialsSuite) TestListCredentialsJSON(c *gc.C) {
	// TODO(axw) test once json marshalling works properly
	c.Skip("not implemented: credentials don't marshal to JSON yet")
}

func (s *listCredentialsSuite) TestListCredentialsFileMissing(c *gc.C) {
	err := os.RemoveAll(s.jujuXDGDataHome)
	c.Assert(err, jc.ErrorIsNil)

	out := s.listCredentials(c)
	out = strings.Replace(out, "\n", "", -1)
	c.Assert(out, gc.Equals, "CLOUD  CREDENTIALS")

	out = s.listCredentials(c, "--format", "yaml")
	out = strings.Replace(out, "\n", "", -1)
	c.Assert(out, gc.Equals, "credentials: {}")

	// TODO(axw) test json once json marshaling works properly
}

func (s *listCredentialsSuite) listCredentials(c *gc.C, args ...string) string {
	ctx, err := testing.RunCommand(c, cloud.NewListCredentialsCommand(), args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stderr(ctx), gc.Equals, "")
	return testing.Stdout(ctx)
}
