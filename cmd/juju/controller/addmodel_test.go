// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	_ "github.com/juju/juju/internal/provider/ec2"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

type AddModelSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fakeAddModelAPI      *fakeAddClient
	fakeCloudAPI         *fakeCloudAPI
	fakeProvider         *fakeProvider
	fakeProviderRegistry *fakeProviderRegistry
	store                *jujuclient.MemStore
}

var _ = tc.Suite(&AddModelSuite{})

func (s *AddModelSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	agentVersion, err := semversion.Parse("2.55.5")
	c.Assert(err, tc.ErrorIsNil)
	s.fakeAddModelAPI = &fakeAddClient{
		model: base.ModelInfo{
			Name:         "test",
			Type:         model.IAAS,
			UUID:         "fake-model-uuid",
			Owner:        "ignored-for-now",
			AgentVersion: &agentVersion,
		},
	}
	s.fakeCloudAPI = &fakeCloudAPI{
		authTypes: []cloud.AuthType{
			cloud.EmptyAuthType,
			cloud.AccessKeyAuthType,
		},
		credentials: []names.CloudCredentialTag{
			names.NewCloudCredentialTag("cloud/admin/default"),
			names.NewCloudCredentialTag("aws/other/secrets"),
		},
	}
	s.fakeCloudAPI.clouds = map[names.CloudTag]cloud.Cloud{
		names.NewCloudTag("aws"): {
			Name:      "aws",
			AuthTypes: s.fakeCloudAPI.authTypes,
			Regions: []cloud.Region{
				{Name: "us-east-1"},
				{Name: "us-west-1"},
			},
		},
	}

	s.fakeProvider = &fakeProvider{
		detected: cloud.NewEmptyCloudCredential(),
	}
	s.fakeProviderRegistry = &fakeProviderRegistry{
		provider: s.fakeProvider,
	}

	// Set up the current controller, and write just enough info
	// so we don't try to refresh
	controllerName := "test-master"
	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = controllerName
	s.store.Controllers[controllerName] = jujuclient.ControllerDetails{}
	s.store.Accounts[controllerName] = jujuclient.AccountDetails{
		User: "bob",
	}
	s.store.Credentials["aws"] = cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			"secrets": cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
				"access-key": "key",
				"secret-key": "sekret",
			}),
		},
	}
}

type fakeAPIConnection struct {
	api.Connection
}

func (*fakeAPIConnection) Close() error {
	return nil
}

func (s *AddModelSuite) run(c *tc.C, args ...string) (*cmd.Context, error) {
	command, _ := controller.NewAddModelCommandForTest(
		&fakeAPIConnection{},
		s.fakeAddModelAPI,
		s.fakeCloudAPI,
		s.store,
		s.fakeProviderRegistry,
	)
	return cmdtesting.RunCommand(c, command, args...)
}

func (s *AddModelSuite) TestInit(c *tc.C) {
	modelNameErr := "%q is not a valid name: model names may only contain lowercase letters, digits and hyphens"
	for i, test := range []struct {
		args        []string
		err         string
		name        string
		owner       string
		cloudRegion string
		values      map[string]interface{}
	}{
		{
			err: common.MissingModelNameError("add-model").Error(),
		}, {
			args: []string{"new-model"},
			name: "new-model",
		}, {
			args: []string{"n"},
			name: "n",
		}, {
			args: []string{"new model"},
			err:  fmt.Sprintf(modelNameErr, "new model"),
		}, {
			args: []string{"newModel"},
			err:  fmt.Sprintf(modelNameErr, "newModel"),
		}, {
			args: []string{"-"},
			err:  fmt.Sprintf(modelNameErr, "-"),
		}, {
			args: []string{"new@model"},
			err:  fmt.Sprintf(modelNameErr, "new@model"),
		}, {
			args:  []string{"new-model", "--owner", "foo"},
			name:  "new-model",
			owner: "foo",
		}, {
			args: []string{"new-model", "--owner", "not=valid"},
			err:  `"not=valid" is not a valid user`,
		}, {
			args:   []string{"new-model", "--config", "key=value", "--config", "key2=value2"},
			name:   "new-model",
			values: map[string]interface{}{"key": "value", "key2": "value2"},
		}, {
			args:        []string{"new-model", "cloud/region"},
			name:        "new-model",
			cloudRegion: "cloud/region",
		}, {
			args: []string{"new-model", "cloud/region", "extra", "args"},
			err:  `unrecognized args: \["extra" "args"\]`,
		},
	} {
		c.Logf("test %d", i)
		wrappedCommand, command := controller.NewAddModelCommandForTest(nil, nil, nil, s.store, nil)
		err := cmdtesting.InitCommand(wrappedCommand, test.args)
		if test.err != "" {
			c.Assert(err, tc.ErrorMatches, test.err)
			continue
		}

		c.Assert(err, tc.ErrorIsNil)
		c.Assert(command.Name, tc.Equals, test.name)
		c.Assert(command.Owner, tc.Equals, test.owner)
		c.Assert(command.CloudRegion, tc.Equals, test.cloudRegion)
		attrs, err := command.Config.ReadAttrs(nil)
		c.Assert(err, tc.ErrorIsNil)
		if len(test.values) == 0 {
			c.Assert(attrs, tc.HasLen, 0)
		} else {
			c.Assert(attrs, tc.DeepEquals, test.values)
		}
	}
}

