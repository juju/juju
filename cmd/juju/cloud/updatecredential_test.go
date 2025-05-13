// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	jujucmd "github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

type updateCredentialSuite struct {
	testing.FakeJujuXDGDataHomeSuite

	store       *jujuclient.MemStore
	testCommand jujucmd.Command
	api         *fakeUpdateCredentialAPI
}

var _ = tc.Suite(&updateCredentialSuite{})

func (s *updateCredentialSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.store = &jujuclient.MemStore{
		Controllers: map[string]jujuclient.ControllerDetails{
			"controller": {},
		},
		CurrentControllerName: "controller",
	}
	s.api = &fakeUpdateCredentialAPI{}
	s.testCommand = cloud.NewUpdateCredentialCommandForTest(s.store, s.api)
}

func (s *updateCredentialSuite) TestBadArgs(c *tc.C) {
	_, err := cmdtesting.RunCommand(c, s.testCommand, "cloud", "credential", "extra")
	c.Assert(err, tc.ErrorMatches, `only a cloud name and / or credential name need to be provided`)
}

func (s *updateCredentialSuite) TestNoArgs(c *tc.C) {
	_, err := cmdtesting.RunCommand(c, s.testCommand)
	c.Assert(err, tc.ErrorMatches, regexp.QuoteMeta(`Usage: juju update-credential [options] [<cloud-name> [<credential-name>]]`))
}

func (s *updateCredentialSuite) TestBadFileSpecified(c *tc.C) {
	ctx, err := cmdtesting.RunCommand(c, s.testCommand, "-f", "somefile.yaml")
	c.Assert(err, tc.NotNil)
	c.Assert(err.Error(), tc.Contains, "could not get credentials from file: reading credentials file: open somefile.yaml")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
}

func (s *updateCredentialSuite) makeCredentialsTestFile(c *tc.C, data string) string {
	dir := c.MkDir()
	credsFile := filepath.Join(dir, "cred.yaml")
	err := os.WriteFile(credsFile, []byte(data), 0644)
	c.Assert(err, tc.IsNil)
	return credsFile
}

func (s *updateCredentialSuite) TestFileSpecified(c *tc.C) {
	testFile := s.makeCredentialsTestFile(c, `
credentials:
  somecloud:
    its-credential:
      auth-type: interactive
      trust-password: "123"
  anothercloud:
    its-credential-too:
      auth-type: interactive
      trust-password: "123"
`)
	result, err := cloud.CredentialsFromFile(testFile, "", "")
	c.Assert(err, tc.ErrorIsNil)

	one := jujucloud.NewCredential(jujucloud.InteractiveAuthType, map[string]string{"trust-password": "123"})
	assertFoundCredentials(c, result, map[string]map[string]jujucloud.Credential{
		"somecloud":    {"its-credential": one},
		"anothercloud": {"its-credential-too": one},
	})
}

func (s *updateCredentialSuite) TestFileSpecifiedWithCloud(c *tc.C) {
	testFile := s.makeCredentialsTestFile(c, `
credentials:
  somecloud:
    its-credential:
      auth-type: interactive
      trust-password: "123"
    its-other-credential:
      auth-type: interactive
      trust-password: "123"
  anothercloud:
    its-credential:
      auth-type: interactive
      trust-password: "123"
`)
	cloudName := "somecloud"
	result, err := cloud.CredentialsFromFile(testFile, cloudName, "")
	c.Assert(err, tc.ErrorIsNil)

	one := jujucloud.NewCredential(jujucloud.InteractiveAuthType, map[string]string{"trust-password": "123"})
	assertFoundCredentials(c, result, map[string]map[string]jujucloud.Credential{
		cloudName: {
			"its-other-credential": one,
			"its-credential":       one,
		},
	})
}

func (s *updateCredentialSuite) TestFileSpecifiedButHasNoDesiredCloud(c *tc.C) {
	testFile := s.makeCredentialsTestFile(c, `
credentials:
  somecloud:
    its-credential:
      auth-type: interactive
      trust-password: "123"
`)
	result, err := cloud.CredentialsFromFile(testFile, "anothercloud", "")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
	c.Assert(result, tc.IsNil)
}

