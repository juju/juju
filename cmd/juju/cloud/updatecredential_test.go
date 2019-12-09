// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"

	jujucmd "github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type updateCredentialSuite struct {
	testing.FakeJujuXDGDataHomeSuite

	store                *jujuclient.MemStore
	testCommand          jujucmd.Command
	api                  *fakeUpdateCredentialAPI
	updateLocalCacheFunc func(cloudName string, details jujucloud.CloudCredential) error
}

var _ = gc.Suite(&updateCredentialSuite{})

func (s *updateCredentialSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.store = &jujuclient.MemStore{
		Controllers: map[string]jujuclient.ControllerDetails{
			"controller": {},
		},
		CurrentControllerName: "controller",
	}
	s.api = &fakeUpdateCredentialAPI{v: 5}
	s.testCommand = cloud.NewUpdateCredentialCommandForTest(s.store, s.api)
}

func (s *updateCredentialSuite) TestBadArgs(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, s.testCommand, "cloud", "credential", "extra")
	c.Assert(err, gc.ErrorMatches, `only a cloud name and / or credential name need to be provided`)
}

func (s *updateCredentialSuite) TestNoArgs(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, s.testCommand)
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`Usage: juju update-credential [options] [<cloud-name> [<credential-name>]]`))
}

func (s *updateCredentialSuite) TestBadFileSpecified(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, s.testCommand, "-f", "somefile.yaml")
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), jc.Contains, "could not get credentials from file: reading credentials file: open somefile.yaml")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}

func (s *updateCredentialSuite) makeCredentialsTestFile(c *gc.C, data string) string {
	dir := c.MkDir()
	credsFile := filepath.Join(dir, "cred.yaml")
	err := ioutil.WriteFile(credsFile, []byte(data), 0644)
	c.Assert(err, gc.IsNil)
	return credsFile
}

func (s *updateCredentialSuite) TestFileSpecified(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)

	one := jujucloud.NewCredential(jujucloud.InteractiveAuthType, map[string]string{"trust-password": "123"})
	assertFoundCredentials(c, result, map[string]map[string]jujucloud.Credential{
		"somecloud":    {"its-credential": one},
		"anothercloud": {"its-credential-too": one},
	})
}

func (s *updateCredentialSuite) TestFileSpecifiedWithCloud(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)

	one := jujucloud.NewCredential(jujucloud.InteractiveAuthType, map[string]string{"trust-password": "123"})
	assertFoundCredentials(c, result, map[string]map[string]jujucloud.Credential{
		cloudName: {
			"its-other-credential": one,
			"its-credential":       one,
		},
	})
}

func (s *updateCredentialSuite) TestFileSpecifiedButHasNoDesiredCloud(c *gc.C) {
	testFile := s.makeCredentialsTestFile(c, `
credentials:
  somecloud:
    its-credential:
      auth-type: interactive
      trust-password: "123"
`)
	result, err := cloud.CredentialsFromFile(testFile, "anothercloud", "")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(result, gc.IsNil)
}

func (s *updateCredentialSuite) TestFileSpecifiedWithCloudAndCredential(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)

	one := jujucloud.NewCredential(jujucloud.InteractiveAuthType, map[string]string{"trust-password": "123"})
	assertFoundCredentials(c, result, map[string]map[string]jujucloud.Credential{cloudName: {"its-credential": one}})
}

func (s *updateCredentialSuite) TestFileSpecifiedWithCloudAndCredentialNotFound(c *gc.C) {
	testFile := s.makeCredentialsTestFile(c, `
credentials:
  somecloud:
    its-other-credential:
      auth-type: interactive
      trust-password: "123"
`)
	result, err := cloud.CredentialsFromFile(testFile, "somecloud", "its-credential")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(result, gc.IsNil)
}

func (s *updateCredentialSuite) TestFileSpecifiedWithCloudAndCredentialInDifferentCloud(c *gc.C) {
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
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(result, gc.IsNil)
}