func (s *AddModelSuite) TestAddExistingName(c *tc.C) {
	// If there's any model details existing, we just overwrite them. The
	// controller will error out if the model already exists. Overwriting
	// means we'll replace any stale details from an previously existing
	// model with the same name.
	err := s.store.UpdateModel("test-master", "bob/test", jujuclient.ModelDetails{
		ModelUUID: "stale-uuid",
		ModelType: model.IAAS,
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.run(c, "test")
	c.Assert(err, tc.ErrorIsNil)

	details, err := s.store.ModelByName("test-master", "bob/test")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(details, tc.DeepEquals, &jujuclient.ModelDetails{
		ModelUUID: "fake-model-uuid",
		ModelType: model.IAAS,
	})
}

func (s *AddModelSuite) TestAddModelUnauthorizedMentionsJujuGrant(c *tc.C) {
	s.fakeAddModelAPI.err = &params.Error{
		Message: "permission denied",
		Code:    params.CodeUnauthorized,
	}
	ctx, _ := s.run(c, "test")
	errString := strings.Replace(cmdtesting.Stderr(ctx), "\n", " ", -1)
	c.Assert(errString, tc.Matches, `.*juju grant.*`)
}

func (s *AddModelSuite) TestCredentialsPassedThrough(c *tc.C) {
	_, err := s.run(c, "test", "--credential", "secrets")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(s.fakeAddModelAPI.cloudCredential, tc.Equals, names.NewCloudCredentialTag("aws/bob/secrets"))
}

func (s *AddModelSuite) TestCredentialsOtherUserPassedThrough(c *tc.C) {
	_, err := s.run(c, "test", "--credential", "other/secrets")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(s.fakeAddModelAPI.cloudCredential, tc.Equals, names.NewCloudCredentialTag("aws/other/secrets"))
}

func (s *AddModelSuite) TestCredentialsOtherUserPassedThroughWhenCloud(c *tc.C) {
	_, err := s.run(c, "test", "--credential", "other/secrets", "aws/us-west-1")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(s.fakeAddModelAPI.cloudCredential, tc.Equals, names.NewCloudCredentialTag("aws/other/secrets"))
}

func (s *AddModelSuite) TestCredentialsOtherUserCredentialNotFound(c *tc.C) {
	// Have the API respond with no credentials.
	s.PatchValue(&s.fakeCloudAPI.credentials, []names.CloudCredentialTag{})

	ctx, err := s.run(c, "test", "--credential", "other/secrets")
	c.Assert(err, tc.DeepEquals, cmd.ErrSilent)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
Use 
* 'juju add-credential -c' to upload a credential to a controller or
* 'juju autoload-credentials' to add credentials from local files or
* 'juju add-model --credential' to use a local credential.
Use 'juju credentials' to list all available credentials.
`[1:])
	//c.Assert(c.GetTestLog(), tc.Contains, "credential 'other/secrets' not found")

	// There should be no detection or UpdateCredentials call.
	s.fakeCloudAPI.CheckCallNames(c, "Clouds", "Cloud", "UserCredentials")
}

func (s *AddModelSuite) TestCredentialsNoDefaultCloud(c *tc.C) {
	s.fakeCloudAPI.clouds = nil
	_, err := s.run(c, "test", "--credential", "secrets")
	c.Assert(err, tc.ErrorMatches, `you do not have add-model access to any clouds on this controller.
Please ask the controller administrator to grant you add-model permission
for a particular cloud to which you want to add a model.`)
}

func (s *AddModelSuite) TestCredentialsOneCached(c *tc.C) {
	// Disable empty auth and clear the local credentials,
	// forcing a check for credentials in the controller.
	s.PatchValue(&s.fakeCloudAPI.authTypes, []cloud.AuthType{cloud.AccessKeyAuthType})
	delete(s.store.Credentials, "aws")

	// Cache just a single credential in the controller,
	// so it is selected automatically.
	credentialTag := names.NewCloudCredentialTag("aws/foo/secrets")
	s.PatchValue(&s.fakeCloudAPI.credentials, []names.CloudCredentialTag{credentialTag})

	_, err := s.run(c, "test", "aws/us-west-1")
	c.Assert(err, tc.ErrorIsNil)

	// The cached credential should be used, along with
	// the user-specified cloud region.
	c.Assert(s.fakeAddModelAPI.cloudCredential, tc.Equals, credentialTag)
	c.Assert(s.fakeAddModelAPI.cloudRegion, tc.Equals, "us-west-1")
}

func (s *AddModelSuite) TestControllerCredentialsDetected(c *tc.C) {
	// Disable empty auth and clear the local credentials,
	// forcing a check for credentials in the controller.
	// There are multiple credentials in the controller,
	// so none of them will be chosen by default.
	s.PatchValue(&s.fakeCloudAPI.authTypes, []cloud.AuthType{cloud.AccessKeyAuthType})

	// Delete all local credentials, so we don't choose
	// any of them to upload. This will force credential
	// detection.
	delete(s.store.Credentials, "aws")

	_, err := s.run(c, "test")
	c.Assert(err, tc.ErrorIsNil)

	credentialTag := names.NewCloudCredentialTag("aws/bob/default")
	credential := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{})
	credential.Label = "finalized"

	c.Assert(s.fakeAddModelAPI.cloudCredential, tc.Equals, credentialTag)
	s.fakeCloudAPI.CheckCallNames(c, "Clouds", "Cloud", "UserCredentials")
}

func (s *AddModelSuite) TestControllerCredentialsDetectedAmbiguous(c *tc.C) {
	// Disable empty auth and clear the local credentials,
	// forcing a check for credentials in the controller.
	// There are multiple credentials in the controller,
	// so none of them will be chosen by default.
	s.PatchValue(&s.fakeCloudAPI.authTypes, []cloud.AuthType{cloud.AccessKeyAuthType})

	// Delete all local credentials, so we don't choose
	// any of them to upload. This will force credential
	// detection.
	delete(s.store.Credentials, "aws")

	s.PatchValue(&s.fakeProvider.detected, &cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			"one": {},
			"two": {},
		},
	})

	ctx, err := s.run(c, "test")
	c.Assert(err, tc.DeepEquals, cmd.ErrSilent)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
Use 
* 'juju add-credential -c' to upload a credential to a controller or
* 'juju autoload-credentials' to add credentials from local files or
* 'juju add-model --credential' to use a local credential.
Use 'juju credentials' to list all available credentials.
`[1:])
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
	//	c.Assert(c.GetTestLog(), tc.Contains, `
	//
	// more than one credential detected. Add all detected credentials
	// to the client with:
	//
	//	juju autoload-credentials
	//
	// and then run the add-model command again with the --credential option.`[1:])
}

func (s *AddModelSuite) TestCloudRegionPassedThrough(c *tc.C) {
	_, err := s.run(c, "test", "aws/us-west-1")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(s.fakeAddModelAPI.cloudName, tc.Equals, "aws")
	c.Assert(s.fakeAddModelAPI.cloudRegion, tc.Equals, "us-west-1")
}

func (s *AddModelSuite) TestDefaultCloudPassedThrough(c *tc.C) {
	_, err := s.run(c, "test")
	c.Assert(err, tc.ErrorIsNil)

	s.fakeCloudAPI.CheckCallNames(c, "Clouds", "Cloud")
	c.Assert(s.fakeAddModelAPI.cloudName, tc.Equals, "aws")
	c.Assert(s.fakeAddModelAPI.cloudRegion, tc.Equals, "")
}

func (s *AddModelSuite) TestDefaultCloudRegionPassedThrough(c *tc.C) {
	_, err := s.run(c, "test", "us-west-1")
	c.Assert(err, tc.ErrorIsNil)

	s.fakeCloudAPI.CheckCalls(c, []testhelpers.StubCall{
		{"Cloud", []interface{}{names.NewCloudTag("us-west-1")}},
		{"Clouds", nil},
		{"Cloud", []interface{}{names.NewCloudTag("aws")}},
	})
	c.Assert(s.fakeAddModelAPI.cloudName, tc.Equals, "aws")
	c.Assert(s.fakeAddModelAPI.cloudRegion, tc.Equals, "us-west-1")
}

func (s *AddModelSuite) TestNoDefaultCloudRegion(c *tc.C) {
	s.fakeCloudAPI.clouds = nil
	ctx, err := s.run(c, "test", "us-west-1")
	c.Assert(err, tc.DeepEquals, cmd.ErrSilent)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "Use 'juju clouds' to see a list of all available clouds or 'juju add-cloud' to a add one.\n")
	//	c.Assert(c.GetTestLog(), tc.Contains, `
	//you do not have add-model access to any clouds on this controller.
	//Please ask the controller administrator to grant you add-model permission
	//for a particular cloud to which you want to add a model.`[1:])
	s.fakeCloudAPI.CheckCalls(c, []testhelpers.StubCall{
		{"Cloud", []interface{}{names.NewCloudTag("us-west-1")}},
		{"Clouds", nil},
	})
}

