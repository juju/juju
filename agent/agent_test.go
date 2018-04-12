// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	"fmt"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
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
		UpgradedToVersion: jujuversion.Current,
	},
	checkErr: "password not found in configuration",
}, {
	about: "missing model tag",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		UpgradedToVersion: jujuversion.Current,
		Password:          "sekrit",
		Controller:        testing.ControllerTag,
	},
	checkErr: "model not found in configuration",
}, {
	about: "invalid model tag",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		UpgradedToVersion: jujuversion.Current,
		Password:          "sekrit",
		Controller:        testing.ControllerTag,
		Model:             names.NewModelTag("uuid"),
	},
	checkErr: `"uuid" is not a valid model uuid`,
}, {
	about: "missing controller tag",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		UpgradedToVersion: jujuversion.Current,
		Password:          "sekrit",
		Model:             testing.ModelTag,
	},
	checkErr: "controller not found in configuration",
}, {
	about: "invalid controller tag",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		UpgradedToVersion: jujuversion.Current,
		Password:          "sekrit",
		Controller:        names.NewControllerTag("uuid"),
		Model:             testing.ModelTag,
	},
	checkErr: `"uuid" is not a valid controller uuid`,
}, {
	about: "missing CA cert",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		UpgradedToVersion: jujuversion.Current,
		Password:          "sekrit",
		Controller:        testing.ControllerTag,
		Model:             testing.ModelTag,
	},
	checkErr: "CA certificate not found in configuration",
}, {
	about: "need api addresses",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		UpgradedToVersion: jujuversion.Current,
		Password:          "sekrit",
		CACert:            "ca cert",
		Controller:        testing.ControllerTag,
		Model:             testing.ModelTag,
	},
	checkErr: "API addresses not found in configuration",
}, {
	about: "invalid api address",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		UpgradedToVersion: jujuversion.Current,
		Password:          "sekrit",
		CACert:            "ca cert",
		Controller:        testing.ControllerTag,
		Model:             testing.ModelTag,
		APIAddresses:      []string{"localhost:8080", "bad-address"},
	},
	checkErr: `invalid API server address "bad-address"`,
}, {
	about: "good api addresses",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		UpgradedToVersion: jujuversion.Current,
		Password:          "sekrit",
		CACert:            "ca cert",
		Controller:        testing.ControllerTag,
		Model:             testing.ModelTag,
		APIAddresses:      []string{"localhost:1234"},
	},
}, {
	about: "everything...",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		Password:          "sekrit",
		UpgradedToVersion: jujuversion.Current,
		CACert:            "ca cert",
		Controller:        testing.ControllerTag,
		Model:             testing.ModelTag,
		APIAddresses:      []string{"localhost:1235"},
		Nonce:             "a nonce",
	},
}, {
	about: "missing logDir sets default",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewMachineTag("1"),
		Password:          "sekrit",
		UpgradedToVersion: jujuversion.Current,
		CACert:            "ca cert",
		Controller:        testing.ControllerTag,
		Model:             testing.ModelTag,
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
		UpgradedToVersion: jujuversion.Current,
		CACert:            "ca cert",
		Controller:        testing.ControllerTag,
		Model:             testing.ModelTag,
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
		UpgradedToVersion: jujuversion.Current,
		CACert:            "ca cert",
		Controller:        testing.ControllerTag,
		Model:             testing.ModelTag,
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
		UpgradedToVersion: jujuversion.Current,
		Password:          "sekrit",
	},
	checkErr: "entity tag must be MachineTag, UnitTag or ApplicationTag, got names.UserTag",
}, {
	about: "agentConfig accepts a Unit tag",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewUnitTag("ubuntu/1"),
		Password:          "sekrit",
		UpgradedToVersion: jujuversion.Current,
		Controller:        testing.ControllerTag,
		Model:             testing.ModelTag,
		CACert:            "ca cert",
		APIAddresses:      []string{"localhost:1235"},
	},
	inspectConfig: func(c *gc.C, cfg agent.Config) {
		c.Check(cfg.Dir(), gc.Equals, "/data/dir/agents/unit-ubuntu-1")
	},
}, {
	about: "agentConfig accepts an Application tag",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewApplicationTag("ubuntu"),
		Password:          "sekrit",
		UpgradedToVersion: jujuversion.Current,
		Controller:        testing.ControllerTag,
		Model:             testing.ModelTag,
		CACert:            "ca cert",
		APIAddresses:      []string{"localhost:1235"},
	},
	inspectConfig: func(c *gc.C, cfg agent.Config) {
		c.Check(cfg.Dir(), gc.Equals, "/data/dir/agents/application-ubuntu")
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
	UpgradedToVersion: jujuversion.Current,
	Password:          "sekrit",
	CACert:            "ca cert",
	APIAddresses:      []string{"localhost:1235"},
	Nonce:             "a nonce",
	Controller:        testing.ControllerTag,
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
	c.Assert(conf.UpgradedToVersion(), jc.DeepEquals, jujuversion.Current)
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

func (*suite) TestAPIInfoServesLocalhostOnlyWhenServingInfoPresent(c *gc.C) {
	attrParams := attributeParams
	attrParams.APIAddresses = []string{"localhost:1235", "localhost:1236"}
	servingInfo := stateServingInfo()
	conf, err := agent.NewStateMachineConfig(attrParams, servingInfo)
	c.Assert(err, jc.ErrorIsNil)
	apiinfo, ok := conf.APIInfo()
	c.Assert(ok, jc.IsTrue)
	c.Check(apiinfo.Addrs, gc.DeepEquals, []string{"localhost:47"})
}

func (*suite) TestMongoInfo(c *gc.C) {
	attrParams := attributeParams
	attrParams.APIAddresses = []string{"foo.example:1235", "bar.example:1236", "localhost:88"}
	servingInfo := stateServingInfo()
	conf, err := agent.NewStateMachineConfig(attrParams, servingInfo)
	c.Assert(err, jc.ErrorIsNil)
	mongoInfo, ok := conf.MongoInfo()
	c.Assert(ok, jc.IsTrue)
	c.Check(mongoInfo.Info.Addrs, jc.DeepEquals, []string{"localhost:69", "foo.example:69", "bar.example:69"})
	c.Check(mongoInfo.Info.DisableTLS, jc.IsFalse)
}

func (*suite) TestPromotedMongoInfo(c *gc.C) {
	attrParams := attributeParams
	attrParams.APIAddresses = []string{"foo.example:1235", "bar.example:1236", "localhost:88"}
	conf, err := agent.NewAgentConfig(attrParams)
	c.Assert(err, jc.ErrorIsNil)

	mongoInfo, ok := conf.MongoInfo()
	c.Assert(ok, jc.IsFalse)

	// Promote the agent to a controller by
	// setting state serving info. As soon
	// as this is done, we should be able
	// to use MongoInfo.
	conf.SetStateServingInfo(stateServingInfo())

	mongoInfo, ok = conf.MongoInfo()
	c.Assert(ok, jc.IsTrue)
	c.Check(mongoInfo.Info.Addrs, jc.DeepEquals, []string{"localhost:69", "foo.example:69", "bar.example:69"})
	c.Check(mongoInfo.Info.DisableTLS, jc.IsFalse)
}

func (*suite) TestAPIInfoDoesntAddLocalhostWhenNoServingInfo(c *gc.C) {
	attrParams := attributeParams
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
	addr := fmt.Sprintf("localhost:%d", servingInfo.StatePort)
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

	c.Assert(conf.UpgradedToVersion(), gc.Equals, jujuversion.Current)

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
		"host.com:1111",
		"0.2.0.1:2222",
		"0.2.0.2:2222",
		"0.4.0.1:4444",
		"elsewhere.net:4444",
	})
}

func (*suite) TestSetCACert(c *gc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conf.CACert(), gc.Equals, "ca cert")

	conf.SetCACert("new ca cert")
	c.Assert(conf.CACert(), gc.Equals, "new ca cert")
}
