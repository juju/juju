// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
)

type listCredentialsSuite struct {
	testing.BaseSuite
	store              *jujuclient.MemStore
	personalCloudsFunc func() (map[string]jujucloud.Cloud, error)
	cloudByNameFunc    func(string) (*jujucloud.Cloud, error)
	apiF               func() (cloud.ListCredentialsAPI, error)
	testAPI            *mockAPI
}

var _ = gc.Suite(&listCredentialsSuite{
	personalCloudsFunc: func() (map[string]jujucloud.Cloud, error) {
		return map[string]jujucloud.Cloud{
			"mycloud":      {},
			"missingcloud": {},
		}, nil
	},
	cloudByNameFunc: func(name string) (*jujucloud.Cloud, error) {
		if name == "missingcloud" {
			return nil, errors.NotValidf(name)
		}
		return &jujucloud.Cloud{Type: "test-provider"}, nil
	},
})

func (s *listCredentialsSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	unreg := environs.RegisterProvider("test-provider", &mockProvider{})
	s.AddCleanup(func(_ *gc.C) {
		unreg()
	})
}

func (s *listCredentialsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.store = jujuclient.NewMemStore()
	s.store.Credentials = map[string]jujucloud.CloudCredential{
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
	}
	s.testAPI = &mockAPI{
		credentialContentsF: func(cloud, credential string, withSecrets bool) ([]params.CredentialContentResult, error) {
			return nil, nil
		},
	}
	s.apiF = func() (cloud.ListCredentialsAPI, error) {
		return s.testAPI, nil
	}
}

func (s *listCredentialsSuite) TestListCredentialsTabular(c *gc.C) {
	out := s.listCredentials(c, "--client")
	c.Assert(out, gc.Equals, `

Client Credentials:
Cloud    Credentials
aws      down*, bob
azure    azhja
google   default
mycloud  me

`[1:])
}

func (s *listCredentialsSuite) TestListCredentialsTabularInvalidCredential(c *gc.C) {
	store := jujuclienttesting.WrapClientStore(s.store)
	store.CredentialForCloudFunc = func(cloudName string) (*jujucloud.CloudCredential, error) {
		if cloudName == "mycloud" {
			return nil, errors.Errorf("expected error")
		}
		return s.store.CredentialForCloud(cloudName)
	}

	var logWriter loggo.TestWriter
	writerName := "TestListCredentialsTabularInvalidCredential"
	c.Assert(loggo.RegisterWriter(writerName, &logWriter), jc.ErrorIsNil)
	defer func() {
		loggo.RemoveWriter(writerName)
		logWriter.Clear()
	}()

	ctx := s.listCredentialsWithStore(c, store, "--client")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `

Client Credentials:
Cloud   Credentials
aws     down*, bob
azure   azhja
google  default

`[1:])
	c.Check(logWriter.Log(), jc.LogMatches, []jc.SimpleMessage{
		{
			Level:   loggo.WARNING,
			Message: `error loading credential for cloud mycloud: expected error`,
		},
	})
}

func (s *listCredentialsSuite) TestListCredentialsTabularShowsNoSecrets(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListCredentialsCommandForTest(s.store, s.personalCloudsFunc, s.cloudByNameFunc, s.apiF), "--show-secrets", "--client")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "secrets are not shown in tabular format\n")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `

Client Credentials:
Cloud    Credentials
aws      down*, bob
azure    azhja
google   default
mycloud  me

`[1:])
}

func (s *listCredentialsSuite) TestListCredentialsTabularMissingCloud(c *gc.C) {
	s.store.Credentials["missingcloud"] = jujucloud.CloudCredential{}
	out := s.listCredentials(c, "--client")
	c.Assert(out, gc.Equals, `
The following clouds have been removed and are omitted from the results to avoid leaking secrets.
Run with --show-secrets to display these clouds' credentials: missingcloud


Client Credentials:
Cloud    Credentials
aws      down*, bob
azure    azhja
google   default
mycloud  me

`[1:])
}