func (s *updateCredentialSuite) TestFileSpecifiedWithCloudAndCredential(c *tc.C) {
	testFile := s.makeCredentialsTestFile(c, `
credentials:
  somecloud:
    its-credential:
      auth-type: interactive
      trust-password: "123"
    its-other-credential:
      auth-type: interactive
      trust-password: "123"
  anothercloud:
    its-credential:
      auth-type: interactive
      trust-password: "123"
    its-other-credential:
      auth-type: interactive
      trust-password: "123"
`)
	cloudName := "somecloud"
	result, err := cloud.CredentialsFromFile(testFile, cloudName, "its-credential")
	c.Assert(err, tc.ErrorIsNil)

	one := jujucloud.NewCredential(jujucloud.InteractiveAuthType, map[string]string{"trust-password": "123"})
	assertFoundCredentials(c, result, map[string]map[string]jujucloud.Credential{cloudName: {"its-credential": one}})
}

func (s *updateCredentialSuite) TestFileSpecifiedWithCloudAndCredentialNotFound(c *tc.C) {
	testFile := s.makeCredentialsTestFile(c, `
credentials:
  somecloud:
    its-other-credential:
      auth-type: interactive
      trust-password: "123"
`)
	result, err := cloud.CredentialsFromFile(testFile, "somecloud", "its-credential")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
	c.Assert(result, tc.IsNil)
}

func (s *updateCredentialSuite) TestFileSpecifiedWithCloudAndCredentialInDifferentCloud(c *tc.C) {
	testFile := s.makeCredentialsTestFile(c, `
credentials:
  somecloud:
    its-other-credential:
      auth-type: interactive
      trust-password: "123"
  anothercloud:
    its-credential:
      auth-type: interactive
      trust-password: "123"
`)
	result, err := cloud.CredentialsFromFile(testFile, "somecloud", "its-credential")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
	c.Assert(result, tc.IsNil)
}

func (s *updateCredentialSuite) TestFileSpecifiedNoDesiredCloudAndCredential(c *tc.C) {
	testFile := s.makeCredentialsTestFile(c, `
credentials:
  anothercloud:
    its-another-credential:
      auth-type: interactive
      trust-password: "123"
`)
	result, err := cloud.CredentialsFromFile(testFile, "somecloud", "its-credential")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
	c.Assert(result, tc.IsNil)
}

func (s *updateCredentialSuite) TestCloudCredentialFromLocalCacheNoneFound(c *tc.C) {
	result, err := cloud.CredentialsFromLocalCache(s.store, "anothercloud", "its-credential")
	c.Assert(err, tc.ErrorMatches, `loading credentials: credentials for cloud anothercloud not found`)
	c.Assert(result, tc.IsNil)
}

func (s *updateCredentialSuite) TestCloudCredentialFromLocalCacheWithCloud(c *tc.C) {
	s.storeWithCredentials(c)
	cloudName := "somecloud"
	result, err := cloud.CredentialsFromLocalCache(s.store, cloudName, "")
	c.Assert(err, tc.ErrorIsNil)

	one := jujucloud.NewCredential(jujucloud.AccessKeyAuthType, map[string]string{
		"access-key": "key",
		"secret-key": "secret"})
	assertFoundCredentials(c, result, map[string]map[string]jujucloud.Credential{
		cloudName: {
			"its-another-credential": one,
			"its-credential":         one,
		},
	})
}

func assertFoundCredentials(c *tc.C, found map[string]jujucloud.CloudCredential, expected map[string]map[string]jujucloud.Credential) {
	// jujucloud.Credential has some unexported fields so we cannot compare structs.
	c.Assert(len(found), tc.Equals, len(expected))
	for foundCloudName, foundCloudCredentials := range found {
		expectedCloudCredentials, ok := expected[foundCloudName]
		c.Assert(ok, tc.IsTrue)
		c.Assert(len(foundCloudCredentials.AuthCredentials), tc.Equals, len(expectedCloudCredentials))
		for foundCredentialName, foundCredential := range foundCloudCredentials.AuthCredentials {
			expectedCredential, ok := expectedCloudCredentials[foundCredentialName]
			c.Assert(ok, tc.IsTrue)
			c.Assert(foundCredential.AuthType(), tc.DeepEquals, expectedCredential.AuthType())
			c.Assert(foundCredential.Attributes(), tc.DeepEquals, expectedCredential.Attributes())
		}
	}
}

