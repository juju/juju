// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
	_ "github.com/juju/juju/provider/ec2"
	"github.com/juju/juju/testing"
)

type AddModelSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fakeAddModelAPI      *fakeAddClient
	fakeCloudAPI         *fakeCloudAPI
	fakeProvider         *fakeProvider
	fakeProviderRegistry *fakeProviderRegistry
	store                *jujuclient.MemStore
}

var _ = gc.Suite(&AddModelSuite{})

func (s *AddModelSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	agentVersion, err := version.Parse("2.55.5")
	c.Assert(err, jc.ErrorIsNil)
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

func (s *AddModelSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command, _ := controller.NewAddModelCommandForTest(
		&fakeAPIConnection{},
		s.fakeAddModelAPI,
		s.fakeCloudAPI,
		s.store,
		s.fakeProviderRegistry,
	)
	return cmdtesting.RunCommand(c, command, args...)
}

func (s *AddModelSuite) TestInit(c *gc.C) {
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
			err: "model name is required",
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
			c.Assert(err, gc.ErrorMatches, test.err)
			continue
		}

		c.Assert(err, jc.ErrorIsNil)
		c.Assert(command.Name, gc.Equals, test.name)
		c.Assert(command.Owner, gc.Equals, test.owner)
		c.Assert(command.CloudRegion, gc.Equals, test.cloudRegion)
		attrs, err := command.Config.ReadAttrs(nil)
		c.Assert(err, jc.ErrorIsNil)
		if len(test.values) == 0 {
			c.Assert(attrs, gc.HasLen, 0)
		} else {
			c.Assert(attrs, jc.DeepEquals, test.values)
		}
	}
}

func (s *AddModelSuite) TestAddExistingName(c *gc.C) {
	// If there's any model details existing, we just overwrite them. The
	// controller will error out if the model already exists. Overwriting
	// means we'll replace any stale details from an previously existing
	// model with the same name.
	err := s.store.UpdateModel("test-master", "bob/test", jujuclient.ModelDetails{
		ModelUUID: "stale-uuid",
		ModelType: model.IAAS,
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.run(c, "test")
	c.Assert(err, jc.ErrorIsNil)

	details, err := s.store.ModelByName("test-master", "bob/test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details, jc.DeepEquals, &jujuclient.ModelDetails{
		ModelUUID: "fake-model-uuid", ModelType: model.IAAS})
}

func (s *AddModelSuite) TestAddModelUnauthorizedMentionsJujuGrant(c *gc.C) {
	s.fakeAddModelAPI.err = &params.Error{
		Message: "permission denied",
		Code:    params.CodeUnauthorized,
	}
	ctx, _ := s.run(c, "test")
	errString := strings.Replace(cmdtesting.Stderr(ctx), "\n", " ", -1)
	c.Assert(errString, gc.Matches, `.*juju grant.*`)
}

func (s *AddModelSuite) TestCredentialsPassedThrough(c *gc.C) {
	_, err := s.run(c, "test", "--credential", "secrets")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.fakeAddModelAPI.cloudCredential, gc.Equals, names.NewCloudCredentialTag("aws/bob/secrets"))
}

func (s *AddModelSuite) TestCredentialsOtherUserPassedThrough(c *gc.C) {
	_, err := s.run(c, "test", "--credential", "other/secrets")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.fakeAddModelAPI.cloudCredential, gc.Equals, names.NewCloudCredentialTag("aws/other/secrets"))
}

func (s *AddModelSuite) TestCredentialsOtherUserPassedThroughWhenCloud(c *gc.C) {
	_, err := s.run(c, "test", "--credential", "other/secrets", "aws/us-west-1")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.fakeAddModelAPI.cloudCredential, gc.Equals, names.NewCloudCredentialTag("aws/other/secrets"))
}

func (s *AddModelSuite) TestCredentialsOtherUserCredentialNotFound(c *gc.C) {
	// Have the API respond with no credentials.
	s.PatchValue(&s.fakeCloudAPI.credentials, []names.CloudCredentialTag{})

	_, err := s.run(c, "test", "--credential", "other/secrets")
	c.Assert(err, gc.ErrorMatches, "credential 'other/secrets' not found")

	// There should be no detection or UpdateCredentials call.
	s.fakeCloudAPI.CheckCallNames(c, "DefaultCloud", "Cloud", "UserCredentials")
}