func (s *AddModelSuite) TestAmbiguousCloud(c *tc.C) {
	s.fakeCloudAPI.clouds[names.NewCloudTag("lxd")] = cloud.Cloud{}
	ctx, err := s.run(c, "test", "us-west-1")
	c.Assert(err, tc.DeepEquals, cmd.ErrSilent)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "Use 'juju clouds' to see a list of all available clouds or 'juju add-cloud' to a add one.\n")
	//	c.Assert(c.GetTestLog(), tc.Contains, `
	//this controller manages more than one cloud.
	//Please specify which cloud/region to use:
	//
	//    juju add-model [options] <model-name> cloud[/region]
	//
	//The clouds/regions supported by this controller are:
	//
	//Cloud  Regions
	//aws    us-east-1, us-west-1
	//lxd
	//`[1:])
	s.fakeCloudAPI.CheckCalls(c, []testhelpers.StubCall{
		{"Cloud", []interface{}{names.NewCloudTag("us-west-1")}},
		{"Clouds", nil},
		{"Cloud", []interface{}{names.NewCloudTag("aws")}},
	})
}

func (s *AddModelSuite) TestCloudUnspecifiedRegionPassedThrough(c *tc.C) {
	// Use a cloud that doesn't support empty authorization.
	s.fakeCloudAPI = &fakeCloudAPI{
		authTypes: []cloud.AuthType{
			cloud.AccessKeyAuthType,
		},
		credentials: []names.CloudCredentialTag{
			names.NewCloudCredentialTag("cloud/admin/default"),
			names.NewCloudCredentialTag("aws/other/secrets"),
		},
	}
	_, err := s.run(c, "test", "aws")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(s.fakeAddModelAPI.cloudName, tc.Equals, "aws")
	c.Assert(s.fakeAddModelAPI.cloudRegion, tc.Equals, "")
}