func (s *listCredentialsSuite) TestListCredentialsTabularFiltered(c *gc.C) {
	out := s.listCredentials(c, "aws", "--client")
	c.Assert(out, gc.Equals, `

Client Credentials:
Cloud  Credentials
aws    down*, bob

`[1:])
}

func (s *listCredentialsSuite) TestListCredentialsTabularFilteredLocalOnly(c *gc.C) {
	out := s.listCredentials(c, "aws", "--client")
	c.Assert(out, gc.Equals, `

Client Credentials:
Cloud  Credentials
aws    down*, bob

`[1:])
}

func (s *listCredentialsSuite) TestListRemoteCredentialsWithSecrets(c *gc.C) {
	s.testAPI.credentialContentsF = func(cloud, credential string, withSecrets bool) ([]params.CredentialContentResult, error) {
		c.Assert(withSecrets, jc.IsTrue)
		return nil, nil
	}
	out := s.listCredentials(c, "aws", "--show-secrets", "--format", "yaml", "--client")
	c.Assert(out, gc.Equals, `
client-credentials:
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
`[1:])
}

func (s *listCredentialsSuite) TestListAllCredentials(c *gc.C) {
	s.store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	s.store.CurrentControllerName = "mycontroller"
	s.testAPI.credentialContentsF = func(cloud, credential string, withSecrets bool) ([]params.CredentialContentResult, error) {
		return []params.CredentialContentResult{
			{Result: &params.ControllerCredentialInfo{Content: params.CredentialContent{Cloud: "remote-cloud", Name: "remote-name"}}},
			{Error: apiservererrors.ServerError(errors.New("kabbom"))},
		}, nil
	}
	out := s.listCredentials(c)
	c.Assert(out, gc.Equals, `

Controller Credentials:
Cloud         Credentials
remote-cloud  remote-name

Client Credentials:
Cloud    Credentials
aws      down*, bob
azure    azhja
google   default
mycloud  me

`[1:])
}

func (s *listCredentialsSuite) TestListCredentialsYAMLWithSecrets(c *gc.C) {
	s.store.Credentials["missingcloud"] = jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"default": jujucloud.NewCredential(
				jujucloud.AccessKeyAuthType,
				map[string]string{
					"access-key": "key",
					"secret-key": "secret",
				},
			),
		},
	}
	out := s.listCredentials(c, "--format", "yaml", "--show-secrets", "--client")
	c.Assert(out, gc.Equals, `
client-credentials:
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
  missingcloud:
    default:
      auth-type: access-key
      access-key: key
      secret-key: secret
  mycloud:
    me:
      auth-type: access-key
      access-key: key
      secret-key: secret
`[1:])
}

func (s *listCredentialsSuite) TestListCredentialsYAMLWithSecretsInvalidCredential(c *gc.C) {
	s.store.Credentials["missingcloud"] = jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"default": jujucloud.NewCredential(
				jujucloud.AccessKeyAuthType,
				map[string]string{
					"access-key": "key",
					"secret-key": "secret",
				},
			),
		},
	}
	store := jujuclienttesting.WrapClientStore(s.store)
	store.CredentialForCloudFunc = func(cloudName string) (*jujucloud.CloudCredential, error) {
		if cloudName == "mycloud" {
			return nil, errors.Errorf("expected error")
		}
		return s.store.CredentialForCloud(cloudName)
	}

	var logWriter loggo.TestWriter
	writerName := "TestListCredentialsYAMLWithSecretsInvalidCredential"
	c.Assert(loggo.RegisterWriter(writerName, &logWriter), jc.ErrorIsNil)
	defer func() {
		loggo.RemoveWriter(writerName)
		logWriter.Clear()
	}()

	ctx := s.listCredentialsWithStore(c, store, "--format", "yaml", "--show-secrets", "--client")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
client-credentials:
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
  missingcloud:
    default:
      auth-type: access-key
      access-key: key
      secret-key: secret