func (s *AddModelSuite) TestCredentialsNoDefaultCloud(c *gc.C) {
	s.fakeCloudAPI.SetErrors(&params.Error{Code: params.CodeNotFound})
	_, err := s.run(c, "test", "--credential", "secrets")
	c.Assert(err, gc.ErrorMatches, `there is no default cloud defined, please specify one using:

    juju add-model \[flags\] \<model-name\> cloud\[/region\]`)
}

func (s *AddModelSuite) TestCredentialsOneCached(c *gc.C) {
	// Disable empty auth and clear the local credentials,
	// forcing a check for credentials in the controller.
	s.PatchValue(&s.fakeCloudAPI.authTypes, []cloud.AuthType{cloud.AccessKeyAuthType})
	delete(s.store.Credentials, "aws")

	// Cache just a single credential in the controller,
	// so it is selected automatically.
	credentialTag := names.NewCloudCredentialTag("aws/foo/secrets")
	s.PatchValue(&s.fakeCloudAPI.credentials, []names.CloudCredentialTag{credentialTag})

	_, err := s.run(c, "test", "aws/us-west-1")
	c.Assert(err, jc.ErrorIsNil)

	// The cached credential should be used, along with
	// the user-specified cloud region.
	c.Assert(s.fakeAddModelAPI.cloudCredential, gc.Equals, credentialTag)
	c.Assert(s.fakeAddModelAPI.cloudRegion, gc.Equals, "us-west-1")
}

func (s *AddModelSuite) TestControllerCredentialsDetected(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)

	credentialTag := names.NewCloudCredentialTag("aws/bob/default")
	credential := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{})
	credential.Label = "finalized"

	c.Assert(s.fakeAddModelAPI.cloudCredential, gc.Equals, credentialTag)
	s.fakeCloudAPI.CheckCallNames(c, "DefaultCloud", "Cloud", "UserCredentials")
}

func (s *AddModelSuite) TestControllerCredentialsDetectedAmbiguous(c *gc.C) {
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

	_, err := s.run(c, "test")
	c.Assert(err, gc.ErrorMatches, `
more than one credential detected. Add all detected credentials
to the client with:

    juju autoload-credentials

and then run the add-model command again with the --credential flag.`[1:])
}

func (s *AddModelSuite) TestCloudRegionPassedThrough(c *gc.C) {
	_, err := s.run(c, "test", "aws/us-west-1")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.fakeAddModelAPI.cloudName, gc.Equals, "aws")
	c.Assert(s.fakeAddModelAPI.cloudRegion, gc.Equals, "us-west-1")
}

func (s *AddModelSuite) TestDefaultCloudPassedThrough(c *gc.C) {
	_, err := s.run(c, "test")
	c.Assert(err, jc.ErrorIsNil)

	s.fakeCloudAPI.CheckCallNames(c, "DefaultCloud", "Cloud")
	c.Assert(s.fakeAddModelAPI.cloudName, gc.Equals, "aws")
	c.Assert(s.fakeAddModelAPI.cloudRegion, gc.Equals, "")
}

func (s *AddModelSuite) TestDefaultCloudRegionPassedThrough(c *gc.C) {
	_, err := s.run(c, "test", "us-west-1")
	c.Assert(err, jc.ErrorIsNil)

	s.fakeCloudAPI.CheckCalls(c, []gitjujutesting.StubCall{
		{"Cloud", []interface{}{names.NewCloudTag("us-west-1")}},
		{"DefaultCloud", nil},
		{"Cloud", []interface{}{names.NewCloudTag("aws")}},
	})
	c.Assert(s.fakeAddModelAPI.cloudName, gc.Equals, "aws")
	c.Assert(s.fakeAddModelAPI.cloudRegion, gc.Equals, "us-west-1")
}

func (s *AddModelSuite) TestNoDefaultCloudRegion(c *gc.C) {
	s.fakeCloudAPI.SetErrors(
		&params.Error{Code: params.CodeNotFound}, // no default region
	)
	_, err := s.run(c, "test", "us-west-1")
	c.Assert(err, gc.ErrorMatches, `
"us-west-1" is not a cloud supported by this controller,
and there is no default cloud. The clouds/regions supported
by this controller are:

Cloud  Regions
aws    us-east-1, us-west-1
lxd    
`[1:])
	s.fakeCloudAPI.CheckCalls(c, []gitjujutesting.StubCall{
		{"Cloud", []interface{}{names.NewCloudTag("us-west-1")}},
		{"DefaultCloud", nil},
		{"Clouds", nil},
	})
}