func (s *updateCredentialSuite) TestCloudCredentialFromLocalCacheWithCloudAndCredential(c *tc.C) {
	s.storeWithCredentials(c)
	cloudName := "somecloud"
	credentialName := "its-credential"
	result, err := cloud.CredentialsFromLocalCache(s.store, cloudName, credentialName)
	c.Assert(err, tc.ErrorIsNil)

	one := jujucloud.NewCredential(jujucloud.AccessKeyAuthType, map[string]string{
		"access-key": "key",
		"secret-key": "secret"})
	assertFoundCredentials(c, result, map[string]map[string]jujucloud.Credential{
		cloudName: {
			"its-credential": one,
		},
	})
}

func (s *updateCredentialSuite) TestUpdateLocalWithCloudWhenNoneExists(c *tc.C) {
	ctx, err := cmdtesting.RunCommand(c, s.testCommand, "somecloud", "its-credential", "--client")
	c.Assert(err, tc.ErrorMatches, "could not get credentials from local client: loading credentials: credentials for cloud somecloud not found")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
}

func (s *updateCredentialSuite) TestUpdateLocalWithCloudWhenCredentialDoesNotExists(c *tc.C) {
	s.storeWithCredentials(c)
	ctx, err := cmdtesting.RunCommand(c, s.testCommand, "somecloud", "fluffy-credential", "--client")
	c.Assert(err, tc.ErrorMatches, `could not get credentials from local client: credential "fluffy-credential" for cloud "somecloud" in local client not found`)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
}

func (s *updateCredentialSuite) TestUpdateLocal(c *tc.C) {
	testFile := s.makeCredentialsTestFile(c, `
credentials:
  somecloud:
    its-credential:
      auth-type: access-key
      access-key: "555"
      secret-key: "once upon a juju"
`)
	s.storeWithCredentials(c)
	before := s.store.Credentials["somecloud"].AuthCredentials["its-credential"].Attributes()["access-key"]
	c.Assert(before, tc.DeepEquals, "key")
	ctxt, err := cmdtesting.RunCommand(c, s.testCommand, "somecloud", "its-credential", "--client", "-f", testFile)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctxt), tc.Equals, "Local client was updated successfully with provided credential information.\n")
	c.Assert(cmdtesting.Stdout(ctxt), tc.Equals, "")
	after := s.store.Credentials["somecloud"].AuthCredentials["its-credential"].Attributes()["access-key"]
	c.Assert(after, tc.DeepEquals, "555")
}

