// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"io/ioutil"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/testing"
)

type createSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fake       *fakeCreateClient
	parser     func(interface{}) (interface{}, error)
	store      configstore.Storage
	serverUUID string
	server     configstore.EnvironInfo
}

var _ = gc.Suite(&createSuite{})

func (s *createSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.fake = &fakeCreateClient{
		env: params.Model{
			Name:     "test",
			UUID:     "fake-model-uuid",
			OwnerTag: "ignored-for-now",
		},
	}
	s.parser = nil
	store := configstore.Default
	s.AddCleanup(func(*gc.C) {
		configstore.Default = store
	})
	s.store = configstore.NewMem()
	configstore.Default = func() (configstore.Storage, error) {
		return s.store, nil
	}
	// Set up the current controller, and write just enough info
	// so we don't try to refresh
	controllerName := "local.test-master"
	s.serverUUID = "fake-server-uuid"
	info := s.store.CreateInfo("local.test-master:test-master")
	info.SetAPIEndpoint(configstore.APIEndpoint{
		Addresses:  []string{"localhost"},
		CACert:     testing.CACert,
		ModelUUID:  s.serverUUID,
		ServerUUID: s.serverUUID,
	})
	info.SetAPICredentials(configstore.APICredentials{User: "bob", Password: "sekrit"})
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)
	s.server = info
	err = modelcmd.WriteCurrentController(controllerName)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *createSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command, _ := controller.NewCreateModelCommandForTest(s.fake, s.parser)
	return testing.RunCommand(c, command, args...)
}

func (s *createSuite) TestInit(c *gc.C) {

	for i, test := range []struct {
		args   []string
		err    string
		name   string
		owner  string
		path   string
		values map[string]string
	}{
		{
			err: "model name is required",
		}, {
			args: []string{"new-model"},
			name: "new-model",
		}, {
			args:  []string{"new-model", "--owner", "foo"},
			name:  "new-model",
			owner: "foo",
		}, {
			args: []string{"new-model", "--owner", "not=valid"},
			err:  `"not=valid" is not a valid user`,
		}, {
			args:   []string{"new-model", "key=value", "key2=value2"},
			name:   "new-model",
			values: map[string]string{"key": "value", "key2": "value2"},
		}, {
			args: []string{"new-model", "key=value", "key=value2"},
			err:  `key "key" specified more than once`,
		}, {
			args: []string{"new-model", "another"},
			err:  `expected "key=value", got "another"`,
		}, {
			args: []string{"new-model", "--config", "some-file"},
			name: "new-model",
			path: "some-file",
		},
	} {
		c.Logf("test %d", i)
		wrappedCommand, command := controller.NewCreateModelCommandForTest(nil, nil)
		err := testing.InitCommand(wrappedCommand, test.args)
		if test.err != "" {
			c.Assert(err, gc.ErrorMatches, test.err)
			continue
		}

		c.Assert(err, jc.ErrorIsNil)
		c.Assert(command.Name, gc.Equals, test.name)
		c.Assert(command.Owner, gc.Equals, test.owner)
		c.Assert(command.ConfigFile.Path, gc.Equals, test.path)
		// The config value parse method returns an empty map
		// if there were no values
		if len(test.values) == 0 {
			c.Assert(command.ConfValues, gc.HasLen, 0)
		} else {
			c.Assert(command.ConfValues, jc.DeepEquals, test.values)
		}
	}
}

func (s *createSuite) TestCreateExistingName(c *gc.C) {
	// Make a configstore entry with the same name.
	info := s.store.CreateInfo("local.test-master:test")
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.run(c, "test")
	c.Assert(err, gc.ErrorMatches, `model "test" already exists`)
}

func (s *createSuite) TestComandLineConfigPassedThrough(c *gc.C) {
	_, err := s.run(c, "test", "account=magic", "cloud=special")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.fake.config["account"], gc.Equals, "magic")
	c.Assert(s.fake.config["cloud"], gc.Equals, "special")
}

func (s *createSuite) TestConfigFileValuesPassedThrough(c *gc.C) {
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
	c.Assert(s.fake.config["account"], gc.Equals, "magic")
	c.Assert(s.fake.config["cloud"], gc.Equals, "9")
}

func (s *createSuite) TestConfigFileWithNestedMaps(c *gc.C) {
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
	c.Assert(s.fake.config["foo"], gc.Equals, "bar")
	c.Assert(s.fake.config["nested"], jc.DeepEquals, nestedConfig)
}

