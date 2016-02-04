// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	"fmt"
	"path/filepath"
	"reflect"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&suite{})

var agentConfigTests = []struct {
	about         string
	params        agent.AgentConfigParams
	checkErr      string
	inspectConfig func(*gc.C, agent.Config)
}{{
	about:    "missing data directory",
	checkErr: "data directory not found in configuration",
}, {
	about: "missing tag",
	params: agent.AgentConfigParams{
		Paths: agent.Paths{DataDir: "/data/dir"},
	},
	checkErr: "entity tag not found in configuration",
}, {
	about: "missing upgraded to version",
	params: agent.AgentConfigParams{
		Paths: agent.Paths{DataDir: "/data/dir"},
		Tag:   names.NewMachineTag("1"),
	},
	checkErr: "upgradedToVersion not found in configuration",
}, {
	about: "missing password",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		UpgradedToVersion: version.Current,
	},
	checkErr: "password not found in configuration",
}, {
	about: "missing model tag",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		UpgradedToVersion: version.Current,
		Password:          "sekrit",
	},
	checkErr: "model not found in configuration",
}, {
	about: "invalid model tag",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		UpgradedToVersion: version.Current,
		Password:          "sekrit",
		Model:             names.NewModelTag("uuid"),
	},
	checkErr: `"uuid" is not a valid model uuid`,
}, {
	about: "missing CA cert",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		UpgradedToVersion: version.Current,
		Password:          "sekrit",
		Model:             testing.ModelTag,
	},
	checkErr: "CA certificate not found in configuration",
}, {
	about: "need either state or api addresses",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		UpgradedToVersion: version.Current,
		Password:          "sekrit",
		CACert:            "ca cert",
		Model:             testing.ModelTag,
	},
	checkErr: "state or API addresses not found in configuration",
}, {
	about: "invalid state address",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		UpgradedToVersion: version.Current,
		Password:          "sekrit",
		CACert:            "ca cert",
		Model:             testing.ModelTag,
		StateAddresses:    []string{"localhost:8080", "bad-address"},
	},
	checkErr: `invalid controller address "bad-address"`,
}, {
	about: "invalid api address",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		UpgradedToVersion: version.Current,
		Password:          "sekrit",
		CACert:            "ca cert",
		Model:             testing.ModelTag,
		APIAddresses:      []string{"localhost:8080", "bad-address"},
	},
	checkErr: `invalid API server address "bad-address"`,
}, {
	about: "good state addresses",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		UpgradedToVersion: version.Current,
		Password:          "sekrit",
		CACert:            "ca cert",
		Model:             testing.ModelTag,
		StateAddresses:    []string{"localhost:1234"},
	},
}, {
	about: "good api addresses",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		UpgradedToVersion: version.Current,
		Password:          "sekrit",
		CACert:            "ca cert",
		Model:             testing.ModelTag,
		APIAddresses:      []string{"localhost:1234"},
	},
}, {
	about: "both state and api addresses",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		UpgradedToVersion: version.Current,
		Password:          "sekrit",
		CACert:            "ca cert",
		Model:             testing.ModelTag,
		StateAddresses:    []string{"localhost:1234"},
		APIAddresses:      []string{"localhost:1235"},
	},
}, {
	about: "everything...",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		Password:          "sekrit",
		UpgradedToVersion: version.Current,
		CACert:            "ca cert",
		Model:             testing.ModelTag,
		StateAddresses:    []string{"localhost:1234"},
		APIAddresses:      []string{"localhost:1235"},
		Nonce:             "a nonce",
	},
}, {
	about: "missing logDir sets default",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		Password:          "sekrit",
		UpgradedToVersion: version.Current,
		CACert:            "ca cert",
		Model:             testing.ModelTag,
		StateAddresses:    []string{"localhost:1234"},
		APIAddresses:      []string{"localhost:1235"},
		Nonce:             "a nonce",
	},
	inspectConfig: func(c *gc.C, cfg agent.Config) {
		c.Check(cfg.LogDir(), gc.Equals, agent.DefaultPaths.LogDir)
	},
}, {
	about: "missing metricsSpoolDir sets default",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		Password:          "sekrit",
		UpgradedToVersion: version.Current,
		CACert:            "ca cert",
		Model:             testing.ModelTag,
		StateAddresses:    []string{"localhost:1234"},
		APIAddresses:      []string{"localhost:1235"},
		Nonce:             "a nonce",
	},
	inspectConfig: func(c *gc.C, cfg agent.Config) {
		c.Check(cfg.MetricsSpoolDir(), gc.Equals, agent.DefaultPaths.MetricsSpoolDir)
	},
}, {
	about: "setting a custom metricsSpoolDir",
	params: agent.AgentConfigParams{
		Paths: agent.Paths{
			DataDir:         "/data/dir",
			MetricsSpoolDir: "/tmp/nowhere",
		},
		Tag:               names.NewMachineTag("1"),
		Password:          "sekrit",
		UpgradedToVersion: version.Current,
		CACert:            "ca cert",
		Model:             testing.ModelTag,
		StateAddresses:    []string{"localhost:1234"},
		APIAddresses:      []string{"localhost:1235"},
		Nonce:             "a nonce",
	},
	inspectConfig: func(c *gc.C, cfg agent.Config) {
		c.Check(cfg.MetricsSpoolDir(), gc.Equals, "/tmp/nowhere")
	},
}, {
	about: "agentConfig must not be a User tag",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewUserTag("admin"), // this is a joke, the admin user is nil.
		UpgradedToVersion: version.Current,
		Password:          "sekrit",
	},
	checkErr: "entity tag must be MachineTag or UnitTag, got names.UserTag",
}, {
	about: "agentConfig accepts a Unit tag",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewUnitTag("ubuntu/1"),
		Password:          "sekrit",
		UpgradedToVersion: version.Current,
		Model:             testing.ModelTag,
		CACert:            "ca cert",
		StateAddresses:    []string{"localhost:1234"},
		APIAddresses:      []string{"localhost:1235"},
	},
	inspectConfig: func(c *gc.C, cfg agent.Config) {
		c.Check(cfg.Dir(), gc.Equals, "/data/dir/agents/unit-ubuntu-1")
	},
}, {
	about: "prefer-ipv6 parsed when set",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		Password:          "sekrit",
		UpgradedToVersion: version.Current,
		CACert:            "ca cert",
		Model:             testing.ModelTag,
		StateAddresses:    []string{"localhost:1234"},
		APIAddresses:      []string{"localhost:1235"},
		Nonce:             "a nonce",
		PreferIPv6:        true,
	},
	inspectConfig: func(c *gc.C, cfg agent.Config) {
		c.Check(cfg.PreferIPv6(), jc.IsTrue)
	},
}, {
	about: "missing prefer-ipv6 defaults to false",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		Password:          "sekrit",
		UpgradedToVersion: version.Current,
		CACert:            "ca cert",
		Model:             testing.ModelTag,
		StateAddresses:    []string{"localhost:1234"},
		APIAddresses:      []string{"localhost:1235"},
		Nonce:             "a nonce",
	},
	inspectConfig: func(c *gc.C, cfg agent.Config) {
		c.Check(cfg.PreferIPv6(), jc.IsFalse)
	},
}}

