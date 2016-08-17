// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"fmt"
	"io/ioutil"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	_ "github.com/juju/juju/provider/ec2"
	"github.com/juju/juju/testing"
	"gopkg.in/juju/names.v2"
)

type addSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fakeAddModelAPI *fakeAddClient
	fakeCloundAPI   *fakeCloudAPI
	store           *jujuclienttesting.MemStore
}

var _ = gc.Suite(&addSuite{})

func (s *addSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.fakeAddModelAPI = &fakeAddClient{
		model: params.ModelInfo{
			Name:     "test",
			UUID:     "fake-model-uuid",
			OwnerTag: "ignored-for-now",
		},
	}
	s.fakeCloundAPI = &fakeCloudAPI{}

	// Set up the current controller, and write just enough info
	// so we don't try to refresh
	controllerName := "test-master"
	s.store = jujuclienttesting.NewMemStore()
	s.store.CurrentControllerName = controllerName
	s.store.Controllers[controllerName] = jujuclient.ControllerDetails{}
	s.store.Accounts[controllerName] = jujuclient.AccountDetails{
		User: "bob@local",
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

func (s *addSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command, _ := controller.NewAddModelCommandForTest(&fakeAPIConnection{}, s.fakeAddModelAPI, s.fakeCloundAPI, s.store)
	return testing.RunCommand(c, command, args...)
}

func (s *addSuite) TestInit(c *gc.C) {
	modelNameErr := "%q is not a valid name: model names may only contain lowercase letters, digits and hyphens"
	for i, test := range []struct {
		args   []string
		err    string
		name   string
		owner  string
		values map[string]interface{}
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
			args: []string{"new-model", "extra", "args"},
			err:  `unrecognized args: \["extra" "args"\]`,
		},
	} {
		c.Logf("test %d", i)
		wrappedCommand, command := controller.NewAddModelCommandForTest(nil, nil, nil, s.store)
		err := testing.InitCommand(wrappedCommand, test.args)
		if test.err != "" {
			c.Assert(err, gc.ErrorMatches, test.err)
			continue
		}

		c.Assert(err, jc.ErrorIsNil)
		c.Assert(command.Name, gc.Equals, test.name)
		c.Assert(command.Owner, gc.Equals, test.owner)
		attrs, err := command.Config.ReadAttrs(nil)
		c.Assert(err, jc.ErrorIsNil)
		if len(test.values) == 0 {
			c.Assert(attrs, gc.HasLen, 0)
		} else {
			c.Assert(attrs, jc.DeepEquals, test.values)
		}
	}
}

func (s *addSuite) TestAddExistingName(c *gc.C) {
	// If there's any model details existing, we just overwrite them. The
	// controller will error out if the model already exists. Overwriting
	// means we'll replace any stale details from an previously existing
	// model with the same name.
	err := s.store.UpdateModel("test-master", "bob@local/test", jujuclient.ModelDetails{
		"stale-uuid",
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.run(c, "test")
	c.Assert(err, jc.ErrorIsNil)

	details, err := s.store.ModelByName("test-master", "bob@local/test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details, jc.DeepEquals, &jujuclient.ModelDetails{"fake-model-uuid"})
}

func (s *addSuite) TestCredentialsPassedThrough(c *gc.C) {
	c.Skip("TODO(wallyworld) - port to using new credential management")
	_, err := s.run(c, "test", "--credential", "secrets")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.fakeAddModelAPI.cloudCredential, gc.Equals, "secrets")
	c.Assert(s.fakeAddModelAPI.config["type"], gc.Equals, "ec2")
}

func (s *addSuite) TestComandLineConfigPassedThrough(c *gc.C) {
	_, err := s.run(c, "test", "--config", "account=magic", "--config", "cloud=special")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.fakeAddModelAPI.config["account"], gc.Equals, "magic")
	c.Assert(s.fakeAddModelAPI.config["cloud"], gc.Equals, "special")
}

func (s *addSuite) TestConfigFileValuesPassedThrough(c *gc.C) {
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

func (s *addSuite) TestConfigFileWithNestedMaps(c *gc.C) {
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

func (s *addSuite) TestConfigFileFailsToConform(c *gc.C) {
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

func (s *addSuite) TestConfigFileFormatError(c *gc.C) {
	file, err := ioutil.TempFile(c.MkDir(), "")
	c.Assert(err, jc.ErrorIsNil)
	file.Write(([]byte)("not: valid: yaml"))
	file.Close()

	_, err = s.run(c, "test", "--config", file.Name())
	c.Assert(err, gc.ErrorMatches, `unable to parse config: yaml: .*`)
}

func (s *addSuite) TestConfigFileDoesntExist(c *gc.C) {
	_, err := s.run(c, "test", "--config", "missing-file")
	errMsg := ".*" + utils.NoSuchFileErrRegexp
	c.Assert(err, gc.ErrorMatches, errMsg)
}

func (s *addSuite) TestConfigValuePrecedence(c *gc.C) {
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

func (s *addSuite) TestAddErrorRemoveConfigstoreInfo(c *gc.C) {
	s.fakeAddModelAPI.err = errors.New("bah humbug")

	_, err := s.run(c, "test")
	c.Assert(err, gc.ErrorMatches, "bah humbug")

	_, err = s.store.ModelByName("test-master", "bob@local/test")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *addSuite) TestAddStoresValues(c *gc.C) {
	_, err := s.run(c, "test")
	c.Assert(err, jc.ErrorIsNil)

	model, err := s.store.ModelByName("test-master", "bob@local/test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model, jc.DeepEquals, &jujuclient.ModelDetails{"fake-model-uuid"})
}

func (s *addSuite) TestNoEnvCacheOtherUser(c *gc.C) {
	_, err := s.run(c, "test", "--owner", "zeus")
	c.Assert(err, jc.ErrorIsNil)

	// Creating a model for another user does not update the model cache.
	_, err = s.store.ModelByName("test-master", "bob@local/test")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

// fakeAddClient is used to mock out the behavior of the real
// AddModel command.
type fakeAddClient struct {
	owner           string
	cloudRegion     string
	cloudCredential names.CloudCredentialTag
	config          map[string]interface{}
	err             error
	model           params.ModelInfo
}

var _ controller.AddModelAPI = (*fakeAddClient)(nil)

func (*fakeAddClient) Close() error {
	return nil
}

func (f *fakeAddClient) CreateModel(name, owner, cloudRegion string, cloudCredential names.CloudCredentialTag, config map[string]interface{}) (params.ModelInfo, error) {
	if f.err != nil {
		return params.ModelInfo{}, f.err
	}
	f.owner = owner
	f.cloudCredential = cloudCredential
	f.cloudRegion = cloudRegion
	f.config = config
	return f.model, nil
}

// TODO(wallyworld) - improve this stub and add test asserts
type fakeCloudAPI struct {
	controller.CloudAPI
}

func (c *fakeCloudAPI) Credentials(names.UserTag, names.CloudTag) ([]names.CloudCredentialTag, error) {
	return []names.CloudCredentialTag{
		names.NewCloudCredentialTag("cloud/admin@local/default"),
	}, nil
}

func (c *fakeCloudAPI) UpdateCredential(names.CloudCredentialTag, cloud.Credential) error {
	return nil
}
