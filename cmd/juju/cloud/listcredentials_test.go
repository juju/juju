// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type listCredentialsSuite struct {
	testing.BaseSuite
	store jujuclient.CredentialGetter
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
	s.store = &jujuclienttesting.MemStore{
		Credentials: sampleCredentials,
	}
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

func (s *listCredentialsSuite) TestListCredentialsNone(c *gc.C) {
	listCmd := cloud.NewListCredentialsCommandForTest(jujuclienttesting.NewMemStore())
	ctx, err := testing.RunCommand(c, listCmd)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stderr(ctx), gc.Equals, "")
	out := strings.Replace(testing.Stdout(ctx), "\n", "", -1)
	c.Assert(out, gc.Equals, "CLOUD  CREDENTIALS")

	ctx, err = testing.RunCommand(c, listCmd, "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stderr(ctx), gc.Equals, "")
	out = strings.Replace(testing.Stdout(ctx), "\n", "", -1)
	c.Assert(out, gc.Equals, "credentials: {}")

	// TODO(axw) test json once json marshaling works properly
}

func (s *listCredentialsSuite) listCredentials(c *gc.C, args ...string) string {
	ctx, err := testing.RunCommand(c, cloud.NewListCredentialsCommandForTest(s.store), args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stderr(ctx), gc.Equals, "")
	return testing.Stdout(ctx)
}