func (s *AddModelSuite) TestCloudUnspecifiedRegionPassedThrough(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.fakeAddModelAPI.cloudName, gc.Equals, "aws")
	c.Assert(s.fakeAddModelAPI.cloudRegion, gc.Equals, "")
}

func (s *AddModelSuite) TestCloudDefaultRegionUsedIfSet(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.fakeAddModelAPI.cloudName, gc.Equals, "aws")
	c.Assert(s.fakeAddModelAPI.cloudRegion, gc.Equals, "us-west-1")
}

func (s *AddModelSuite) TestInvalidCloudOrRegionName(c *gc.C) {
	_, err := s.run(c, "test", "oro")
	c.Assert(err, gc.ErrorMatches, `
"oro" is neither a cloud supported by this controller,
nor a region in the controller's default cloud "aws".
The clouds/regions supported by this controller are:

Cloud  Regions
aws    us-east-1, us-west-1
lxd    
`[1:])
}

func (s *AddModelSuite) TestComandLineConfigPassedThrough(c *gc.C) {
	_, err := s.run(c, "test", "--config", "account=magic", "--config", "cloud=special")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.fakeAddModelAPI.config["account"], gc.Equals, "magic")
	c.Assert(s.fakeAddModelAPI.config["cloud"], gc.Equals, "special")
}

func (s *AddModelSuite) TestConfigFileValuesPassedThrough(c *gc.C) {
	config := map[string]string{
		"account": "magic",
		"cloud":   "9",
	}
	bytes, err := yaml.Marshal(config)
	c.Assert(err, jc.ErrorIsNil)
	file, err := ioutil.TempFile(c.MkDir(), "")
	c.Assert(err, jc.ErrorIsNil)
	file.Write(bytes)
	file.Close()

	_, err = s.run(c, "test", "--config", file.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fakeAddModelAPI.config["account"], gc.Equals, "magic")
	c.Assert(s.fakeAddModelAPI.config["cloud"], gc.Equals, "9")
}

func (s *AddModelSuite) TestConfigFileWithNestedMaps(c *gc.C) {
	nestedConfig := map[string]interface{}{
		"account": "magic",
		"cloud":   "9",
	}
	config := map[string]interface{}{
		"foo":    "bar",
		"nested": nestedConfig,
	}

	bytes, err := yaml.Marshal(config)
	c.Assert(err, jc.ErrorIsNil)
	file, err := ioutil.TempFile(c.MkDir(), "")
	c.Assert(err, jc.ErrorIsNil)
	file.Write(bytes)
	file.Close()

	_, err = s.run(c, "test", "--config", file.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fakeAddModelAPI.config["foo"], gc.Equals, "bar")
	c.Assert(s.fakeAddModelAPI.config["nested"], jc.DeepEquals, nestedConfig)
}

func (s *AddModelSuite) TestConfigFileFailsToConform(c *gc.C) {
	nestedConfig := map[int]interface{}{
		9: "9",
	}
	config := map[string]interface{}{
		"foo":    "bar",
		"nested": nestedConfig,
	}
	bytes, err := yaml.Marshal(config)
	c.Assert(err, jc.ErrorIsNil)
	file, err := ioutil.TempFile(c.MkDir(), "")
	c.Assert(err, jc.ErrorIsNil)
	file.Write(bytes)
	file.Close()

	_, err = s.run(c, "test", "--config", file.Name())
	c.Assert(err, gc.ErrorMatches, `unable to parse config: map keyed with non-string value`)
}

func (s *AddModelSuite) TestConfigFileFormatError(c *gc.C) {
	file, err := ioutil.TempFile(c.MkDir(), "")
	c.Assert(err, jc.ErrorIsNil)
	file.Write(([]byte)("not: valid: yaml"))
	file.Close()

	_, err = s.run(c, "test", "--config", file.Name())
	c.Assert(err, gc.ErrorMatches, `unable to parse config: yaml: .*`)
}

func (s *AddModelSuite) TestConfigFileDoesntExist(c *gc.C) {
	_, err := s.run(c, "test", "--config", "missing-file")
	errMsg := ".*" + utils.NoSuchFileErrRegexp
	c.Assert(err, gc.ErrorMatches, errMsg)
}

func (s *AddModelSuite) TestConfigValuePrecedence(c *gc.C) {
	config := map[string]string{
		"account": "magic",
		"cloud":   "9",
	}
	bytes, err := yaml.Marshal(config)
	c.Assert(err, jc.ErrorIsNil)
	file, err := ioutil.TempFile(c.MkDir(), "")
	c.Assert(err, jc.ErrorIsNil)
	file.Write(bytes)
	file.Close()

	_, err = s.run(c, "test", "--config", file.Name(), "--config", "account=magic", "--config", "cloud=special")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fakeAddModelAPI.config["account"], gc.Equals, "magic")
	c.Assert(s.fakeAddModelAPI.config["cloud"], gc.Equals, "special")
}