func (*suite) TestNewAgentConfig(c *gc.C) {
	for i, test := range agentConfigTests {
		c.Logf("%v: %s", i, test.about)
		config, err := agent.NewAgentConfig(test.params)
		if test.checkErr == "" {
			c.Assert(err, jc.ErrorIsNil)
			if test.inspectConfig != nil {
				test.inspectConfig(c, config)
			}
		} else {
			c.Assert(err, gc.ErrorMatches, test.checkErr)
		}
	}
}

func (*suite) TestMigrate(c *gc.C) {
	initialParams := agent.AgentConfigParams{
		Paths: agent.Paths{
			DataDir: c.MkDir(),
			LogDir:  c.MkDir(),
		},
		Tag:               names.NewMachineTag("1"),
		Nonce:             "nonce",
		Password:          "secret",
		UpgradedToVersion: version.MustParse("1.16.5"),
		Jobs: []multiwatcher.MachineJob{
			multiwatcher.JobManageModel,
			multiwatcher.JobHostUnits,
		},
		CACert:         "ca cert",
		Model:          testing.ModelTag,
		StateAddresses: []string{"localhost:1234"},
		APIAddresses:   []string{"localhost:4321"},
		Values: map[string]string{
			"key1": "value1",
			"key2": "value2",
			"key3": "value3",
		},
	}

	migrateTests := []struct {
		comment      string
		fields       []string
		newParams    agent.MigrateParams
		expectValues map[string]string
		expectErr    string
	}{{
		comment:   "nothing to change",
		fields:    nil,
		newParams: agent.MigrateParams{},
	}, {
		fields: []string{"Paths"},
		newParams: agent.MigrateParams{
			Paths: agent.Paths{DataDir: c.MkDir()},
		},
	}, {
		fields: []string{"Paths"},
		newParams: agent.MigrateParams{
			Paths: agent.Paths{
				DataDir: c.MkDir(),
				LogDir:  c.MkDir(),
			},
		},
	}, {
		fields: []string{"Jobs"},
		newParams: agent.MigrateParams{
			Jobs: []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		},
	}, {
		comment:   "invalid/immutable field specified",
		fields:    []string{"InvalidField"},
		newParams: agent.MigrateParams{},
		expectErr: `unknown field "InvalidField"`,
	}, {
		comment: "Values can be added, changed or removed",
		fields:  []string{"Values", "DeleteValues"},
		newParams: agent.MigrateParams{
			DeleteValues: []string{"key2", "key3"}, // delete
			Values: map[string]string{
				"key1":     "new value1", // change
				"new key3": "value3",     // add
				"empty":    "",           // add empty val
			},
		},
		expectValues: map[string]string{
			"key1":     "new value1",
			"new key3": "value3",
			"empty":    "",
		},
	}}
	for i, test := range migrateTests {
		summary := "migrate fields"
		if test.comment != "" {
			summary += " (" + test.comment + ") "
		}
		c.Logf("test %d: %s %v", i, summary, test.fields)

		initialConfig, err := agent.NewAgentConfig(initialParams)
		c.Assert(err, jc.ErrorIsNil)

		newConfig, err := agent.NewAgentConfig(initialParams)
		c.Assert(err, jc.ErrorIsNil)

		c.Assert(initialConfig.Write(), gc.IsNil)
		c.Assert(agent.ConfigFileExists(initialConfig), jc.IsTrue)

		err = newConfig.Migrate(test.newParams)
		c.Assert(err, jc.ErrorIsNil)
		err = newConfig.Write()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(agent.ConfigFileExists(newConfig), jc.IsTrue)

		// Make sure we can read it back successfully and it
		// matches what we wrote.
		configPath := agent.ConfigPath(newConfig.DataDir(), newConfig.Tag())
		c.Logf("new config path: %v", configPath)
		readConfig, err := agent.ReadConfig(configPath)
		c.Check(err, jc.ErrorIsNil)
		c.Check(newConfig, jc.DeepEquals, readConfig)

		// Make sure only the specified fields were changed and
		// the rest matches.
		for _, field := range test.fields {
			switch field {
			case "Values":
				err = agent.PatchConfig(initialConfig, field, test.expectValues)
				c.Check(err, jc.ErrorIsNil)
			case "DeleteValues":
				err = agent.PatchConfig(initialConfig, field, test.newParams.DeleteValues)
				c.Check(err, jc.ErrorIsNil)
			default:
				value := reflect.ValueOf(test.newParams).FieldByName(field)
				if value.IsValid() && test.expectErr == "" {
					err = agent.PatchConfig(initialConfig, field, value.Interface())
					c.Check(err, jc.ErrorIsNil)
				} else {
					err = agent.PatchConfig(initialConfig, field, value)
					c.Check(err, gc.ErrorMatches, test.expectErr)
				}
			}
		}
		c.Check(newConfig, jc.DeepEquals, initialConfig)
	}
}