`[1:])
	c.Check(logWriter.Log(), jc.LogMatches, []jc.SimpleMessage{
		{
			Level:   loggo.WARNING,
			Message: `error loading credential for cloud mycloud: expected error`,
		},
	})
}

func (s *listCredentialsSuite) TestListCredentialsYAMLNoSecrets(c *gc.C) {
	s.store.Credentials["missingcloud"] = jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"default": jujucloud.NewCredential(
				jujucloud.AccessKeyAuthType,
				map[string]string{
					"access-key": "key",
					"secret-key": "secret",
				},
			),
		},
	}
	out := s.listCredentials(c, "--format", "yaml", "--client")
	c.Assert(out, gc.Equals, `
client-credentials:
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
	out := s.listCredentials(c, "--format", "yaml", "azure", "--client")
	c.Assert(out, gc.Equals, `
client-credentials:
  azure:
    azhja:
      auth-type: userpass
      application-id: app-id
      subscription-id: subscription-id
      tenant-id: tenant-id
`[1:])
}

func (s *listCredentialsSuite) TestListCredentialsJSONWithSecrets(c *gc.C) {
	out := s.listCredentials(c, "--format", "json", "--show-secrets", "--client")
	c.Assert(out, gc.Equals, `
{"client-credentials":{"aws":{"default-credential":"down","default-region":"ap-southeast-2","cloud-credentials":{"bob":{"auth-type":"access-key","details":{"access-key":"key","secret-key":"secret"}},"down":{"auth-type":"userpass","details":{"password":"password","username":"user"}}}},"azure":{"cloud-credentials":{"azhja":{"auth-type":"userpass","details":{"application-id":"app-id","application-password":"app-secret","subscription-id":"subscription-id","tenant-id":"tenant-id"}}}},"google":{"cloud-credentials":{"default":{"auth-type":"oauth2","details":{"client-email":"email","client-id":"id","private-key":"key"}}}},"mycloud":{"cloud-credentials":{"me":{"auth-type":"access-key","details":{"access-key":"key","secret-key":"secret"}}}}}}
`[1:])
}

