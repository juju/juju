// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type listCredentialsSuite struct {
	testing.BaseSuite
	store              jujuclient.CredentialGetter
	personalCloudsFunc func() (map[string]jujucloud.Cloud, error)
	cloudByNameFunc    func(string) (*jujucloud.Cloud, error)
}

var _ = gc.Suite(&listCredentialsSuite{
	personalCloudsFunc: func() (map[string]jujucloud.Cloud, error) {
		return map[string]jujucloud.Cloud{
			"mycloud": {},
		}, nil
	},
	cloudByNameFunc: func(string) (*jujucloud.Cloud, error) {
		return &jujucloud.Cloud{Type: "test-provider"}, nil
	},
})

func (s *listCredentialsSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	environs.RegisterProvider("test-provider", &mockProvider{})
}

func (s *listCredentialsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.store = &jujuclienttesting.MemStore{
		Credentials: map[string]jujucloud.CloudCredential{
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
						jujucloud.UserPassAuthType,
						map[string]string{
							"username": "user",
							"password": "password",
						},
					),
				},
			},
			"google": {
				AuthCredentials: map[string]jujucloud.Credential{
					"default": jujucloud.NewCredential(
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
			"mycloud": {
				AuthCredentials: map[string]jujucloud.Credential{
					"me": jujucloud.NewCredential(
						jujucloud.AccessKeyAuthType,
						map[string]string{
							"access-key": "key",
							"secret-key": "secret",
						},
					),
				},
			},
		},
	}
}

func (s *listCredentialsSuite) TestListCredentialsTabular(c *gc.C) {
	out := s.listCredentials(c)
	c.Assert(out, gc.Equals, `
CLOUD    CREDENTIALS
aws      down*, bob
azure    azhja
google   default
mycloud  me

`[1:])
}

func (s *listCredentialsSuite) TestListCredentialsTabularFiltered(c *gc.C) {
	out := s.listCredentials(c, "aws")
	c.Assert(out, gc.Equals, `
CLOUD  CREDENTIALS
aws    down*, bob

`[1:])
}