func (s *AddModelSuite) TestAddErrorRemoveConfigstoreInfo(c *gc.C) {
	s.fakeAddModelAPI.err = errors.New("bah humbug")

	_, err := s.run(c, "test")
	c.Assert(err, gc.ErrorMatches, "bah humbug")

	_, err = s.store.ModelByName("test-master", "bob/test")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *AddModelSuite) TestAddStoresValues(c *gc.C) {
	const controllerName = "test-master"

	_, err := s.run(c, "test")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.store.CurrentControllerName, gc.Equals, controllerName)
	modelName, err := s.store.CurrentModel(controllerName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(modelName, gc.Equals, "bob/test")

	m, err := s.store.ModelByName(controllerName, modelName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m, jc.DeepEquals, &jujuclient.ModelDetails{ModelUUID: "fake-model-uuid", ModelType: model.IAAS})
}

func (s *AddModelSuite) TestNoSwitch(c *gc.C) {
	const controllerName = "test-master"
	checkNoModelSelected := func() {
		_, err := s.store.CurrentModel(controllerName)
		c.Check(err, jc.Satisfies, errors.IsNotFound)
	}
	checkNoModelSelected()

	_, err := s.run(c, "test", "--no-switch")
	c.Assert(err, jc.ErrorIsNil)

	// New model should not be selected by should still exist in the
	// store.
	checkNoModelSelected()
	m, err := s.store.ModelByName(controllerName, "bob/test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m, jc.DeepEquals, &jujuclient.ModelDetails{ModelUUID: "fake-model-uuid", ModelType: model.IAAS})
}

func (s *AddModelSuite) TestNoEnvCacheOtherUser(c *gc.C) {
	_, err := s.run(c, "test", "--owner", "zeus")
	c.Assert(err, jc.ErrorIsNil)

	// Creating a model for another user does not update the model cache.
	_, err = s.store.ModelByName("test-master", "bob/test")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
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

func (f *fakeAddClient) CreateModel(name, owner, cloudName, cloudRegion string, cloudCredential names.CloudCredentialTag, config map[string]interface{}) (base.ModelInfo, error) {
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
	controller.CloudAPI
	gitjujutesting.Stub
	authTypes   []cloud.AuthType
	credentials []names.CloudCredentialTag
}

func (c *fakeCloudAPI) DefaultCloud() (names.CloudTag, error) {
	c.MethodCall(c, "DefaultCloud")
	return names.NewCloudTag("aws"), c.NextErr()
}

func (c *fakeCloudAPI) Clouds() (map[names.CloudTag]cloud.Cloud, error) {
	c.MethodCall(c, "Clouds")
	return map[names.CloudTag]cloud.Cloud{
		names.NewCloudTag("aws"): {
			Name:      "aws",
			AuthTypes: c.authTypes,
			Regions: []cloud.Region{
				{Name: "us-east-1"},
				{Name: "us-west-1"},
			},
		},
		names.NewCloudTag("lxd"): {},
	}, c.NextErr()
}

func (c *fakeCloudAPI) Cloud(tag names.CloudTag) (cloud.Cloud, error) {
	c.MethodCall(c, "Cloud", tag)
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

func (c *fakeCloudAPI) UserCredentials(user names.UserTag, cloud names.CloudTag) ([]names.CloudCredentialTag, error) {
	c.MethodCall(c, "UserCredentials", user, cloud)
	return c.credentials, c.NextErr()
}

func (c *fakeCloudAPI) AddCredential(tag string, credential cloud.Credential) error {
	c.MethodCall(c, "AddCredential", tag, credential)
	return c.NextErr()
}

type fakeProviderRegistry struct {
	gitjujutesting.Stub
	environs.ProviderRegistry
	provider environs.EnvironProvider
}

func (r *fakeProviderRegistry) Provider(providerType string) (environs.EnvironProvider, error) {
	r.MethodCall(r, "Provider", providerType)
	return r.provider, r.NextErr()
}

type fakeProvider struct {
	gitjujutesting.Stub
	environs.EnvironProvider
	detected *cloud.CloudCredential
}

func (p *fakeProvider) DetectCredentials() (*cloud.CloudCredential, error) {
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