func (s *updateCredentialSuite) TestUpdateRemoteCredentialWithFilePath(c *tc.C) {
	tmpFile, err := os.CreateTemp("", "juju-bootstrap-test")
	c.Assert(err, tc.ErrorIsNil)
	defer func() {
		tmpFile.Close()
		err := os.Remove(tmpFile.Name())
		c.Assert(err, tc.ErrorIsNil)
	}()

	s.store.Accounts = map[string]jujuclient.AccountDetails{
		"controller": {
			User: "admin@local",
		},
	}
	s.store.Credentials = map[string]jujucloud.CloudCredential{
		"google": {
			AuthCredentials: map[string]jujucloud.Credential{
				"gce": jujucloud.NewCredential(
					jujucloud.JSONFileAuthType,
					map[string]string{"file": tmpFile.Name()},
				),
			},
		},
	}

	s.api.clouds = func() (map[names.CloudTag]jujucloud.Cloud, error) {
		return map[names.CloudTag]jujucloud.Cloud{
			names.NewCloudTag("google"): {Name: "google", Type: "gce"},
		}, nil
	}

	contents := []byte("{something: special}\n")
	err = os.WriteFile(tmpFile.Name(), contents, 0644)
	c.Assert(err, tc.ErrorIsNil)

	// Double check credential from local cache does not contain contents. We expect it to be file path.
	c.Assert(s.store.Credentials["google"].AuthCredentials["gce"].Attributes()["file"], tc.Not(tc.Equals), string(contents))

	s.api.updateCloudsCredentials = func(cloudCredentials map[string]jujucloud.Credential, f bool) ([]params.UpdateCredentialResult, error) {
		c.Assert(cloudCredentials, tc.HasLen, 1)
		for k, v := range cloudCredentials {
			c.Assert(k, tc.DeepEquals, names.NewCloudCredentialTag("google/admin@local/gce").String())
			c.Assert(v.Attributes()["file"], tc.Equals, string(contents))
		}
		return nil, nil
	}
	_, err = cmdtesting.RunCommand(c, s.testCommand, "google", "gce", "-c", "controller")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *updateCredentialSuite) TestUpdateLocalCredentialWithFilePath(c *tc.C) {
	tmpFile, err := os.CreateTemp("", "juju-bootstrap-test")
	c.Assert(err, tc.ErrorIsNil)
	defer func() {
		tmpFile.Close()
		err := os.Remove(tmpFile.Name())
		c.Assert(err, tc.ErrorIsNil)
	}()

	s.store.Accounts = map[string]jujuclient.AccountDetails{
		"controller": {
			User: "admin@local",
		},
	}
	s.store.Credentials = map[string]jujucloud.CloudCredential{
		"google": {
			AuthCredentials: map[string]jujucloud.Credential{
				"gce": jujucloud.NewCredential(
					jujucloud.JSONFileAuthType,
					map[string]string{"file": "old-file-name"},
				),
			},
		},
	}

	contents := []byte("{something: special}\n")
	err = os.WriteFile(tmpFile.Name(), contents, 0644)
	c.Assert(err, tc.ErrorIsNil)

	testFile := s.makeCredentialsTestFile(c, fmt.Sprintf(`
credentials:
  google:
    gce:
      auth-type: jsonfile
      file: %v
`, tmpFile.Name()))
	_, err = cmdtesting.RunCommand(c, s.testCommand, "google", "gce", "--client", "-f", testFile)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.store.Credentials["google"].AuthCredentials["gce"].Attributes()["file"], tc.Not(tc.Contains), string(contents))
	c.Assert(s.store.Credentials["google"].AuthCredentials["gce"].Attributes()["file"], tc.Equals, tmpFile.Name())
}

func (s *updateCredentialSuite) TestUpdateRemote(c *tc.C) {
	s.api.updateCloudsCredentials = func(cloudCredentials map[string]jujucloud.Credential, f bool) ([]params.UpdateCredentialResult, error) {
		c.Assert(cloudCredentials, tc.HasLen, 1)
		expectedTag := names.NewCloudCredentialTag("aws/admin@local/my-credential").String()
		for k, v := range cloudCredentials {
			c.Assert(k, tc.DeepEquals, expectedTag)
			c.Assert(v, tc.DeepEquals, jujucloud.NewCredential(jujucloud.AccessKeyAuthType, map[string]string{"access-key": "key", "secret-key": "secret"}))
		}
		return []params.UpdateCredentialResult{{CredentialTag: expectedTag}}, nil
	}
	s.storeWithCredentials(c)
	ctx, err := cmdtesting.RunCommand(c, s.testCommand, "aws", "my-credential", "-c", "controller")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Contains, ``)
	c.Assert(cmdtesting.Stderr(ctx), tc.Contains, `
Controller credential "my-credential" for user "admin@local" for cloud "aws" on controller "controller" updated.
For more information, see 'juju show-credential aws my-credential'.
`[1:])
}