func (s *listCredentialsSuite) TestListCredentialsJSONWithSecretsInvalidCredential(c *gc.C) {
	store := jujuclienttesting.WrapClientStore(s.store)
	store.CredentialForCloudFunc = func(cloudName string) (*jujucloud.CloudCredential, error) {
		if cloudName == "mycloud" {
			return nil, errors.Errorf("expected error")
		}
		return s.store.CredentialForCloud(cloudName)
	}

	var logWriter loggo.TestWriter
	writerName := "TestListCredentialsJSONWithSecretsInvalidCredential"
	c.Assert(loggo.RegisterWriter(writerName, &logWriter), jc.ErrorIsNil)
	defer func() {
		loggo.RemoveWriter(writerName)
		logWriter.Clear()
	}()

	ctx := s.listCredentialsWithStore(c, store, "--format", "json", "--show-secrets", "--client")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
{"client-credentials":{"aws":{"default-credential":"down","default-region":"ap-southeast-2","cloud-credentials":{"bob":{"auth-type":"access-key","details":{"access-key":"key","secret-key":"secret"}},"down":{"auth-type":"userpass","details":{"password":"password","username":"user"}}}},"azure":{"cloud-credentials":{"azhja":{"auth-type":"userpass","details":{"application-id":"app-id","application-password":"app-secret","subscription-id":"subscription-id","tenant-id":"tenant-id"}}}},"google":{"cloud-credentials":{"default":{"auth-type":"oauth2","details":{"client-email":"email","client-id":"id","private-key":"key"}}}}}}
`[1:])
	c.Check(logWriter.Log(), jc.LogMatches, []jc.SimpleMessage{
		{
			Level:   loggo.WARNING,
			Message: `error loading credential for cloud mycloud: expected error`,
		},
	})
}

func (s *listCredentialsSuite) TestListCredentialsJSONNoSecrets(c *gc.C) {
	out := s.listCredentials(c, "--format", "json", "--client")
	c.Assert(out, gc.Equals, `
{"client-credentials":{"aws":{"default-credential":"down","default-region":"ap-southeast-2","cloud-credentials":{"bob":{"auth-type":"access-key","details":{"access-key":"key"}},"down":{"auth-type":"userpass","details":{"username":"user"}}}},"azure":{"cloud-credentials":{"azhja":{"auth-type":"userpass","details":{"application-id":"app-id","subscription-id":"subscription-id","tenant-id":"tenant-id"}}}},"google":{"cloud-credentials":{"default":{"auth-type":"oauth2","details":{"client-email":"email","client-id":"id"}}}},"mycloud":{"cloud-credentials":{"me":{"auth-type":"access-key","details":{"access-key":"key"}}}}}}
`[1:])
}

func (s *listCredentialsSuite) TestListCredentialsJSONFiltered(c *gc.C) {
	out := s.listCredentials(c, "--format", "json", "azure", "--client")
	c.Assert(out, gc.Equals, `
{"client-credentials":{"azure":{"cloud-credentials":{"azhja":{"auth-type":"userpass","details":{"application-id":"app-id","subscription-id":"subscription-id","tenant-id":"tenant-id"}}}}}}
`[1:])
}

func (s *listCredentialsSuite) TestListCredentialsClient(c *gc.C) {
	s.store = &jujuclient.MemStore{
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
	out := strings.Replace(s.listCredentials(c, "--client"), "\n", "", -1)
	c.Assert(out, gc.Equals, "Client Credentials:Cloud  Credentialsaws    bob")

	out = strings.Replace(s.listCredentials(c, "--client", "--format", "yaml"), "\n", "", -1)
	c.Assert(out, gc.Equals, "client-credentials:  aws:    bob:      auth-type: oauth2")

	out = strings.Replace(s.listCredentials(c, "--client", "--format", "json"), "\n", "", -1)
	c.Assert(out, gc.Equals, `{"client-credentials":{"aws":{"cloud-credentials":{"bob":{"auth-type":"oauth2"}}}}}`)
}

func (s *listCredentialsSuite) TestListCredentialsNone(c *gc.C) {
	listCmd := cloud.NewListCredentialsCommandForTest(jujuclient.NewMemStore(), s.personalCloudsFunc, s.cloudByNameFunc, s.apiF)
	ctx, err := cmdtesting.RunCommand(c, listCmd, "--client")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	out := strings.Replace(cmdtesting.Stdout(ctx), "\n", "", -1)
	c.Assert(out, gc.Equals, "No credentials from this client to display.")

	ctx, err = cmdtesting.RunCommand(c, listCmd, "--client", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	out = strings.Replace(cmdtesting.Stdout(ctx), "\n", "", -1)
	c.Assert(out, gc.Equals, "{}")

	ctx, err = cmdtesting.RunCommand(c, listCmd, "--client", "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	out = strings.Replace(cmdtesting.Stdout(ctx), "\n", "", -1)
	c.Assert(out, gc.Equals, `{}`)
}

func (s *listCredentialsSuite) listCredentials(c *gc.C, args ...string) string {
	ctx := s.listCredentialsWithStore(c, s.store, args...)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	return cmdtesting.Stdout(ctx)
}

func (s *listCredentialsSuite) listCredentialsWithStore(c *gc.C, store jujuclient.ClientStore, args ...string) *cmd.Context {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListCredentialsCommandForTest(store, s.personalCloudsFunc, s.cloudByNameFunc, s.apiF), args...)
	c.Assert(err, jc.ErrorIsNil)
	return ctx
}

type mockAPI struct {
	jujutesting.Stub

	credentialContentsF func(cloud, credential string, withSecrets bool) ([]params.CredentialContentResult, error)
}

func (m *mockAPI) CredentialContents(cloud, credential string, withSecrets bool) ([]params.CredentialContentResult, error) {
	m.AddCall("CredentialContents", cloud, credential, withSecrets)
	return m.credentialContentsF(cloud, credential, withSecrets)
}

func (m *mockAPI) Close() error {
	m.AddCall("Close")
	return nil
}