func (s *AddModelSuite) TestCloudDefaultRegionUsedIfSet(c *tc.C) {
	// Overwrite the credentials with a default region.
	s.store.Credentials["aws"] = cloud.CloudCredential{
		DefaultRegion: "us-west-1",
		AuthCredentials: map[string]cloud.Credential{
			"secrets": cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
				"access-key": "key",
				"secret-key": "sekret",
			}),
		},
	}
	// Use a cloud that doesn't support empty authorization.
	s.fakeCloudAPI = &fakeCloudAPI{
		authTypes: []cloud.AuthType{
			cloud.AccessKeyAuthType,
		},
		credentials: []names.CloudCredentialTag{
			names.NewCloudCredentialTag("cloud/admin/default"),
			names.NewCloudCredentialTag("aws/other/secrets"),
		},
	}
	_, err := s.run(c, "test", "aws")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(s.fakeAddModelAPI.cloudName, tc.Equals, "aws")
	c.Assert(s.fakeAddModelAPI.cloudRegion, tc.Equals, "us-west-1")
}

func (s *AddModelSuite) TestExplicitCloudRegionUsed(c *tc.C) {
	// When a controller credential is used, any explicit region is honoured.

	// Delete all local credentials, so we don't choose
	// any of them to upload. This will force a credential
	// to be retrieved from the controller.
	delete(s.store.Credentials, "aws")

	_, err := s.run(c, "test", "aws/us-east-1", "--credential", "other/secrets")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(s.fakeAddModelAPI.cloudName, tc.Equals, "aws")
	c.Assert(s.fakeAddModelAPI.cloudRegion, tc.Equals, "us-east-1")
}