func (s *updateCredentialSuite) storeWithCredentials(c *tc.C) {
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
	data := `
clouds:
  somecloud:
    type: ec2
    auth-types: [access-key]
    endpoint: http://custom
`[1:]
	err := os.WriteFile(osenv.JujuXDGDataHomePath("clouds.yaml"), []byte(data), 0600)
	c.Assert(err, tc.ErrorIsNil)

	s.api.clouds = func() (map[names.CloudTag]jujucloud.Cloud, error) {
		return map[names.CloudTag]jujucloud.Cloud{
			names.NewCloudTag("aws"):       {Name: "aws", Type: "ec2"},
			names.NewCloudTag("somecloud"): {Name: "somecloud", Type: "openstack"},
		}, nil
	}
}

func (s *updateCredentialSuite) TestUpdateRemoteResultNotUserCloudError(c *tc.C) {
	s.storeWithCredentials(c)
	s.api.clouds = func() (map[names.CloudTag]jujucloud.Cloud, error) {
		return map[names.CloudTag]jujucloud.Cloud{
			names.NewCloudTag("somecloud"): {Name: "somecloud", Type: "openstack"},
		}, nil
	}
	_, err := cmdtesting.RunCommand(c, s.testCommand, "aws", "my-credential", "-c", "controller")
	c.Assert(err, tc.NotNil)
	//c.Assert(c.GetTestLog(), tc.Contains, `No cloud "aws" available to user "admin@local" remotely on controller "controller"`)
}

func (s *updateCredentialSuite) TestUpdateRemoteResultError(c *tc.C) {
	s.api.updateCloudsCredentials = func(cloudCredentials map[string]jujucloud.Credential, f bool) ([]params.UpdateCredentialResult, error) {
		return nil, errors.New("kaboom")
	}
	s.storeWithCredentials(c)
	_, err := cmdtesting.RunCommand(c, s.testCommand, "aws", "my-credential", "-c", "controller")
	c.Assert(err, tc.NotNil)
	//c.Assert(c.GetTestLog(), tc.Contains, ` kaboom`)
	//c.Assert(c.GetTestLog(), tc.Contains, `Could not update credentials remotely, on controller "controller"`)
}

func (s *updateCredentialSuite) TestUpdateRemoteForce(c *tc.C) {
	s.api.updateCloudsCredentials = func(cloudCredentials map[string]jujucloud.Credential, f bool) ([]params.UpdateCredentialResult, error) {
		c.Assert(f, tc.IsTrue)
		return nil, nil
	}
	s.storeWithCredentials(c)
	_, err := cmdtesting.RunCommand(c, s.testCommand, "aws", "my-credential", "-c", "controller", "--force")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *updateCredentialSuite) TestUpdateRemoteWithModels(c *tc.C) {
	s.api.updateCloudsCredentials = func(cloudCredentials map[string]jujucloud.Credential, f bool) ([]params.UpdateCredentialResult, error) {
		return []params.UpdateCredentialResult{
			{
				CredentialTag: names.NewCloudCredentialTag("aws/admin/my-credential").String(),
				Models: []params.UpdateCredentialModelResult{
					{
						ModelName: "model-a",
						Errors: []params.ErrorResult{
							{apiservererrors.ServerError(errors.New("kaboom"))},
							{apiservererrors.ServerError(errors.New("kaboom 2"))},
						},
					},
					{
						ModelName: "model-b",
						Errors: []params.ErrorResult{
							{apiservererrors.ServerError(errors.New("one failure"))},
						},
					},
					{
						ModelName: "model-c",
					},
				},
			},
		}, nil
	}
	s.storeWithCredentials(c)

	ctx, err := cmdtesting.RunCommand(c, s.testCommand, "aws", "my-credential", "-c", "controller")
	c.Assert(err, tc.DeepEquals, jujucmd.ErrSilent)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
Credential valid for:
  model-c
Credential invalid for:
  model-a:
    kaboom
    kaboom 2
  model-b:
    one failure
Failed models may require a different credential.
Use 'juju set-credential' to change credential for these models before repeating this update.
`[1:])
}

func (s *updateCredentialSuite) TestUpdateRemoteWithModelsError(c *tc.C) {
	s.api.updateCloudsCredentials = func(cloudCredentials map[string]jujucloud.Credential, f bool) ([]params.UpdateCredentialResult, error) {
		return []params.UpdateCredentialResult{
			{
				CredentialTag: names.NewCloudCredentialTag("aws/admin/my-credential").String(),
				Models: []params.UpdateCredentialModelResult{
					{
						ModelName: "model-a",
						Errors: []params.ErrorResult{
							{apiservererrors.ServerError(errors.New("kaboom"))},
							{apiservererrors.ServerError(errors.New("kaboom 2"))},
						},
					},
					{
						ModelName: "model-b",
						Errors: []params.ErrorResult{
							{apiservererrors.ServerError(errors.New("one failure"))},
						},
					},
					{
						ModelName: "model-c",
					},
				},
				Error: apiservererrors.ServerError(errors.New("models issues")),
			},
		}, nil
	}
	s.storeWithCredentials(c)

	ctx, err := cmdtesting.RunCommand(c, s.testCommand, "aws", "my-credential", "-c", "controller")
	c.Assert(err, tc.DeepEquals, jujucmd.ErrSilent)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
Credential valid for:
  model-c
Credential invalid for:
  model-a:
    kaboom
    kaboom 2
  model-b:
    one failure
Failed models may require a different credential.
Use 'juju set-credential' to change credential for these models before repeating this update.
`[1:])
}