func (s *listCredentialsSuite) TestListCredentialsYAMLWithSecrets(c *gc.C) {
	out := s.listCredentials(c, "--format", "yaml", "--show-secrets")
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
      auth-type: userpass
      password: password
      username: user
  azure:
    azhja:
      auth-type: userpass
      application-id: app-id
      application-password: app-secret
      subscription-id: subscription-id
      tenant-id: tenant-id
  google:
    default:
      auth-type: oauth2
      client-email: email
      client-id: id
      private-key: key
  mycloud:
    me:
      auth-type: access-key
      access-key: key
      secret-key: secret
`[1:])
}

func (s *listCredentialsSuite) TestListCredentialsYAMLNoSecrets(c *gc.C) {
	out := s.listCredentials(c, "--format", "yaml")
	c.Assert(out, gc.Equals, `
credentials:
  aws:
    default-credential: down
    default-region: ap-southeast-2
    bob:
      auth-type: access-key
      access-key: key
    down:
      auth-type: userpass
      username: user
  azure:
    azhja:
      auth-type: userpass
      application-id: app-id
      subscription-id: subscription-id
      tenant-id: tenant-id
  google:
    default:
      auth-type: oauth2
      client-email: email
      client-id: id
  mycloud:
    me:
      auth-type: access-key
      access-key: key
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
      subscription-id: subscription-id
      tenant-id: tenant-id
`[1:])
}

func (s *listCredentialsSuite) TestListCredentialsJSONWithSecrets(c *gc.C) {
	out := s.listCredentials(c, "--format", "json", "--show-secrets")
	c.Assert(out, gc.Equals, `
{"credentials":{"aws":{"default-credential":"down","default-region":"ap-southeast-2","cloud-credentials":{"bob":{"auth-type":"access-key","details":{"access-key":"key","secret-key":"secret"}},"down":{"auth-type":"userpass","details":{"password":"password","username":"user"}}}},"azure":{"cloud-credentials":{"azhja":{"auth-type":"userpass","details":{"application-id":"app-id","application-password":"app-secret","subscription-id":"subscription-id","tenant-id":"tenant-id"}}}},"google":{"cloud-credentials":{"default":{"auth-type":"oauth2","details":{"client-email":"email","client-id":"id","private-key":"key"}}}},"mycloud":{"cloud-credentials":{"me":{"auth-type":"access-key","details":{"access-key":"key","secret-key":"secret"}}}}}}
`[1:])
}

func (s *listCredentialsSuite) TestListCredentialsJSONNoSecrets(c *gc.C) {
	out := s.listCredentials(c, "--format", "json")
	c.Assert(out, gc.Equals, `
{"credentials":{"aws":{"default-credential":"down","default-region":"ap-southeast-2","cloud-credentials":{"bob":{"auth-type":"access-key","details":{"access-key":"key"}},"down":{"auth-type":"userpass","details":{"username":"user"}}}},"azure":{"cloud-credentials":{"azhja":{"auth-type":"userpass","details":{"application-id":"app-id","subscription-id":"subscription-id","tenant-id":"tenant-id"}}}},"google":{"cloud-credentials":{"default":{"auth-type":"oauth2","details":{"client-email":"email","client-id":"id"}}}},"mycloud":{"cloud-credentials":{"me":{"auth-type":"access-key","details":{"access-key":"key"}}}}}}
`[1:])
}

func (s *listCredentialsSuite) TestListCredentialsJSONFiltered(c *gc.C) {
	out := s.listCredentials(c, "--format", "json", "azure")
	c.Assert(out, gc.Equals, `
{"credentials":{"azure":{"cloud-credentials":{"azhja":{"auth-type":"userpass","details":{"application-id":"app-id","subscription-id":"subscription-id","tenant-id":"tenant-id"}}}}}}
`[1:])
}

func (s *listCredentialsSuite) TestListCredentialsEmpty(c *gc.C) {
	s.store = &jujuclienttesting.MemStore{
		Credentials: map[string]jujucloud.CloudCredential{
			"aws": {
				AuthCredentials: map[string]jujucloud.Credential{
					"bob": jujucloud.NewCredential(
						jujucloud.OAuth2AuthType,
						map[string]string{},
					),
				},
			},
		},
	}
	out := strings.Replace(s.listCredentials(c), "\n", "", -1)
	c.Assert(out, gc.Equals, "CLOUD  CREDENTIALSaws    bob")

	out = strings.Replace(s.listCredentials(c, "--format", "yaml"), "\n", "", -1)
	c.Assert(out, gc.Equals, "credentials:  aws:    bob:      auth-type: oauth2")

	out = strings.Replace(s.listCredentials(c, "--format", "json"), "\n", "", -1)
	c.Assert(out, gc.Equals, `{"credentials":{"aws":{"cloud-credentials":{"bob":{"auth-type":"oauth2"}}}}}`)
}

func (s *listCredentialsSuite) TestListCredentialsNone(c *gc.C) {
	listCmd := cloud.NewListCredentialsCommandForTest(jujuclienttesting.NewMemStore(), s.personalCloudsFunc, s.cloudByNameFunc)
	ctx, err := testing.RunCommand(c, listCmd)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stderr(ctx), gc.Equals, "")
	out := strings.Replace(testing.Stdout(ctx), "\n", "", -1)
	c.Assert(out, gc.Equals, "No credentials to display.")

	ctx, err = testing.RunCommand(c, listCmd, "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stderr(ctx), gc.Equals, "")
	out = strings.Replace(testing.Stdout(ctx), "\n", "", -1)
	c.Assert(out, gc.Equals, "credentials: {}")

	ctx, err = testing.RunCommand(c, listCmd, "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stderr(ctx), gc.Equals, "")
	out = strings.Replace(testing.Stdout(ctx), "\n", "", -1)
	c.Assert(out, gc.Equals, `{"credentials":{}}`)
}

func (s *listCredentialsSuite) listCredentials(c *gc.C, args ...string) string {
	ctx, err := testing.RunCommand(c, cloud.NewListCredentialsCommandForTest(s.store, s.personalCloudsFunc, s.cloudByNameFunc), args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stderr(ctx), gc.Equals, "")
	return testing.Stdout(ctx)
}