func (s *createSuite) TestConfigFileFailsToConform(c *gc.C) {
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
	c.Assert(err, gc.ErrorMatches, `unable to parse config file: map keyed with non-string value`)
}

func (s *createSuite) TestConfigFileFailsWithUnknownType(c *gc.C) {
	config := map[string]interface{}{
		"account": "magic",
		"cloud":   "9",
	}

	bytes, err := yaml.Marshal(config)
	c.Assert(err, jc.ErrorIsNil)
	file, err := ioutil.TempFile(c.MkDir(), "")
	c.Assert(err, jc.ErrorIsNil)
	file.Write(bytes)
	file.Close()

	s.parser = func(interface{}) (interface{}, error) { return "not a map", nil }
	_, err = s.run(c, "test", "--config", file.Name())
	c.Assert(err, gc.ErrorMatches, `config must contain a YAML map with string keys`)
}

func (s *createSuite) TestConfigFileFormatError(c *gc.C) {
	file, err := ioutil.TempFile(c.MkDir(), "")
	c.Assert(err, jc.ErrorIsNil)
	file.Write(([]byte)("not: valid: yaml"))
	file.Close()

	_, err = s.run(c, "test", "--config", file.Name())
	c.Assert(err, gc.ErrorMatches, `unable to parse config file: yaml: .*`)
}

func (s *createSuite) TestConfigFileDoesntExist(c *gc.C) {
	_, err := s.run(c, "test", "--config", "missing-file")
	errMsg := ".*" + utils.NoSuchFileErrRegexp
	c.Assert(err, gc.ErrorMatches, errMsg)
}

func (s *createSuite) TestConfigValuePrecedence(c *gc.C) {
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

	_, err = s.run(c, "test", "--config", file.Name(), "account=magic", "cloud=special")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.config["account"], gc.Equals, "magic")
	c.Assert(s.fake.config["cloud"], gc.Equals, "special")
}

func (s *createSuite) TestCreateErrorRemoveConfigstoreInfo(c *gc.C) {
	s.fake.err = errors.New("bah humbug")

	_, err := s.run(c, "test")
	c.Assert(err, gc.ErrorMatches, "bah humbug")

	_, err = s.store.ReadInfo("local.test-master:test")
	c.Assert(err, gc.ErrorMatches, `model "local.test-master:test" not found`)
}

func (s *createSuite) TestCreateStoresValues(c *gc.C) {
	_, err := s.run(c, "test")
	c.Assert(err, jc.ErrorIsNil)

	info, err := s.store.ReadInfo("local.test-master:test")
	c.Assert(err, jc.ErrorIsNil)
	// Stores the credentials of the original environment
	c.Assert(info.APICredentials(), jc.DeepEquals, s.server.APICredentials())
	endpoint := info.APIEndpoint()
	expected := s.server.APIEndpoint()
	c.Assert(endpoint.Addresses, jc.DeepEquals, expected.Addresses)
	c.Assert(endpoint.Hostnames, jc.DeepEquals, expected.Hostnames)
	c.Assert(endpoint.ServerUUID, gc.Equals, expected.ServerUUID)
	c.Assert(endpoint.CACert, gc.Equals, expected.CACert)
	c.Assert(endpoint.ModelUUID, gc.Equals, "fake-model-uuid")
}

func (s *createSuite) TestNoEnvCacheOtherUser(c *gc.C) {
	_, err := s.run(c, "test", "--owner", "zeus")
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.store.ReadInfo("local.test-master:test")
	c.Assert(err, gc.ErrorMatches, `model "local.test-master:test" not found`)
}

// fakeCreateClient is used to mock out the behavior of the real
//  CreateModel command.
type fakeCreateClient struct {
	owner   string
	account map[string]interface{}
	config  map[string]interface{}
	err     error
	env     params.Model
}

var _ controller.CreateEnvironmentAPI = (*fakeCreateClient)(nil)

func (*fakeCreateClient) Close() error {
	return nil
}

func (*fakeCreateClient) ConfigSkeleton(provider, region string) (params.ModelConfig, error) {
	return params.ModelConfig{
		"type":       "dummy",
		"controller": false,
	}, nil
}
func (f *fakeCreateClient) CreateModel(owner string, account, config map[string]interface{}) (params.Model, error) {
	if f.err != nil {
		return params.Model{}, f.err
	}
	f.owner = owner
	f.account = account
	f.config = config
	return f.env, nil
}
