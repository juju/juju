// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system_test

import (
	"io/ioutil"
	"os"
	"os/user"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/system"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/testing"
)

type createSuite struct {
	testing.FakeJujuHomeSuite
	fake       *fakeCreateClient
	store      configstore.Storage
	serverUUID string
	server     configstore.EnvironInfo
}

var _ = gc.Suite(&createSuite{})

func (s *createSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.SetFeatureFlags(feature.JES)
	s.fake = &fakeCreateClient{}
	store := configstore.Default
	s.AddCleanup(func(*gc.C) {
		configstore.Default = store
	})
	s.store = configstore.NewMem()
	configstore.Default = func() (configstore.Storage, error) {
		return s.store, nil
	}
	// Set up the current environment, and write just enough info
	// so we don't try to refresh
	envName := "test-master"
	s.serverUUID = "fake-server-uuid"
	info := s.store.CreateInfo(envName)
	info.SetAPIEndpoint(configstore.APIEndpoint{
		Addresses:   []string{"localhost"},
		CACert:      testing.CACert,
		EnvironUUID: s.serverUUID,
		ServerUUID:  s.serverUUID,
	})
	info.SetAPICredentials(configstore.APICredentials{User: "bob", Password: "sekrit"})
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)
	s.server = info
	err = envcmd.WriteCurrentEnvironment(envName)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *createSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command := system.NewCreateEnvironmentCommand(s.fake)
	return testing.RunCommand(c, envcmd.WrapSystem(command), args...)
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
			err: "environment name is required",
		}, {
			args: []string{"new-env"},
			name: "new-env",
		}, {
			args:  []string{"new-env", "--owner", "foo"},
			name:  "new-env",
			owner: "foo",
		}, {
			args: []string{"new-env", "--owner", "not=valid"},
			err:  `"not=valid" is not a valid user`,
		}, {
			args:   []string{"new-env", "key=value", "key2=value2"},
			name:   "new-env",
			values: map[string]string{"key": "value", "key2": "value2"},
		}, {
			args: []string{"new-env", "key=value", "key=value2"},
			err:  `key "key" specified more than once`,
		}, {
			args: []string{"new-env", "another"},
			err:  `expected "key=value", got "another"`,
		}, {
			args: []string{"new-env", "--config", "some-file"},
			name: "new-env",
			path: "some-file",
		},
	} {
		c.Logf("test %d", i)
		create := &system.CreateEnvironmentCommand{}
		err := testing.InitCommand(create, test.args)
		if test.err != "" {
			c.Assert(err, gc.ErrorMatches, test.err)
			continue
		}

		c.Assert(err, jc.ErrorIsNil)
		c.Assert(create.Name(), gc.Equals, test.name)
		c.Assert(create.Owner(), gc.Equals, test.owner)
		c.Assert(create.ConfigFile().Path, gc.Equals, test.path)
		// The config value parse method returns an empty map
		// if there were no values
		if len(test.values) == 0 {
			c.Assert(create.ConfValues(), gc.HasLen, 0)
		} else {
			c.Assert(create.ConfValues(), jc.DeepEquals, test.values)
		}
	}
}