func (s *updateCredentialSuite) TestFileSpecifiedNoDesiredCloudAndCredential(c *gc.C) {
	testFile := s.makeCredentialsTestFile(c, `
credentials:
  anothercloud:
    its-another-credential:
      auth-type: interactive
      trust-password: "123"
`)
	result, err := cloud.CredentialsFromFile(testFile, "somecloud", "its-credential")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(result, gc.IsNil)
}

func (s *updateCredentialSuite) TestCloudCredentialFromLocalCacheNoneFound(c *gc.C) {
	result, err := cloud.CredentialsFromLocalCache(s.store, "anothercloud", "its-credential")
	c.Assert(err, gc.ErrorMatches, `loading credentials: credentials for cloud anothercloud not found`)
	c.Assert(result, gc.IsNil)
}

func (s *updateCredentialSuite) TestCloudCredentialFromLocalCacheWithCloud(c *gc.C) {
	s.storeWithCredentials(c)
	cloudName := "somecloud"
	result, err := cloud.CredentialsFromLocalCache(s.store, cloudName, "")
	c.Assert(err, jc.ErrorIsNil)

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

func assertFoundCredentials(c *gc.C, found map[string]jujucloud.CloudCredential, expected map[string]map[string]jujucloud.Credential) {
	// jujucloud.Credential has some unexported fields so we cannot compare structs.
	c.Assert(len(found), gc.Equals, len(expected))
	for foundCloudName, foundCloudCredentials := range found {
		expectedCloudCredentials, ok := expected[foundCloudName]
		c.Assert(ok, jc.IsTrue)
		c.Assert(len(foundCloudCredentials.AuthCredentials), gc.Equals, len(expectedCloudCredentials))
		for foundCredentialName, foundCredential := range foundCloudCredentials.AuthCredentials {
			expectedCredential, ok := expectedCloudCredentials[foundCredentialName]
			c.Assert(ok, jc.IsTrue)
			c.Assert(foundCredential.AuthType(), gc.DeepEquals, expectedCredential.AuthType())
			c.Assert(foundCredential.Attributes(), jc.DeepEquals, expectedCredential.Attributes())
		}
	}
}

func (s *updateCredentialSuite) TestCloudCredentialFromLocalCacheWithCloudAndCredential(c *gc.C) {
	s.storeWithCredentials(c)
	cloudName := "somecloud"
	credentialName := "its-credential"
	result, err := cloud.CredentialsFromLocalCache(s.store, cloudName, credentialName)
	c.Assert(err, jc.ErrorIsNil)

	one := jujucloud.NewCredential(jujucloud.AccessKeyAuthType, map[string]string{
		"access-key": "key",
		"secret-key": "secret"})
	assertFoundCredentials(c, result, map[string]map[string]jujucloud.Credential{
		cloudName: {
			"its-credential": one,
		},
	})
}

func (s *updateCredentialSuite) TestUpdateLocalWithCloudWhenNoneExists(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, s.testCommand, "somecloud", "its-credential", "--client")
	c.Assert(err, gc.ErrorMatches, "could not get credentials from local client: loading credentials: credentials for cloud somecloud not found")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}

func (s *updateCredentialSuite) TestUpdateLocalWithCloudWhenCredentialDoesNotExists(c *gc.C) {
	s.storeWithCredentials(c)
	ctx, err := cmdtesting.RunCommand(c, s.testCommand, "somecloud", "fluffy-credential", "--client")
	c.Assert(err, gc.ErrorMatches, `could not get credentials from local client: credential "fluffy-credential" for cloud "somecloud" in local client not found`)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}

func (s *updateCredentialSuite) TestUpdateLocal(c *gc.C) {
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
	c.Assert(before, gc.DeepEquals, "key")
	ctxt, err := cmdtesting.RunCommand(c, s.testCommand, "somecloud", "its-credential", "--client", "-f", testFile)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctxt), gc.Equals, "Local client was updated successfully with provided credential information.\n")
	c.Assert(cmdtesting.Stdout(ctxt), gc.Equals, "")
	after := s.store.Credentials["somecloud"].AuthCredentials["its-credential"].Attributes()["access-key"]
	c.Assert(after, gc.DeepEquals, "555")
}

