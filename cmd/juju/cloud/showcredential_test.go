// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type ShowCredentialSuite struct {
	coretesting.BaseSuite

	api   *fakeCredentialContentAPI
	store *jujuclient.MemStore
}

var _ = gc.Suite(&ShowCredentialSuite{})

func (s *ShowCredentialSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.store = &jujuclient.MemStore{
		Controllers: map[string]jujuclient.ControllerDetails{
			"controller": {},
		},
		CurrentControllerName: "controller",
	}
	s.api = &fakeCredentialContentAPI{v: 2}
}

func (s *ShowCredentialSuite) putCredentialsInStore(c *gc.C) {
	authCreds := map[string]string{"access-key": "key", "secret-key": "secret"}
	s.store.Accounts = map[string]jujuclient.AccountDetails{
		"controller": {
			User: "admin@local",
		},
	}
	s.store.Credentials = map[string]jujucloud.CloudCredential{
		"aws": {
			AuthCredentials: map[string]jujucloud.Credential{
				"my-credential": jujucloud.NewCredential(jujucloud.AccessKeyAuthType, authCreds),
			},
		},
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"its-credential":         jujucloud.NewCredential(jujucloud.AccessKeyAuthType, authCreds),
				"its-another-credential": jujucloud.NewCredential(jujucloud.AccessKeyAuthType, authCreds),
			},
		},
	}
}

func (s *ShowCredentialSuite) TestShowCredentialBadArgs(c *gc.C) {
	cmd := cloud.NewShowCredentialCommandForTest(s.store, s.api)
	_, err := cmdtesting.RunCommand(c, cmd, "cloud")
	c.Assert(err, gc.ErrorMatches, "both cloud and credential name are needed")
	_, err = cmdtesting.RunCommand(c, cmd, "cloud", "credential", "extra")
	c.Assert(err, gc.ErrorMatches, `only cloud and credential names are supported`)
}

func (s *ShowCredentialSuite) TestShowCredentialAPICallError(c *gc.C) {
	s.api.SetErrors(errors.New("boom"), nil)
	cmd := cloud.NewShowCredentialCommandForTest(s.store, s.api)
	ctx, err := cmdtesting.RunCommand(c, cmd, "-c", "controller")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
ERROR credential content lookup on the controller failed: boom
No credentials from this client or from a controller to display.
`[1:])
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, ``)
	s.api.CheckCallNames(c, "CredentialContents", "Close")
}

func (s *ShowCredentialSuite) TestShowCredentialNone(c *gc.C) {
	s.api.contents = []params.CredentialContentResult{}
	cmd := cloud.NewShowCredentialCommandForTest(s.store, s.api)
	ctx, err := cmdtesting.RunCommand(c, cmd, "-c", "controller")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "No credentials from this client or from a controller to display.\n")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, ``)
	s.api.CheckCallNames(c, "CredentialContents", "Close")
}

func (s *ShowCredentialSuite) TestShowCredentialBothClientAndController(c *gc.C) {
	_true := true
	s.putCredentialsInStore(c)
	s.api.contents = []params.CredentialContentResult{
		{
			Result: &params.ControllerCredentialInfo{
				Content: params.CredentialContent{
					Cloud:    "aws",
					Name:     "credential-name",
					AuthType: "userpass",
					Valid:    &_true,
					Attributes: map[string]string{
						"username": "fred",
						"password": "sekret"},
				},
				Models: []params.ModelAccess{
					{Model: "abcmodel", Access: "admin"},
					{Model: "xyzmodel", Access: "read"},
					{Model: "no-access-model"},
				},
			},
		},
	}
	cmd := cloud.NewShowCredentialCommandForTest(s.store, s.api)
	ctx, err := cmdtesting.RunCommand(c, cmd, "--show-secrets")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, ``)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
controller-credentials:
  aws:
    credential-name:
      content:
        auth-type: userpass
        validity-check: valid
        password: sekret
        username: fred
      models:
        abcmodel: admin
        no-access-model: no access
        xyzmodel: read
client-credentials:
  aws:
    my-credential:
      content:
        auth-type: access-key
        access-key: key
        secret-key: secret
  somecloud:
    its-another-credential:
      content:
        auth-type: access-key
        access-key: key
        secret-key: secret
    its-credential:
      content:
        auth-type: access-key
        access-key: key
        secret-key: secret
`[1:])
	s.api.CheckCallNames(c, "CredentialContents", "Close")
	c.Assert(s.api.inclsecrets, jc.IsTrue)
}

func (s *ShowCredentialSuite) TestShowCredentialMany(c *gc.C) {
	s.putCredentialsInStore(c)
	_true := true
	_false := false
	s.api.contents = []params.CredentialContentResult{
		{
			Result: &params.ControllerCredentialInfo{
				Content: params.CredentialContent{
					Cloud:      "cloud-name",
					Name:       "one",
					AuthType:   "userpass",
					Valid:      &_true,
					Attributes: map[string]string{"username": "fred"},
				},
				// Don't have models here.
			},
		}, {
			Error: apiservererrors.ServerError(errors.New("boom")),
		}, {
			Result: &params.ControllerCredentialInfo{
				Content: params.CredentialContent{
					Cloud:    "cloud-name",
					Name:     "two",
					AuthType: "userpass",
					Valid:    &_false,
					Attributes: map[string]string{
						"username":  "fred",
						"something": "visible-attr",
						"password":  "sekret",
						"hidden":    "very-very-sekret",
					},
				},
				Models: []params.ModelAccess{
					{Model: "abcmodel", Access: "admin"},
					{Model: "xyzmodel", Access: "read"},
					{Model: "no-access-model"},
				},
			},
		}, {
			Result: &params.ControllerCredentialInfo{
				Content: params.CredentialContent{
					Cloud:    "diff-cloud",
					Name:     "three",
					AuthType: "oauth1",
					Valid:    &_true,
					Attributes: map[string]string{
						"something": "visible-attr",
					},
				},
				Models: []params.ModelAccess{
					{Model: "klmmodel", Access: "write"},
				},
			},
		},
	}
	cmd := cloud.NewShowCredentialCommandForTest(s.store, s.api)
	ctx, err := cmdtesting.RunCommand(c, cmd, "-c", "controller")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "boom\n")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
controller-credentials:
  cloud-name:
    one:
      content:
        auth-type: userpass
        validity-check: valid
        username: fred
    two:
      content:
        auth-type: userpass
        validity-check: invalid
        hidden: very-very-sekret
        password: sekret
        something: visible-attr
        username: fred
      models:
        abcmodel: admin
        no-access-model: no access
        xyzmodel: read
  diff-cloud:
    three:
      content:
        auth-type: oauth1
        validity-check: valid
        something: visible-attr
      models:
        klmmodel: write
`[1:])
	s.api.CheckCallNames(c, "CredentialContents", "Close")
}

type fakeCredentialContentAPI struct {
	testing.Stub
	v           int
	contents    []params.CredentialContentResult
	inclsecrets bool
}

func (f *fakeCredentialContentAPI) CredentialContents(cloud, credential string, withSecrets bool) ([]params.CredentialContentResult, error) {
	f.AddCall("CredentialContents", cloud, credential, withSecrets)
	f.inclsecrets = withSecrets
	return f.contents, f.NextErr()
}

func (f *fakeCredentialContentAPI) Close() error {
	f.AddCall("Close")
	return f.NextErr()
}