func (s *createSuite) TestCreateExistingName(c *gc.C) {
	// Make a configstore entry with the same name.
	info := s.store.CreateInfo("test")
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.run(c, "test")
	c.Assert(err, gc.ErrorMatches, `environment "test" already exists`)
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

func (s *createSuite) TestConfigFileFormatError(c *gc.C) {
	file, err := ioutil.TempFile(c.MkDir(), "")
	c.Assert(err, jc.ErrorIsNil)
	file.Write(([]byte)("not: valid: yaml"))
	file.Close()

	_, err = s.run(c, "test", "--config", file.Name())
	c.Assert(err, gc.ErrorMatches, `YAML error: .*`)
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

var setConfigSpecialCaseDefaultsTests = []struct {
	about        string
	userEnvVar   string
	userCurrent  func() (*user.User, error)
	config       map[string]interface{}
	expectConfig map[string]interface{}
	expectError  string
}{{
	about:      "use env var if available",
	userEnvVar: "bob",
	config: map[string]interface{}{
		"name": "envname",
		"type": "local",
	},
	expectConfig: map[string]interface{}{
		"name":      "envname",
		"type":      "local",
		"namespace": "bob-envname",
	},
}, {
	about: "fall back to user.Current",
	userCurrent: func() (*user.User, error) {
		return &user.User{Username: "bob"}, nil
	},
	config: map[string]interface{}{
		"name": "envname",
		"type": "local",
	},
	expectConfig: map[string]interface{}{
		"name":      "envname",
		"type":      "local",
		"namespace": "bob-envname",
	},
}, {
	about:      "other provider types unaffected",
	userEnvVar: "bob",
	config: map[string]interface{}{
		"name": "envname",
		"type": "dummy",
	},
	expectConfig: map[string]interface{}{
		"name": "envname",
		"type": "dummy",
	},
}, {
	about: "explicit namespace takes precedence",
	userCurrent: func() (*user.User, error) {
		return &user.User{Username: "bob"}, nil
	},
	config: map[string]interface{}{
		"name":      "envname",
		"namespace": "something",
		"type":      "local",
	},
	expectConfig: map[string]interface{}{
		"name":      "envname",
		"namespace": "something",
		"type":      "local",
	},
}, {
	about: "user.Current returns error",
	userCurrent: func() (*user.User, error) {
		return nil, errors.New("an error")
	},
	config: map[string]interface{}{
		"name": "envname",
		"type": "local",
	},
	expectError: "failed to determine username for namespace: an error",
}}

func (s *createSuite) TestSetConfigSpecialCaseDefaults(c *gc.C) {
	noUserCurrent := func() (*user.User, error) {
		panic("should not be called")
	}
	s.PatchValue(system.UserCurrent, noUserCurrent)
	// We test setConfigSpecialCaseDefaults independently
	// because we can't use the local provider in the tests.
	for i, test := range setConfigSpecialCaseDefaultsTests {
		c.Logf("test %d: %s", i, test.about)
		os.Setenv("USER", test.userEnvVar)
		if test.userCurrent != nil {
			*system.UserCurrent = test.userCurrent
		} else {
			*system.UserCurrent = noUserCurrent
		}
		err := system.SetConfigSpecialCaseDefaults(test.config["name"].(string), test.config)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
		} else {
			c.Assert(err, gc.IsNil)
			c.Assert(test.config, jc.DeepEquals, test.expectConfig)
		}
	}

}

func (s *createSuite) TestCreateErrorRemoveConfigstoreInfo(c *gc.C) {
	s.fake.err = errors.New("bah humbug")

	_, err := s.run(c, "test")
	c.Assert(err, gc.ErrorMatches, "bah humbug")

	_, err = s.store.ReadInfo("test")
	c.Assert(err, gc.ErrorMatches, `environment "test" not found`)
}

func (s *createSuite) TestCreateStoresValues(c *gc.C) {
	s.fake.env = params.Environment{
		Name:       "test",
		UUID:       "fake-env-uuid",
		OwnerTag:   "ignored-for-now",
		ServerUUID: s.serverUUID,
	}
	_, err := s.run(c, "test")
	c.Assert(err, jc.ErrorIsNil)

	info, err := s.store.ReadInfo("test")
	c.Assert(err, jc.ErrorIsNil)
	// Stores the credentials of the original environment
	c.Assert(info.APICredentials(), jc.DeepEquals, s.server.APICredentials())
	endpoint := info.APIEndpoint()
	expected := s.server.APIEndpoint()
	c.Assert(endpoint.Addresses, jc.DeepEquals, expected.Addresses)
	c.Assert(endpoint.Hostnames, jc.DeepEquals, expected.Hostnames)
	c.Assert(endpoint.ServerUUID, gc.Equals, expected.ServerUUID)
	c.Assert(endpoint.CACert, gc.Equals, expected.CACert)
	c.Assert(endpoint.EnvironUUID, gc.Equals, "fake-env-uuid")
}

func (s *createSuite) TestNoEnvCacheOtherUser(c *gc.C) {
	s.fake.env = params.Environment{
		Name:       "test",
		UUID:       "fake-env-uuid",
		OwnerTag:   "ignored-for-now",
		ServerUUID: s.serverUUID,
	}
	_, err := s.run(c, "test", "--owner", "zeus")
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.store.ReadInfo("test")
	c.Assert(err, gc.ErrorMatches, `environment "test" not found`)
}

// fakeCreateClient is used to mock out the behavior of the real
// CreateEnvironment command.
type fakeCreateClient struct {
	owner   string
	account map[string]interface{}
	config  map[string]interface{}
	err     error
	env     params.Environment
}

var _ system.CreateEnvironmentAPI = (*fakeCreateClient)(nil)

func (*fakeCreateClient) Close() error {
	return nil
}

func (*fakeCreateClient) ConfigSkeleton(provider, region string) (params.EnvironConfig, error) {
	return params.EnvironConfig{
		"type":         "dummy",
		"state-server": false,
	}, nil
}
func (f *fakeCreateClient) CreateEnvironment(owner string, account, config map[string]interface{}) (params.Environment, error) {
	var env params.Environment
	if f.err != nil {
		return env, f.err
	}
	f.owner = owner
	f.account = account
	f.config = config
	return f.env, nil
}