func stateServingInfo() params.StateServingInfo {
	return params.StateServingInfo{
		Cert:           "cert",
		PrivateKey:     "key",
		CAPrivateKey:   "ca key",
		StatePort:      69,
		APIPort:        47,
		SharedSecret:   "shared",
		SystemIdentity: "identity",
	}
}

func (*suite) TestNewStateMachineConfig(c *gc.C) {
	type testStruct struct {
		about         string
		params        agent.AgentConfigParams
		servingInfo   params.StateServingInfo
		checkErr      string
		inspectConfig func(*gc.C, agent.Config)
	}
	var tests = []testStruct{{
		about:    "missing controller cert",
		checkErr: "controller cert not found in configuration",
	}, {
		about: "missing controller key",
		servingInfo: params.StateServingInfo{
			Cert: "server cert",
		},
		checkErr: "controller key not found in configuration",
	}, {
		about: "missing ca cert key",
		servingInfo: params.StateServingInfo{
			Cert:       "server cert",
			PrivateKey: "server key",
		},
		checkErr: "ca cert key not found in configuration",
	}, {
		about: "missing state port",
		servingInfo: params.StateServingInfo{
			Cert:         "server cert",
			PrivateKey:   "server key",
			CAPrivateKey: "ca key",
		},
		checkErr: "state port not found in configuration",
	}, {
		about: "params api port",
		servingInfo: params.StateServingInfo{
			Cert:         "server cert",
			PrivateKey:   "server key",
			CAPrivateKey: "ca key",
			StatePort:    69,
		},
		checkErr: "api port not found in configuration",
	}}
	for _, test := range agentConfigTests {
		tests = append(tests, testStruct{
			about:       test.about,
			params:      test.params,
			servingInfo: stateServingInfo(),
			checkErr:    test.checkErr,
		})
	}

	for i, test := range tests {
		c.Logf("%v: %s", i, test.about)
		cfg, err := agent.NewStateMachineConfig(test.params, test.servingInfo)
		if test.checkErr == "" {
			c.Assert(err, jc.ErrorIsNil)
			if test.inspectConfig != nil {
				test.inspectConfig(c, cfg)
			}
		} else {
			c.Assert(err, gc.ErrorMatches, test.checkErr)
		}
	}
}