func (s *AddModelSuite) TestInvalidCloudOrRegionName(c *tc.C) {
	ctx, err := s.run(c, "test", "oro")
	c.Assert(err, tc.DeepEquals, cmd.ErrSilent)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "Use 'juju clouds' to see a list of all available clouds or 'juju add-cloud' to a add one.\n")
	//	c.Assert(c.GetTestLog(), tc.Contains, `
	//
	// "oro" is neither a cloud supported by this controller,
	// nor a region in the controller's default cloud "aws".
	// The clouds/regions supported by this controller are:
	//
	// Cloud  Regions
	// aws    us-east-1, us-west-1
	// `[1:])
}

func (s *AddModelSuite) TestComandLineConfigPassedThrough(c *tc.C) {
	_, err := s.run(c, "test", "--config", "account=magic", "--config", "cloud=special")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(s.fakeAddModelAPI.config["account"], tc.Equals, "magic")
	c.Assert(s.fakeAddModelAPI.config["cloud"], tc.Equals, "special")
}

func (s *AddModelSuite) TestConfigFileValuesPassedThrough(c *tc.C) {
	config := map[string]string{
		"account": "magic",
		"cloud":   "9",
	}
	bytes, err := yaml.Marshal(config)
	c.Assert(err, tc.ErrorIsNil)
	file, err := os.CreateTemp(c.MkDir(), "")
	c.Assert(err, tc.ErrorIsNil)
	file.Write(bytes)
	file.Close()

	_, err = s.run(c, "test", "--config", file.Name())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.fakeAddModelAPI.config["account"], tc.Equals, "magic")
	c.Assert(s.fakeAddModelAPI.config["cloud"], tc.Equals, "9")
}

func (s *AddModelSuite) TestConfigFileWithNestedMaps(c *tc.C) {
	nestedConfig := map[string]interface{}{
		"account": "magic",
		"cloud":   "9",
	}
	config := map[string]interface{}{
		"foo":    "bar",
		"nested": nestedConfig,
	}

	bytes, err := yaml.Marshal(config)
	c.Assert(err, tc.ErrorIsNil)
	file, err := os.CreateTemp(c.MkDir(), "")
	c.Assert(err, tc.ErrorIsNil)
	file.Write(bytes)
	file.Close()

	_, err = s.run(c, "test", "--config", file.Name())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.fakeAddModelAPI.config["foo"], tc.Equals, "bar")
	c.Assert(s.fakeAddModelAPI.config["nested"], tc.DeepEquals, nestedConfig)
}

func (s *AddModelSuite) TestConfigFileFailsToConform(c *tc.C) {
	nestedConfig := map[int]interface{}{
		9: "9",
	}
	config := map[string]interface{}{
		"foo":    "bar",
		"nested": nestedConfig,
	}
	bytes, err := yaml.Marshal(config)
	c.Assert(err, tc.ErrorIsNil)
	file, err := os.CreateTemp(c.MkDir(), "")
	c.Assert(err, tc.ErrorIsNil)
	file.Write(bytes)
	file.Close()

	_, err = s.run(c, "test", "--config", file.Name())
	c.Assert(err, tc.ErrorMatches, `unable to parse config: map keyed with non-string value`)
}

func (s *AddModelSuite) TestConfigFileFormatError(c *tc.C) {
	file, err := os.CreateTemp(c.MkDir(), "")
	c.Assert(err, tc.ErrorIsNil)
	file.Write(([]byte)("not: valid: yaml"))
	file.Close()

	_, err = s.run(c, "test", "--config", file.Name())
	c.Assert(err, tc.ErrorMatches, `unable to parse config: yaml: .*`)
}