func (s *updateCredentialSuite) TestUpdateRemoteCredentialWithFilePath(c *gc.C) {
	tmpFile, err := ioutil.TempFile("", "juju-bootstrap-test")
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		tmpFile.Close()
		err := os.Remove(tmpFile.Name())
		c.Assert(err, jc.ErrorIsNil)
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
	err = ioutil.WriteFile(tmpFile.Name(), contents, 0644)
	c.Assert(err, jc.ErrorIsNil)

	// Double check credential from local cache does not contain contents. We expect it to be file path.
	c.Assert(s.store.Credentials["google"].AuthCredentials["gce"].Attributes()["file"], gc.Not(gc.Equals), string(contents))

	s.api.updateCloudsCredentials = func(cloudCredentials map[string]jujucloud.Credential, f bool) ([]params.UpdateCredentialResult, error) {
		c.Assert(cloudCredentials, gc.HasLen, 1)
		for k, v := range cloudCredentials {
			c.Assert(k, gc.DeepEquals, names.NewCloudCredentialTag("google/admin@local/gce").String())
			c.Assert(v.Attributes()["file"], gc.Equals, string(contents))
		}
		return nil, nil
	}
	_, err = cmdtesting.RunCommand(c, s.testCommand, "google", "gce", "-c", "controller")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *updateCredentialSuite) TestUpdateLocalCredentialWithFilePath(c *gc.C) {
	tmpFile, err := ioutil.TempFile("", "juju-bootstrap-test")
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		tmpFile.Close()
		err := os.Remove(tmpFile.Name())
		c.Assert(err, jc.ErrorIsNil)
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
	err = ioutil.WriteFile(tmpFile.Name(), contents, 0644)
	c.Assert(err, jc.ErrorIsNil)

	testFile := s.makeCredentialsTestFile(c, fmt.Sprintf(`
credentials:
  google:
    gce:
      auth-type: jsonfile
      file: %v
`, tmpFile.Name()))
	_, err = cmdtesting.RunCommand(c, s.testCommand, "google", "gce", "--client", "-f", testFile)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.store.Credentials["google"].AuthCredentials["gce"].Attributes()["file"], gc.Not(jc.Contains), string(contents))
	c.Assert(s.store.Credentials["google"].AuthCredentials["gce"].Attributes()["file"], gc.Equals, tmpFile.Name())
}

func (s *updateCredentialSuite) TestUpdateRemote(c *gc.C) {
	s.api.updateCloudsCredentials = func(cloudCredentials map[string]jujucloud.Credential, f bool) ([]params.UpdateCredentialResult, error) {
		c.Assert(cloudCredentials, gc.HasLen, 1)
		expectedTag := names.NewCloudCredentialTag("aws/admin@local/my-credential").String()
		for k, v := range cloudCredentials {
			c.Assert(k, gc.DeepEquals, expectedTag)
			c.Assert(v, jc.DeepEquals, jujucloud.NewCredential(jujucloud.AccessKeyAuthType, map[string]string{"access-key": "key", "secret-key": "secret"}))
		}
		return []params.UpdateCredentialResult{{CredentialTag: expectedTag}}, nil
	}
	s.storeWithCredentials(c)
	ctx, err := cmdtesting.RunCommand(c, s.testCommand, "aws", "my-credential", "-c", "controller")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.Contains, ``)
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, `
Controller credential "my-credential" for user "admin@local" for cloud "aws" on controller "controller" updated.
For more information, see ‘juju show-credential aws my-credential’.
`[1:])
}

func (s *updateCredentialSuite) storeWithCredentials(c *gc.C) {
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
	err := ioutil.WriteFile(osenv.JujuXDGDataHomePath("clouds.yaml"), []byte(data), 0600)
	c.Assert(err, jc.ErrorIsNil)

	s.api.clouds = func() (map[names.CloudTag]jujucloud.Cloud, error) {
		return map[names.CloudTag]jujucloud.Cloud{
			names.NewCloudTag("aws"):       {Name: "aws", Type: "ec2"},
			names.NewCloudTag("somecloud"): {Name: "somecloud", Type: "openstack"},
		}, nil
	}
}