var attributeParams = agent.AgentConfigParams{
	Paths: agent.Paths{
		DataDir: "/data/dir",
	},
	Tag:               names.NewMachineTag("1"),
	UpgradedToVersion: version.Current,
	Password:          "sekrit",
	CACert:            "ca cert",
	StateAddresses:    []string{"localhost:1234"},
	APIAddresses:      []string{"localhost:1235"},
	Nonce:             "a nonce",
	Model:             testing.ModelTag,
}

func (*suite) TestAttributes(c *gc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conf.DataDir(), gc.Equals, "/data/dir")
	compareSystemIdentityPath := filepath.FromSlash("/data/dir/system-identity")
	systemIdentityPath := filepath.FromSlash(conf.SystemIdentityPath())
	c.Assert(systemIdentityPath, gc.Equals, compareSystemIdentityPath)
	c.Assert(conf.Tag(), gc.Equals, names.NewMachineTag("1"))
	c.Assert(conf.Dir(), gc.Equals, "/data/dir/agents/machine-1")
	c.Assert(conf.Nonce(), gc.Equals, "a nonce")
	c.Assert(conf.UpgradedToVersion(), jc.DeepEquals, version.Current)
}

func (*suite) TestStateServingInfo(c *gc.C) {
	servingInfo := stateServingInfo()
	conf, err := agent.NewStateMachineConfig(attributeParams, servingInfo)
	c.Assert(err, jc.ErrorIsNil)
	gotInfo, ok := conf.StateServingInfo()
	c.Assert(ok, jc.IsTrue)
	c.Assert(gotInfo, jc.DeepEquals, servingInfo)
	newInfo := params.StateServingInfo{
		APIPort:        147,
		StatePort:      169,
		Cert:           "new cert",
		PrivateKey:     "new key",
		CAPrivateKey:   "new ca key",
		SharedSecret:   "new shared",
		SystemIdentity: "new identity",
	}
	conf.SetStateServingInfo(newInfo)
	gotInfo, ok = conf.StateServingInfo()
	c.Assert(ok, jc.IsTrue)
	c.Assert(gotInfo, jc.DeepEquals, newInfo)
}

func (*suite) TestStateServingInfoNotAvailable(c *gc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, jc.ErrorIsNil)

	_, available := conf.StateServingInfo()
	c.Assert(available, jc.IsFalse)
}

func (s *suite) TestAPIAddressesCannotWriteBack(c *gc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, jc.ErrorIsNil)
	value, err := conf.APIAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(value, jc.DeepEquals, []string{"localhost:1235"})
	value[0] = "invalidAdr"
	//Check out change hasn't gone back into the internals
	newValue, err := conf.APIAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newValue, jc.DeepEquals, []string{"localhost:1235"})
}

