// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"errors"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/names/v4"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type removeCredentialSuite struct {
	testing.BaseSuite

	cloudByNameFunc func(string) (*jujucloud.Cloud, error)
	clientF         func() (cloud.RemoveCredentialAPI, error)
	fakeClient      *fakeRemoveCredentialAPI
}

var _ = gc.Suite(&removeCredentialSuite{})

func (s *removeCredentialSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.cloudByNameFunc = jujucloud.CloudByName
	s.fakeClient = &fakeRemoveCredentialAPI{
		clouds: func() (map[names.CloudTag]jujucloud.Cloud, error) { return nil, nil },
	}
	s.clientF = func() (cloud.RemoveCredentialAPI, error) {
		return s.fakeClient, nil
	}
}

func (s *removeCredentialSuite) TestBadArgs(c *gc.C) {
	command := cloud.NewRemoveCredentialCommand()
	_, err := cmdtesting.RunCommand(c, command)
	c.Assert(err, gc.ErrorMatches, "Usage: juju remove-credential <cloud-name> <credential-name>")
	_, err = cmdtesting.RunCommand(c, command, "cloud", "credential", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *removeCredentialSuite) TestMissingLocalCredential(c *gc.C) {
	store := &jujuclient.MemStore{
		Credentials: map[string]jujucloud.CloudCredential{
			"aws": {
				AuthCredentials: map[string]jujucloud.Credential{
					"my-credential": jujucloud.NewCredential(jujucloud.AccessKeyAuthType, nil),
				},
			},
		},
	}
	command := cloud.NewRemoveCredentialCommandForTest(store, s.cloudByNameFunc, s.clientF)
	ctx, err := cmdtesting.RunCommand(c, command, "aws", "foo", "--client")
	c.Assert(err, jc.ErrorIsNil)
	output := cmdtesting.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	c.Assert(output, gc.Equals, `Found local cloud "aws" on this client.No credential called "foo" exists for cloud "aws" on this client`)
}

func (s *removeCredentialSuite) TestBadLocalCloudName(c *gc.C) {
	command := cloud.NewRemoveCredentialCommandForTest(jujuclient.NewMemStore(), s.cloudByNameFunc, s.clientF)
	ctx, err := cmdtesting.RunCommand(c, command, "somecloud", "foo", "--client")
	c.Assert(err, gc.DeepEquals, cmd.ErrSilent)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
No cloud "somecloud" is found.
To view all available clouds, use 'juju clouds'.
To add new cloud, use 'juju add-cloud'.
`[1:])
	c.Assert(c.GetTestLog(), jc.Contains, "cloud somecloud not valid")
}

func (s *removeCredentialSuite) TestRemove(c *gc.C) {
	store := &jujuclient.MemStore{
		Credentials: map[string]jujucloud.CloudCredential{
			"aws": {
				AuthCredentials: map[string]jujucloud.Credential{
					"my-credential":      jujucloud.NewCredential(jujucloud.AccessKeyAuthType, nil),
					"another-credential": jujucloud.NewCredential(jujucloud.AccessKeyAuthType, nil),
				},
			},
		},
	}
	command := cloud.NewRemoveCredentialCommandForTest(store, s.cloudByNameFunc, s.clientF)
	ctx, err := cmdtesting.RunCommand(c, command, "aws", "my-credential", "--client")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
Found local cloud "aws" on this client.
Credential "my-credential" for cloud "aws" removed from this client.
`[1:])
	_, stillThere := store.Credentials["aws"].AuthCredentials["my-credential"]
	c.Assert(stillThere, jc.IsFalse)
	c.Assert(store.Credentials["aws"].AuthCredentials, gc.HasLen, 1)
}

func (s *removeCredentialSuite) setupStore(c *gc.C) *jujuclient.MemStore {
	store := jujuclient.NewMemStore()
	store.Controllers["controller"] = jujuclient.ControllerDetails{ControllerUUID: "cdcssc"}
	store.CurrentControllerName = "controller"
	store.Accounts = map[string]jujuclient.AccountDetails{
		"controller": {
			User: "admin@local",
		},
	}
	store.Credentials = map[string]jujucloud.CloudCredential{
		"aws": {
			AuthCredentials: map[string]jujucloud.Credential{
				"my-credential": jujucloud.NewCredential(jujucloud.AccessKeyAuthType, nil),
			},
		},
	}
	return store
}

func (s *removeCredentialSuite) TestGettingApiClientError(c *gc.C) {
	store := s.setupStore(c)
	s.clientF = func() (cloud.RemoveCredentialAPI, error) { return s.fakeClient, errors.New("kaboom") }
	command := cloud.NewRemoveCredentialCommandForTest(store, s.cloudByNameFunc, s.clientF)
	_, err := cmdtesting.RunCommand(c, command, "aws", "foo", "-c", "controller")
	c.Assert(err, gc.ErrorMatches, "kaboom")
	s.fakeClient.CheckNoCalls(c)
}