func (s *AddModelSuite) TestConfigFileDoesntExist(c *tc.C) {
	_, err := s.run(c, "test", "--config", "missing-file")
	errMsg := ".*" + utils.NoSuchFileErrRegexp
	c.Assert(err, tc.ErrorMatches, errMsg)
}

func (s *AddModelSuite) TestConfigValuePrecedence(c *tc.C) {
	config := map[string]string{
		"account": "magic",
		"cloud":   "9",
	}
	bytes, err := yaml.Marshal(config)
	c.Assert(err, tc.ErrorIsNil)
	file, err := os.CreateTemp(c.MkDir(), "")
	c.Assert(err, tc.ErrorIsNil)
	file.Write(bytes)
	file.Close()

	_, err = s.run(c, "test", "--config", file.Name(), "--config", "account=magic", "--config", "cloud=special")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.fakeAddModelAPI.config["account"], tc.Equals, "magic")
	c.Assert(s.fakeAddModelAPI.config["cloud"], tc.Equals, "special")
}

func (s *AddModelSuite) TestAddErrorRemoveConfigstoreInfo(c *tc.C) {
	s.fakeAddModelAPI.err = errors.New("bah humbug")

	_, err := s.run(c, "test")
	c.Assert(err, tc.ErrorMatches, "bah humbug")

	_, err = s.store.ModelByName("test-master", "bob/test")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *AddModelSuite) TestAddStoresValues(c *tc.C) {
	const controllerName = "test-master"

	_, err := s.run(c, "test")
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.store.CurrentControllerName, tc.Equals, controllerName)
	modelName, err := s.store.CurrentModel(controllerName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelName, tc.Equals, "bob/test")

	m, err := s.store.ModelByName(controllerName, modelName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m, tc.DeepEquals, &jujuclient.ModelDetails{
		ModelUUID: "fake-model-uuid",
		ModelType: model.IAAS,
	})
}

func (s *AddModelSuite) TestSwitch(c *tc.C) {
	const controllerName = "test-master"

	// if the previous switch was on another controller, add model would have switch to model
	s.store.HasControllerChangedOnPreviousSwitch = true

	_, err := s.run(c, "test")
	c.Assert(err, tc.ErrorIsNil)

	modelName, err := s.store.CurrentModel(controllerName)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.store.HasControllerChangedOnPreviousSwitch, tc.Equals, false)
	c.Check(s.store.CurrentControllerName, tc.Equals, controllerName)
	c.Check(modelName, tc.Equals, "bob/test")
}

func (s *AddModelSuite) TestNoSwitch(c *tc.C) {
	const controllerName = "test-master"
	checkNoModelSelected := func() {
		_, err := s.store.CurrentModel(controllerName)
		c.Check(err, tc.ErrorIs, errors.NotFound)
	}
	checkNoModelSelected()

	_, err := s.run(c, "test", "--no-switch")
	c.Assert(err, tc.ErrorIsNil)

	// New model should not be selected by should still exist in the
	// store.
	checkNoModelSelected()
	m, err := s.store.ModelByName(controllerName, "bob/test")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m, tc.DeepEquals, &jujuclient.ModelDetails{
		ModelUUID: "fake-model-uuid",
		ModelType: model.IAAS,
	})
}

func (s *AddModelSuite) TestNoEnvCacheOtherUser(c *tc.C) {
	_, err := s.run(c, "test", "--owner", "zeus")
	c.Assert(err, tc.ErrorIsNil)

	// Creating a model for another user does not update the model cache.
	_, err = s.store.ModelByName("test-master", "bob/test")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *AddModelSuite) TestNamespaceAnnotationsErr(c *tc.C) {
	s.fakeCloudAPI.cloud = &cloud.Cloud{
		Type: "kubernetes",
	}
	s.fakeAddModelAPI.err = &params.Error{
		Message: `failed to open kubernetes client: annotations map[controller.juju.is/id:e911779d-c210-4207-8a37-586029693d85 model.juju.is/id:b36d5a71-fe97-48cb-87f7-479c98a741df] for namespace "borked" must include map[model.juju.is/id:f58485c2-4f08-4571-88c4-2e6b9ece955c]`,
		Code:    params.CodeNotValid,
	}
	_, err := s.run(c, "foobar")
	c.Assert(err, tc.ErrorMatches, `cannot create model "foobar": a namespace called "foobar" already exists on this k8s cluster. Please pick a different model name.`)
}

