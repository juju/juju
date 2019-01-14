// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"io/ioutil"
	"os"
	"strings"

	jujucmd "github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type updateCredentialSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&updateCredentialSuite{})

func (s *updateCredentialSuite) TestBadArgs(c *gc.C) {
	store := &jujuclient.MemStore{
		Controllers: map[string]jujuclient.ControllerDetails{
			"controller": {},
		},
		CurrentControllerName: "controller",
	}
	cmd := cloud.NewUpdateCredentialCommandForTest(store, nil)
	_, err := cmdtesting.RunCommand(c, cmd)
	c.Assert(err, gc.ErrorMatches, "Usage: juju update-credential <cloud-name> <credential-name>")
	_, err = cmdtesting.RunCommand(c, cmd, "cloud", "credential", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *updateCredentialSuite) TestMissingCredential(c *gc.C) {
	store := &jujuclient.MemStore{
		Controllers: map[string]jujuclient.ControllerDetails{
			"controller": {},
		},
		CurrentControllerName: "controller",
		Credentials: map[string]jujucloud.CloudCredential{
			"aws": {
				AuthCredentials: map[string]jujucloud.Credential{
					"my-credential": jujucloud.NewCredential(jujucloud.AccessKeyAuthType, nil),
				},
			},
		},
	}
	cmd := cloud.NewUpdateCredentialCommandForTest(store, nil)
	ctx, err := cmdtesting.RunCommand(c, cmd, "aws", "foo")
	c.Assert(err, jc.ErrorIsNil)
	output := cmdtesting.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	c.Assert(output, gc.Equals, `No credential called "foo" exists for cloud "aws"`)
}

func (s *updateCredentialSuite) TestBadCloudName(c *gc.C) {
	store := &jujuclient.MemStore{
		Controllers: map[string]jujuclient.ControllerDetails{
			"controller": {},
		},
		CurrentControllerName: "controller",
	}
	cmd := cloud.NewUpdateCredentialCommandForTest(store, nil)
	ctx, err := cmdtesting.RunCommand(c, cmd, "somecloud", "foo")
	c.Assert(err, jc.ErrorIsNil)
	output := cmdtesting.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	c.Assert(output, gc.Equals, `Cloud "somecloud" not found`)
}

func (s *updateCredentialSuite) TestUpdate(c *gc.C) {
	fake := &fakeUpdateCredentialAPI{
		updateCredentialsCheckModelsF: func(tag names.CloudCredentialTag, credential jujucloud.Credential) ([]params.UpdateCredentialModelResult, error) {
			c.Assert(tag, gc.DeepEquals, names.NewCloudCredentialTag("aws/admin@local/my-credential"))
			c.Assert(credential, jc.DeepEquals, jujucloud.NewCredential(jujucloud.AccessKeyAuthType, map[string]string{"access-key": "key", "secret-key": "secret"}))
			return nil, nil
		},
	}

	cmd := cloud.NewUpdateCredentialCommandForTest(s.store(c), fake)
	ctx, err := cmdtesting.RunCommand(c, cmd, "aws", "my-credential")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
Controller credential "my-credential" for user "admin@local" on cloud "aws" updated.
For more information, see ‘juju show-credential aws my-credential’.
`[1:])
}

func (s *updateCredentialSuite) TestUpdateCredentialWithFilePath(c *gc.C) {
	tmpFile, err := ioutil.TempFile("", "juju-bootstrap-test")
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		tmpFile.Close()
		err := os.Remove(tmpFile.Name())
		c.Assert(err, jc.ErrorIsNil)
	}()

	store := &jujuclient.MemStore{
		Controllers: map[string]jujuclient.ControllerDetails{
			"controller": {},
		},
		CurrentControllerName: "controller",
		Accounts: map[string]jujuclient.AccountDetails{
			"controller": {
				User: "admin@local",
			},
		},
		Credentials: map[string]jujucloud.CloudCredential{
			"google": {
				AuthCredentials: map[string]jujucloud.Credential{
					"gce": jujucloud.NewCredential(
						jujucloud.JSONFileAuthType,
						map[string]string{"file": tmpFile.Name()},
					),
				},
			},
		},
	}

	contents := []byte("{something: special}\n")
	err = ioutil.WriteFile(tmpFile.Name(), contents, 0644)
	c.Assert(err, jc.ErrorIsNil)

	fake := &fakeUpdateCredentialAPI{
		updateCredentialsCheckModelsF: func(tag names.CloudCredentialTag, credential jujucloud.Credential) ([]params.UpdateCredentialModelResult, error) {
			c.Assert(tag, gc.DeepEquals, names.NewCloudCredentialTag("google/admin@local/gce"))
			c.Assert(credential.Attributes()["file"], gc.Equals, string(contents))
			return nil, nil
		},
	}
	cmd := cloud.NewUpdateCredentialCommandForTest(store, fake)
	_, err = cmdtesting.RunCommand(c, cmd, "google", "gce")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *updateCredentialSuite) store(c *gc.C) jujuclient.ClientStore {
	authCreds := map[string]string{"access-key": "key", "secret-key": "secret"}
	return &jujuclient.MemStore{
		Controllers: map[string]jujuclient.ControllerDetails{
			"controller": {},
		},
		CurrentControllerName: "controller",
		Accounts: map[string]jujuclient.AccountDetails{
			"controller": {
				User: "admin@local",
			},
		},
		Credentials: map[string]jujucloud.CloudCredential{
			"aws": {
				AuthCredentials: map[string]jujucloud.Credential{
					"my-credential": jujucloud.NewCredential(jujucloud.AccessKeyAuthType, authCreds),
				},
			},
		},
	}
}

func (s *updateCredentialSuite) TestUpdateResultError(c *gc.C) {
	fake := &fakeUpdateCredentialAPI{
		updateCredentialsCheckModelsF: func(tag names.CloudCredentialTag, credential jujucloud.Credential) ([]params.UpdateCredentialModelResult, error) {
			return nil, errors.New("kaboom")
		},
	}
	cmd := cloud.NewUpdateCredentialCommandForTest(s.store(c), fake)
	ctx, err := cmdtesting.RunCommand(c, cmd, "aws", "my-credential")
	c.Assert(err, gc.NotNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "Controller credential \"my-credential\" for user \"admin@local\" on cloud \"aws\" not updated: kaboom.\n")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}
func (s *updateCredentialSuite) TestUpdateWithModels(c *gc.C) {
	fake := &fakeUpdateCredentialAPI{
		updateCredentialsCheckModelsF: func(tag names.CloudCredentialTag, credential jujucloud.Credential) ([]params.UpdateCredentialModelResult, error) {
			return []params.UpdateCredentialModelResult{
				{
					ModelName: "model-a",
					Errors: []params.ErrorResult{
						{common.ServerError(errors.New("kaboom"))},
						{common.ServerError(errors.New("kaboom 2"))},
					},
				},
				{
					ModelName: "model-b",
					Errors: []params.ErrorResult{
						{common.ServerError(errors.New("one failure"))},
					},
				},
				{
					ModelName: "model-c",
				},
			}, errors.New("models issues")
		},
	}
	cmd := cloud.NewUpdateCredentialCommandForTest(s.store(c), fake)
	ctx, err := cmdtesting.RunCommand(c, cmd, "aws", "my-credential")
	c.Assert(err, gc.DeepEquals, jujucmd.ErrSilent)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
Credential valid for:
  model-c
Credential invalid for:
  model-a:
    kaboom
    kaboom 2
  model-b:
    one failure
Controller credential "my-credential" for user "admin@local" on cloud "aws" not updated: models issues.
`[1:])
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}

type fakeUpdateCredentialAPI struct {
	updateCredentialsCheckModelsF func(tag names.CloudCredentialTag, credential jujucloud.Credential) ([]params.UpdateCredentialModelResult, error)
}

func (f *fakeUpdateCredentialAPI) UpdateCredentialsCheckModels(tag names.CloudCredentialTag, credential jujucloud.Credential) ([]params.UpdateCredentialModelResult, error) {
	return f.updateCredentialsCheckModelsF(tag, credential)
}

func (*fakeUpdateCredentialAPI) Close() error {
	return nil
}