func (s *removeCredentialSuite) TestGettingApiClientErrorButLocal(c *gc.C) {
	store := s.setupStore(c)
	s.clientF = func() (cloud.RemoveCredentialAPI, error) { return s.fakeClient, errors.New("kaboom") }
	command := cloud.NewRemoveCredentialCommandForTest(store, s.cloudByNameFunc, s.clientF)
	_, err := cmdtesting.RunCommand(c, command, "aws", "foo", "--client")
	c.Assert(err, jc.ErrorIsNil)
	s.fakeClient.CheckNoCalls(c)
}

func (s *removeCredentialSuite) setupClientForRemote(c *gc.C) {
	s.fakeClient.clouds = func() (map[names.CloudTag]jujucloud.Cloud, error) {
		return map[names.CloudTag]jujucloud.Cloud{
			names.NewCloudTag("somecloud"): {
				Name:             "somecloud",
				Type:             "mock-addcredential-provider",
				AuthTypes:        []jujucloud.AuthType{jujucloud.AccessKeyAuthType},
				Endpoint:         "cloud-endpoint",
				IdentityEndpoint: "cloud-identity-endpoint",
			},
		}, nil
	}
}

func (s *removeCredentialSuite) TestBadRemoteCloudName(c *gc.C) {
	store := s.setupStore(c)
	s.setupClientForRemote(c)
	command := cloud.NewRemoveCredentialCommandForTest(store, s.cloudByNameFunc, s.clientF)
	ctx, err := cmdtesting.RunCommand(c, command, "other", "foo", "-c", "controller")
	c.Assert(err, gc.DeepEquals, cmd.ErrSilent)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
No cloud "other" is found.
To view all available clouds, use 'juju clouds'.
To add new cloud, use 'juju add-cloud'.
`[1:])
}

func (s *removeCredentialSuite) TestRemoveRemoteCredential(c *gc.C) {
	store := s.setupStore(c)
	s.setupClientForRemote(c)
	s.fakeClient.revokeCredentialF = func(tag names.CloudCredentialTag) error {
		c.Assert(tag.String(), gc.DeepEquals, "cloudcred-somecloud_admin_foo")
		return nil
	}
	command := cloud.NewRemoveCredentialCommandForTest(store, s.cloudByNameFunc, s.clientF)
	ctx, err := cmdtesting.RunCommand(c, command, "somecloud", "foo", "-c", "controller")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
Found remote cloud "somecloud" from the controller.
Credential "foo" for cloud "somecloud" removed from the controller "controller".
`[1:])
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}

func (s *removeCredentialSuite) TestRemoveRemoteCredentialFail(c *gc.C) {
	store := s.setupStore(c)
	s.setupClientForRemote(c)
	s.fakeClient.revokeCredentialF = func(tag names.CloudCredentialTag) error {
		return errors.New("kaboom")
	}
	command := cloud.NewRemoveCredentialCommandForTest(store, s.cloudByNameFunc, s.clientF)
	ctx, err := cmdtesting.RunCommand(c, command, "somecloud", "foo", "-c", "controller")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "Found remote cloud \"somecloud\" from the controller.\nERROR could not remove remote credential: kaboom\n")
	s.fakeClient.CheckCallNames(c, "Clouds", "RevokeCredential", "Close")
	s.fakeClient.CheckCall(c, 1, "RevokeCredential", names.NewCloudCredentialTag("somecloud/admin/foo"), false)
}

func (s *removeCredentialSuite) TestRemoveRemoteCredentialForce(c *gc.C) {
	store := s.setupStore(c)
	s.setupClientForRemote(c)
	s.fakeClient.revokeCredentialF = func(tag names.CloudCredentialTag) error {
		return nil
	}
	command := cloud.NewRemoveCredentialCommandForTest(store, s.cloudByNameFunc, s.clientF)
	_, err := cmdtesting.RunCommand(c, command, "somecloud", "foo", "-c", "controller", "--force")
	c.Assert(err, jc.ErrorIsNil)
	s.fakeClient.CheckCallNames(c, "Clouds", "RevokeCredential", "Close")
	s.fakeClient.CheckCall(c, 1, "RevokeCredential", names.NewCloudCredentialTag("somecloud/admin/foo"), true)
}

type fakeRemoveCredentialAPI struct {
	jujutesting.Stub
	v                 int
	revokeCredentialF func(tag names.CloudCredentialTag) error
	clouds            func() (map[names.CloudTag]jujucloud.Cloud, error)
}

func (f *fakeRemoveCredentialAPI) Close() error {
	f.AddCall("Close")
	return nil
}

func (f *fakeRemoveCredentialAPI) BestAPIVersion() int {
	f.AddCall("BestAPIVersion")
	return f.v
}

func (f *fakeRemoveCredentialAPI) RevokeCredential(c names.CloudCredentialTag, force bool) error {
	f.AddCall("RevokeCredential", c, force)
	return f.revokeCredentialF(c)
}

func (f *fakeRemoveCredentialAPI) Clouds() (map[names.CloudTag]jujucloud.Cloud, error) {
	f.AddCall("Clouds")
	return f.clouds()
}