// fakeAddClient is used to mock out the behavior of the real
// AddModel command.
type fakeAddClient struct {
	owner           string
	cloudName       string
	cloudRegion     string
	cloudCredential names.CloudCredentialTag
	config          map[string]interface{}
	err             error
	model           base.ModelInfo
}

var _ controller.AddModelAPI = (*fakeAddClient)(nil)

func (*fakeAddClient) Close() error {
	return nil
}

func (f *fakeAddClient) CreateModel(ctx context.Context, name, owner, cloudName, cloudRegion string, cloudCredential names.CloudCredentialTag, config map[string]interface{}) (base.ModelInfo, error) {
	if f.err != nil {
		return base.ModelInfo{}, f.err
	}
	f.owner = owner
	f.cloudCredential = cloudCredential
	f.cloudName = cloudName
	f.cloudRegion = cloudRegion
	f.config = config
	return f.model, nil
}

// TODO(wallyworld) - improve this stub and add test asserts
type fakeCloudAPI struct {
	clouds map[names.CloudTag]cloud.Cloud
	cloud  *cloud.Cloud
	controller.CloudAPI
	testhelpers.Stub
	authTypes   []cloud.AuthType
	credentials []names.CloudCredentialTag
}

func (c *fakeCloudAPI) Clouds(ctx context.Context) (map[names.CloudTag]cloud.Cloud, error) {
	c.MethodCall(c, "Clouds")
	if c.clouds == nil {
		return c.clouds, c.NextErr()
	}
	// Ensure the aws cloud uses the patched auth types.
	awsTag := names.NewCloudTag("aws")
	awsCloud, err := c.Cloud(ctx, awsTag)
	if err != nil {
		return nil, err
	}
	c.clouds[awsTag] = awsCloud
	return c.clouds, c.NextErr()
}

func (c *fakeCloudAPI) Cloud(ctx context.Context, tag names.CloudTag) (cloud.Cloud, error) {
	c.MethodCall(c, "Cloud", tag)
	if c.cloud != nil {
		return *c.cloud, nil
	}
	if tag.Id() != "aws" {
		return cloud.Cloud{}, &params.Error{Code: params.CodeNotFound}
	}
	return cloud.Cloud{
		Name:      "aws",
		Type:      "ec2",
		AuthTypes: c.authTypes,
		Regions: []cloud.Region{
			{Name: "us-east-1"},
			{Name: "us-west-1"},
		},
	}, c.NextErr()
}

func (c *fakeCloudAPI) UserCredentials(ctx context.Context, user names.UserTag, cloud names.CloudTag) ([]names.CloudCredentialTag, error) {
	c.MethodCall(c, "UserCredentials", user, cloud)
	return c.credentials, c.NextErr()
}

func (c *fakeCloudAPI) AddCredential(ctx context.Context, tag string, credential cloud.Credential) error {
	c.MethodCall(c, "AddCredential", tag, credential)
	return c.NextErr()
}

type fakeProviderRegistry struct {
	testhelpers.Stub
	environs.ProviderRegistry
	provider environs.EnvironProvider
}

func (r *fakeProviderRegistry) Provider(providerType string) (environs.EnvironProvider, error) {
	r.MethodCall(r, "Provider", providerType)
	return r.provider, r.NextErr()
}

type fakeProvider struct {
	testhelpers.Stub
	environs.EnvironProvider
	detected *cloud.CloudCredential
}

func (p *fakeProvider) DetectCredentials(cloudName string) (*cloud.CloudCredential, error) {
	p.MethodCall(p, "DetectCredentials")
	return p.detected, p.NextErr()
}

func (p *fakeProvider) FinalizeCredential(
	ctx environs.FinalizeCredentialContext,
	args environs.FinalizeCredentialParams,
) (*cloud.Credential, error) {
	p.MethodCall(p, "FinalizeCredential", ctx, args)
	out := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{})
	out.Label = "finalized"
	return &out, p.NextErr()
}

func (p *fakeProvider) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{cloud.EmptyAuthType: {}}
}