func (s *updateCredentialSuite) TestUpdateRemoteResultNotUserCloudError(c *gc.C) {
	s.storeWithCredentials(c)
	s.api.clouds = func() (map[names.CloudTag]jujucloud.Cloud, error) {
		return map[names.CloudTag]jujucloud.Cloud{
			names.NewCloudTag("somecloud"): {Name: "somecloud", Type: "openstack"},
		}, nil
	}
	_, err := cmdtesting.RunCommand(c, s.testCommand, "aws", "my-credential", "-c", "controller")
	c.Assert(err, gc.NotNil)
	c.Assert(c.GetTestLog(), jc.Contains, `No cloud "aws" available to user "admin@local" remotely on controller "controller"`)
}

func (s *updateCredentialSuite) TestUpdateRemoteResultError(c *gc.C) {
	s.api.updateCloudsCredentials = func(cloudCredentials map[string]jujucloud.Credential, f bool) ([]params.UpdateCredentialResult, error) {
		return nil, errors.New("kaboom")
	}
	s.storeWithCredentials(c)
	_, err := cmdtesting.RunCommand(c, s.testCommand, "aws", "my-credential", "-c", "controller")
	c.Assert(err, gc.NotNil)
	c.Assert(c.GetTestLog(), jc.Contains, ` kaboom`)
	c.Assert(c.GetTestLog(), jc.Contains, `Could not update credentials remotely, on controller "controller"`)
}

func (s *updateCredentialSuite) TestUpdateRemoteForce(c *gc.C) {
	s.api.updateCloudsCredentials = func(cloudCredentials map[string]jujucloud.Credential, f bool) ([]params.UpdateCredentialResult, error) {
		c.Assert(f, jc.IsTrue)
		return nil, nil
	}
	s.storeWithCredentials(c)
	_, err := cmdtesting.RunCommand(c, s.testCommand, "aws", "my-credential", "-c", "controller", "--force")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *updateCredentialSuite) TestUpdateRemoteWithModels(c *gc.C) {
	s.api.updateCloudsCredentials = func(cloudCredentials map[string]jujucloud.Credential, f bool) ([]params.UpdateCredentialResult, error) {
		return []params.UpdateCredentialResult{
			{
				CredentialTag: names.NewCloudCredentialTag("aws/admin/my-credential").String(),
				Models: []params.UpdateCredentialModelResult{
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
				},
				Error: common.ServerError(errors.New("models issues")),
			},
		}, nil
	}
	s.storeWithCredentials(c)

	ctx, err := cmdtesting.RunCommand(c, s.testCommand, "aws", "my-credential", "-c", "controller")
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
Failed models may require a different credential.
Use ‘juju set-credential’ to change credential for these models before repeating this update.
`[1:])
	c.Assert(c.GetTestLog(), jc.Contains, `Controller credential "my-credential" for user "admin@local" for cloud "aws" on controller "controller" not updated: models issues`)
}

type fakeUpdateCredentialAPI struct {
	v                       int
	updateCloudsCredentials func(cloudCredentials map[string]jujucloud.Credential, force bool) ([]params.UpdateCredentialResult, error)
	addCloudsCredentials    func(cloudCredentials map[string]jujucloud.Credential) ([]params.UpdateCredentialResult, error)
	clouds                  func() (map[names.CloudTag]jujucloud.Cloud, error)
}

func (f *fakeUpdateCredentialAPI) Close() error {
	return nil
}

func (f *fakeUpdateCredentialAPI) BestAPIVersion() int {
	return f.v
}

func (f *fakeUpdateCredentialAPI) UpdateCloudsCredentials(c map[string]jujucloud.Credential, force bool) ([]params.UpdateCredentialResult, error) {
	return f.updateCloudsCredentials(c, force)
}

func (f *fakeUpdateCredentialAPI) AddCloudsCredentials(c map[string]jujucloud.Credential) ([]params.UpdateCredentialResult, error) {
	return f.addCloudsCredentials(c)
}

func (f *fakeUpdateCredentialAPI) Clouds() (map[names.CloudTag]jujucloud.Cloud, error) {
	return f.clouds()
}