func (*suite) TestWriteAndRead(c *gc.C) {
	testParams := attributeParams
	testParams.Paths.DataDir = c.MkDir()
	testParams.Paths.LogDir = c.MkDir()
	conf, err := agent.NewAgentConfig(testParams)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(conf.Write(), gc.IsNil)
	reread, err := agent.ReadConfig(agent.ConfigPath(conf.DataDir(), conf.Tag()))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(reread, jc.DeepEquals, conf)
}

func (*suite) TestAPIInfoMissingAddress(c *gc.C) {
	conf := agent.EmptyConfig()
	_, ok := conf.APIInfo()
	c.Assert(ok, jc.IsFalse)
}

func (*suite) TestAPIInfoAddsLocalhostWhenServingInfoPresent(c *gc.C) {
	attrParams := attributeParams
	servingInfo := stateServingInfo()
	conf, err := agent.NewStateMachineConfig(attrParams, servingInfo)
	c.Assert(err, jc.ErrorIsNil)
	apiinfo, ok := conf.APIInfo()
	c.Assert(ok, jc.IsTrue)
	c.Check(apiinfo.Addrs, gc.HasLen, len(attrParams.APIAddresses)+1)
	localhostAddressFound := false
	for _, eachApiAddress := range apiinfo.Addrs {
		if eachApiAddress == "localhost:47" {
			localhostAddressFound = true
			break
		}
	}
	c.Assert(localhostAddressFound, jc.IsTrue)
}

func (*suite) TestAPIInfoAddsLocalhostWhenServingInfoPresentAndPreferIPv6On(c *gc.C) {
	attrParams := attributeParams
	attrParams.PreferIPv6 = true
	servingInfo := stateServingInfo()
	conf, err := agent.NewStateMachineConfig(attrParams, servingInfo)
	c.Assert(err, jc.ErrorIsNil)
	apiinfo, ok := conf.APIInfo()
	c.Assert(ok, jc.IsTrue)
	c.Check(apiinfo.Addrs, gc.HasLen, len(attrParams.APIAddresses)+1)
	localhostAddressFound := false
	for _, eachApiAddress := range apiinfo.Addrs {
		if eachApiAddress == "[::1]:47" {
			localhostAddressFound = true
			break
		}
		c.Check(eachApiAddress, gc.Not(gc.Equals), "localhost:47")
	}
	c.Assert(localhostAddressFound, jc.IsTrue)
}

func (*suite) TestMongoInfoHonorsPreferIPv6(c *gc.C) {
	attrParams := attributeParams
	attrParams.PreferIPv6 = true
	servingInfo := stateServingInfo()
	conf, err := agent.NewStateMachineConfig(attrParams, servingInfo)
	c.Assert(err, jc.ErrorIsNil)
	mongoInfo, ok := conf.MongoInfo()
	c.Assert(ok, jc.IsTrue)
	c.Check(mongoInfo.Info.Addrs, jc.DeepEquals, []string{"[::1]:69"})

	attrParams.PreferIPv6 = false
	conf, err = agent.NewStateMachineConfig(attrParams, servingInfo)
	c.Assert(err, jc.ErrorIsNil)
	mongoInfo, ok = conf.MongoInfo()
	c.Assert(ok, jc.IsTrue)
	c.Check(mongoInfo.Info.Addrs, jc.DeepEquals, []string{"127.0.0.1:69"})
}

func (*suite) TestAPIInfoDoesntAddLocalhostWhenNoServingInfoPreferIPv6Off(c *gc.C) {
	attrParams := attributeParams
	attrParams.PreferIPv6 = false
	conf, err := agent.NewAgentConfig(attrParams)
	c.Assert(err, jc.ErrorIsNil)
	apiinfo, ok := conf.APIInfo()
	c.Assert(ok, jc.IsTrue)
	c.Assert(apiinfo.Addrs, gc.DeepEquals, attrParams.APIAddresses)
}