func (s *updateCredentialSuite) TestUpdateRemoteWithModelsForce(c *tc.C) {
	s.api.updateCloudsCredentials = func(cloudCredentials map[string]jujucloud.Credential, f bool) ([]params.UpdateCredentialResult, error) {
		c.Assert(f, tc.IsTrue)
		return []params.UpdateCredentialResult{
			{
				CredentialTag: names.NewCloudCredentialTag("aws/admin/my-credential").String(),
				Models: []params.UpdateCredentialModelResult{
					{
						ModelName: "model-a",
						Errors: []params.ErrorResult{
							{apiservererrors.ServerError(errors.New("kaboom"))},
							{apiservererrors.ServerError(errors.New("kaboom 2"))},
						},
					},
					{
						ModelName: "model-b",
						Errors: []params.ErrorResult{
							{apiservererrors.ServerError(errors.New("one failure"))},
						},
					},
					{
						ModelName: "model-c",
					},
				},
				Error: apiservererrors.ServerError(errors.New("update error")),
			},
		}, nil
	}
	s.storeWithCredentials(c)

	ctx, err := cmdtesting.RunCommand(c, s.testCommand, "aws", "my-credential", "-c", "controller", "--force")
	c.Assert(err, tc.DeepEquals, jujucmd.ErrSilent)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
Credential valid for:
  model-c
Credential invalid for:
  model-a:
    kaboom
    kaboom 2
  model-b:
    one failure
Failed models may require a different credential.
Use 'juju set-credential' to change credential for these models.
`[1:])
	//c.Assert(c.GetTestLog(), tc.Contains, `Controller credential "my-credential" for user "admin@local" for cloud "aws" on controller "controller" not updated: update error`)
}

type fakeUpdateCredentialAPI struct {
	updateCloudsCredentials func(cloudCredentials map[string]jujucloud.Credential, force bool) ([]params.UpdateCredentialResult, error)
	addCloudsCredentials    func(cloudCredentials map[string]jujucloud.Credential) ([]params.UpdateCredentialResult, error)
	clouds                  func() (map[names.CloudTag]jujucloud.Cloud, error)
}

func (f *fakeUpdateCredentialAPI) Close() error {
	return nil
}

func (f *fakeUpdateCredentialAPI) UpdateCloudsCredentials(ctx context.Context, c map[string]jujucloud.Credential, force bool) ([]params.UpdateCredentialResult, error) {
	return f.updateCloudsCredentials(c, force)
}

func (f *fakeUpdateCredentialAPI) AddCloudsCredentials(ctx context.Context, c map[string]jujucloud.Credential) ([]params.UpdateCredentialResult, error) {
	return f.addCloudsCredentials(c)
}

func (f *fakeUpdateCredentialAPI) Clouds(ctx context.Context) (map[names.CloudTag]jujucloud.Cloud, error) {
	return f.clouds()
}
