// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"io/ioutil"
	"os"
	"strings"

	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

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
	authCreds := map[string]string{"access-key": "key", "secret-key": "secret"}
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
			"aws": {
				AuthCredentials: map[string]jujucloud.Credential{
					"my-credential":      jujucloud.NewCredential(jujucloud.AccessKeyAuthType, authCreds),
					"another-credential": jujucloud.NewCredential(jujucloud.UserPassAuthType, authCreds),
				},
			},
		},
	}
	fake := &fakeUpdateCredentialAPI{}
	cmd := cloud.NewUpdateCredentialCommandForTest(store, fake)
	ctx, err := cmdtesting.RunCommand(c, cmd, "aws", "my-credential")
	c.Assert(err, jc.ErrorIsNil)
	output := cmdtesting.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	c.Assert(output, gc.Equals, `Updated credential "my-credential" for user "admin@local" on cloud "aws".`)
	c.Assert(fake.creds, jc.DeepEquals, map[names.CloudCredentialTag]jujucloud.Credential{
		names.NewCloudCredentialTag("aws/admin@local/my-credential"): jujucloud.NewCredential(jujucloud.AccessKeyAuthType, map[string]string{"access-key": "key", "secret-key": "secret"}),
	})
}

func (s *updateCredentialSuite) TestUpdateCredentialWithFilePath(c *gc.C) {
	tmpFile, err := ioutil.TempFile("", "juju-bootstrap-test")
	c.Assert(err, jc.ErrorIsNil)
	defer func() { err := os.Remove(tmpFile.Name()); c.Assert(err, jc.ErrorIsNil) }()

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

	fake := &fakeUpdateCredentialAPI{}
	cmd := cloud.NewUpdateCredentialCommandForTest(store, fake)
	_, err = cmdtesting.RunCommand(c, cmd, "google", "gce")
	c.Assert(err, jc.ErrorIsNil)

	tag := names.NewCloudCredentialTag("google/admin@local/gce")
	expectedFileContents := fake.creds[tag].Attributes()["file"]
	c.Assert(expectedFileContents, gc.Equals, string(contents))
}

type fakeUpdateCredentialAPI struct {
	creds map[names.CloudCredentialTag]jujucloud.Credential
}

func (f *fakeUpdateCredentialAPI) UpdateCredential(tag names.CloudCredentialTag, credential jujucloud.Credential) error {
	if f.creds == nil {
		f.creds = make(map[names.CloudCredentialTag]jujucloud.Credential)
	}
	f.creds[tag] = credential
	return nil
}

func (*fakeUpdateCredentialAPI) Close() error {
	return nil
}