func (*suite) TestAPIInfoDoesntAddLocalhostWhenNoServingInfoPreferIPv6On(c *gc.C) {
	attrParams := attributeParams
	attrParams.PreferIPv6 = true
	conf, err := agent.NewAgentConfig(attrParams)
	c.Assert(err, jc.ErrorIsNil)
	apiinfo, ok := conf.APIInfo()
	c.Assert(ok, jc.IsTrue)
	c.Assert(apiinfo.Addrs, gc.DeepEquals, attrParams.APIAddresses)
}

func (*suite) TestSetPassword(c *gc.C) {
	attrParams := attributeParams
	servingInfo := stateServingInfo()
	servingInfo.APIPort = 1235
	conf, err := agent.NewStateMachineConfig(attrParams, servingInfo)
	c.Assert(err, jc.ErrorIsNil)

	expectAPIInfo := &api.Info{
		Addrs:    attrParams.APIAddresses,
		CACert:   attrParams.CACert,
		Tag:      attrParams.Tag,
		Password: "",
		Nonce:    attrParams.Nonce,
		ModelTag: attrParams.Model,
	}
	apiInfo, ok := conf.APIInfo()
	c.Assert(ok, jc.IsTrue)
	c.Assert(apiInfo, jc.DeepEquals, expectAPIInfo)
	addr := fmt.Sprintf("127.0.0.1:%d", servingInfo.StatePort)
	expectStateInfo := &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{addr},
			CACert: attrParams.CACert,
		},
		Tag:      attrParams.Tag,
		Password: "",
	}
	info, ok := conf.MongoInfo()
	c.Assert(ok, jc.IsTrue)
	c.Assert(info, jc.DeepEquals, expectStateInfo)

	conf.SetPassword("newpassword")

	expectAPIInfo.Password = "newpassword"
	expectStateInfo.Password = "newpassword"

	apiInfo, ok = conf.APIInfo()
	c.Assert(ok, jc.IsTrue)
	c.Assert(apiInfo, jc.DeepEquals, expectAPIInfo)
	info, ok = conf.MongoInfo()
	c.Assert(ok, jc.IsTrue)
	c.Assert(info, jc.DeepEquals, expectStateInfo)
}

func (*suite) TestSetOldPassword(c *gc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(conf.OldPassword(), gc.Equals, attributeParams.Password)
	conf.SetOldPassword("newoldpassword")
	c.Assert(conf.OldPassword(), gc.Equals, "newoldpassword")
}

func (*suite) TestSetUpgradedToVersion(c *gc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(conf.UpgradedToVersion(), gc.Equals, version.Current)

	expectVers := version.MustParse("3.4.5")
	conf.SetUpgradedToVersion(expectVers)
	c.Assert(conf.UpgradedToVersion(), gc.Equals, expectVers)
}

func (*suite) TestSetAPIHostPorts(c *gc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, jc.ErrorIsNil)

	addrs, err := conf.APIAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.DeepEquals, attributeParams.APIAddresses)

	// All the best candidate addresses for each server are
	// used. Cloud-local addresses are preferred.  Otherwise, public
	// or unknown scope addresses are used.
	//
	// If a server has only machine-local addresses, or none
	// at all, then it will be excluded.
	server1 := network.NewAddresses("0.1.0.1", "0.1.0.2", "host.com")
	server1[0].Scope = network.ScopeCloudLocal
	server1[1].Scope = network.ScopeCloudLocal
	server1[2].Scope = network.ScopePublic

	server2 := network.NewAddresses("0.2.0.1", "0.2.0.2")
	server2[0].Scope = network.ScopePublic
	server2[1].Scope = network.ScopePublic

	server3 := network.NewAddresses("127.0.0.1")
	server3[0].Scope = network.ScopeMachineLocal

	server4 := network.NewAddresses("0.4.0.1", "elsewhere.net")
	server4[0].Scope = network.ScopeUnknown
	server4[1].Scope = network.ScopeUnknown

	conf.SetAPIHostPorts([][]network.HostPort{
		network.AddressesWithPort(server1, 1111),
		network.AddressesWithPort(server2, 2222),
		network.AddressesWithPort(server3, 3333),
		network.AddressesWithPort(server4, 4444),
	})
	addrs, err = conf.APIAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.DeepEquals, []string{
		"0.1.0.1:1111",
		"0.1.0.2:1111",
		"0.2.0.1:2222",
		"0.2.0.2:2222",
		"0.4.0.1:4444",
		"elsewhere.net:4444",
	})
}
