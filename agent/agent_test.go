// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	"path/filepath"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/testing"
)

type suite struct {
	testing.BaseSuite
}

func TestSuite(t *stdtesting.T) {
	tc.Run(t, &suite{})
}

var agentConfigTests = []struct {
	about         string
	params        agent.AgentConfigParams
	checkErr      string
	inspectConfig func(*tc.C, agent.Config)
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
	about: "good api addresses for controller agent",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewControllerAgentTag("0"),
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
	inspectConfig: func(c *tc.C, cfg agent.Config) {
		c.Check(cfg.LogDir(), tc.Equals, agent.DefaultPaths.LogDir)
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
	inspectConfig: func(c *tc.C, cfg agent.Config) {
		c.Check(cfg.MetricsSpoolDir(), tc.Equals, agent.DefaultPaths.MetricsSpoolDir)
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
	inspectConfig: func(c *tc.C, cfg agent.Config) {
		c.Check(cfg.MetricsSpoolDir(), tc.Equals, "/tmp/nowhere")
	},
}, {
	about: "agentConfig must not be a User tag",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               names.NewUserTag("admin"), // this is a joke, the admin user is nil.
		UpgradedToVersion: jujuversion.Current,
		Password:          "sekrit",
	},
	checkErr: "entity tag must be MachineTag, UnitTag, ApplicationTag or ControllerAgentTag, got names.UserTag",
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
	inspectConfig: func(c *tc.C, cfg agent.Config) {
		c.Check(cfg.Dir(), tc.Equals, "/data/dir/agents/unit-ubuntu-1")
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
	inspectConfig: func(c *tc.C, cfg agent.Config) {
		c.Check(cfg.Dir(), tc.Equals, "/data/dir/agents/application-ubuntu")
	},
}, {
	about: "agentConfig accepts an Model tag",
	params: agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: "/data/dir"},
		Tag:               testing.ModelTag,
		Password:          "sekrit",
		UpgradedToVersion: jujuversion.Current,
		Controller:        testing.ControllerTag,
		Model:             testing.ModelTag,
		CACert:            "ca cert",
		APIAddresses:      []string{"localhost:1235"},
	},
	inspectConfig: func(c *tc.C, cfg agent.Config) {
		c.Check(cfg.Dir(), tc.Equals, "/data/dir/agents/model-deadbeef-0bad-400d-8000-4b1d0d06f00d")
	},
}}

func (*suite) TestNewAgentConfig(c *tc.C) {
	for i, test := range agentConfigTests {
		c.Logf("%v: %s", i, test.about)
		config, err := agent.NewAgentConfig(test.params)
		if test.checkErr == "" {
			c.Assert(err, tc.ErrorIsNil)
			if test.inspectConfig != nil {
				test.inspectConfig(c, config)
			}
		} else {
			c.Assert(err, tc.ErrorMatches, test.checkErr)
		}
	}
}

func stateServingInfo() controller.StateServingInfo {
	return controller.StateServingInfo{
		Cert:           "cert",
		PrivateKey:     "key",
		CAPrivateKey:   "ca key",
		StatePort:      69,
		APIPort:        47,
		SharedSecret:   "shared",
		SystemIdentity: "identity",
	}
}

func (*suite) TestNewStateMachineConfig(c *tc.C) {
	type testStruct struct {
		about         string
		params        agent.AgentConfigParams
		servingInfo   controller.StateServingInfo
		checkErr      string
		inspectConfig func(*tc.C, agent.Config)
	}
	var tests = []testStruct{{
		about:    "missing controller cert",
		checkErr: "controller cert not found in configuration",
	}, {
		about: "missing controller key",
		servingInfo: controller.StateServingInfo{
			Cert: "server cert",
		},
		checkErr: "controller key not found in configuration",
	}, {
		about: "missing ca cert key",
		servingInfo: controller.StateServingInfo{
			Cert:       "server cert",
			PrivateKey: "server key",
		},
		checkErr: "ca cert key not found in configuration",
	}, {
		about: "missing state port",
		servingInfo: controller.StateServingInfo{
			Cert:         "server cert",
			PrivateKey:   "server key",
			CAPrivateKey: "ca key",
		},
		checkErr: "state port not found in configuration",
	}, {
		about: "params api port",
		servingInfo: controller.StateServingInfo{
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
			c.Assert(err, tc.ErrorIsNil)
			if test.inspectConfig != nil {
				test.inspectConfig(c, cfg)
			}
		} else {
			c.Assert(err, tc.ErrorMatches, test.checkErr)
		}
	}
}

var attributeParams = agent.AgentConfigParams{
	Paths: agent.Paths{
		DataDir: "/data/dir",
	},
	Tag:                    names.NewMachineTag("1"),
	UpgradedToVersion:      jujuversion.Current,
	Password:               "sekrit",
	CACert:                 "ca cert",
	APIAddresses:           []string{"localhost:1235"},
	Nonce:                  "a nonce",
	Controller:             testing.ControllerTag,
	Model:                  testing.ModelTag,
	JujuDBSnapChannel:      controller.DefaultJujuDBSnapChannel,
	AgentLogfileMaxSizeMB:  150,
	AgentLogfileMaxBackups: 4,
}

func (*suite) TestAttributes(c *tc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(conf.DataDir(), tc.Equals, "/data/dir")
	compareSystemIdentityPath := filepath.FromSlash("/data/dir/system-identity")
	systemIdentityPath := filepath.FromSlash(conf.SystemIdentityPath())
	c.Assert(systemIdentityPath, tc.Equals, compareSystemIdentityPath)
	c.Assert(conf.Tag(), tc.Equals, names.NewMachineTag("1"))
	c.Assert(conf.Dir(), tc.Equals, "/data/dir/agents/machine-1")
	c.Assert(conf.Nonce(), tc.Equals, "a nonce")
	c.Assert(conf.UpgradedToVersion(), tc.DeepEquals, jujuversion.Current)
	c.Assert(conf.JujuDBSnapChannel(), tc.Equals, "4.4/stable")
	c.Assert(conf.AgentLogfileMaxSizeMB(), tc.Equals, 150)
	c.Assert(conf.AgentLogfileMaxBackups(), tc.Equals, 4)
}

func (*suite) TestStateServingInfo(c *tc.C) {
	servingInfo := stateServingInfo()
	conf, err := agent.NewStateMachineConfig(attributeParams, servingInfo)
	c.Assert(err, tc.ErrorIsNil)
	gotInfo, ok := conf.StateServingInfo()
	c.Assert(ok, tc.IsTrue)
	c.Assert(gotInfo, tc.DeepEquals, servingInfo)
	newInfo := controller.StateServingInfo{
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
	c.Assert(ok, tc.IsTrue)
	c.Assert(gotInfo, tc.DeepEquals, newInfo)
}

func (*suite) TestStateServingInfoNotAvailable(c *tc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, tc.ErrorIsNil)

	_, available := conf.StateServingInfo()
	c.Assert(available, tc.IsFalse)
}

func (s *suite) TestAPIAddressesCannotWriteBack(c *tc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, tc.ErrorIsNil)
	value, err := conf.APIAddresses()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(value, tc.DeepEquals, []string{"localhost:1235"})
	value[0] = "invalidAdr"
	//Check out change hasn't gone back into the internals
	newValue, err := conf.APIAddresses()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newValue, tc.DeepEquals, []string{"localhost:1235"})
}

func (*suite) TestWriteAndRead(c *tc.C) {
	testParams := attributeParams
	testParams.Paths.DataDir = c.MkDir()
	testParams.Paths.LogDir = c.MkDir()
	conf, err := agent.NewAgentConfig(testParams)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(conf.Write(), tc.IsNil)
	reread, err := agent.ReadConfig(agent.ConfigPath(conf.DataDir(), conf.Tag()))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(reread, tc.DeepEquals, conf)
}

func (*suite) TestParseConfigData(c *tc.C) {
	testParams := attributeParams
	testParams.Paths.DataDir = c.MkDir()
	testParams.Paths.LogDir = c.MkDir()
	conf, err := agent.NewAgentConfig(testParams)
	c.Assert(err, tc.ErrorIsNil)
	data, err := conf.Render()
	c.Assert(err, tc.ErrorIsNil)
	reread, err := agent.ParseConfigData(data)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(reread, tc.DeepEquals, conf)
}

func (*suite) TestAPIInfoMissingAddress(c *tc.C) {
	conf := agent.EmptyConfig()
	_, ok := conf.APIInfo()
	c.Assert(ok, tc.IsFalse)
}

func (*suite) TestAPIInfoServesLocalhostWhenServingInfoPresent(c *tc.C) {
	attrParams := attributeParams
	attrParams.APIAddresses = []string{"foo.example:1235"}
	servingInfo := stateServingInfo()
	conf, err := agent.NewStateMachineConfig(attrParams, servingInfo)
	c.Assert(err, tc.ErrorIsNil)
	apiinfo, ok := conf.APIInfo()
	c.Assert(ok, tc.IsTrue)
	c.Check(apiinfo.Addrs, tc.SameContents, []string{"localhost:47", "foo.example:1235"})
}

func (*suite) TestMongoInfo(c *tc.C) {
	attrParams := attributeParams
	attrParams.APIAddresses = []string{"foo.example:1235", "bar.example:1236", "localhost:88", "3.4.2.1:1070"}
	servingInfo := stateServingInfo()
	conf, err := agent.NewStateMachineConfig(attrParams, servingInfo)
	c.Assert(err, tc.ErrorIsNil)
	mongoInfo, ok := conf.MongoInfo()
	c.Assert(ok, tc.IsTrue)
	c.Check(mongoInfo.Info.Addrs, tc.DeepEquals, []string{"localhost:69", "3.4.2.1:69"})
	c.Check(mongoInfo.Info.DisableTLS, tc.IsFalse)
}

func (*suite) TestMongoInfoNoCloudLocalAvailable(c *tc.C) {
	attrParams := attributeParams
	attrParams.APIAddresses = []string{"foo.example:1235", "bar.example:1236", "localhost:88"}
	servingInfo := stateServingInfo()
	conf, err := agent.NewStateMachineConfig(attrParams, servingInfo)
	c.Assert(err, tc.ErrorIsNil)
	mongoInfo, ok := conf.MongoInfo()
	c.Assert(ok, tc.IsTrue)
	c.Check(mongoInfo.Info.Addrs, tc.DeepEquals, []string{"localhost:69", "foo.example:69", "bar.example:69"})
	c.Check(mongoInfo.Info.DisableTLS, tc.IsFalse)
}

func (*suite) TestPromotedMongoInfo(c *tc.C) {
	attrParams := attributeParams
	attrParams.APIAddresses = []string{"foo.example:1235", "bar.example:1236", "localhost:88", "3.4.2.1:1070"}
	conf, err := agent.NewAgentConfig(attrParams)
	c.Assert(err, tc.ErrorIsNil)

	_, ok := conf.MongoInfo()
	c.Assert(ok, tc.IsFalse)

	// Promote the agent to a controller by
	// setting state serving info. As soon
	// as this is done, we should be able
	// to use MongoInfo.
	conf.SetStateServingInfo(stateServingInfo())

	mongoInfo, ok := conf.MongoInfo()
	c.Assert(ok, tc.IsTrue)
	c.Check(mongoInfo.Info.Addrs, tc.DeepEquals, []string{"localhost:69", "3.4.2.1:69"})
	c.Check(mongoInfo.Info.DisableTLS, tc.IsFalse)
}

func (*suite) TestAPIInfoDoesNotAddLocalhostWhenNoServingInfo(c *tc.C) {
	attrParams := attributeParams
	conf, err := agent.NewAgentConfig(attrParams)
	c.Assert(err, tc.ErrorIsNil)
	apiinfo, ok := conf.APIInfo()
	c.Assert(ok, tc.IsTrue)
	c.Assert(apiinfo.Addrs, tc.DeepEquals, attrParams.APIAddresses)
}

func (*suite) TestSetPassword(c *tc.C) {
	attrParams := attributeParams
	servingInfo := stateServingInfo()
	conf, err := agent.NewStateMachineConfig(attrParams, servingInfo)
	c.Assert(err, tc.ErrorIsNil)

	expectAPIInfo := &api.Info{
		Addrs:    append([]string{"localhost:47"}, attrParams.APIAddresses...),
		CACert:   attrParams.CACert,
		Tag:      attrParams.Tag,
		Password: "",
		Nonce:    attrParams.Nonce,
		ModelTag: attrParams.Model,
	}
	apiInfo, ok := conf.APIInfo()
	c.Assert(ok, tc.IsTrue)
	c.Assert(apiInfo, tc.DeepEquals, expectAPIInfo)

	conf.SetPassword("newpassword")

	expectAPIInfo.Password = "newpassword"

	apiInfo, ok = conf.APIInfo()
	c.Assert(ok, tc.IsTrue)
	c.Assert(apiInfo, tc.DeepEquals, expectAPIInfo)
}

func (*suite) TestSetOldPassword(c *tc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(conf.OldPassword(), tc.Equals, attributeParams.Password)
	conf.SetOldPassword("newoldpassword")
	c.Assert(conf.OldPassword(), tc.Equals, "newoldpassword")
}

func (*suite) TestSetUpgradedToVersion(c *tc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(conf.UpgradedToVersion(), tc.Equals, jujuversion.Current)

	expectVers := semversion.MustParse("3.4.5")
	conf.SetUpgradedToVersion(expectVers)
	c.Assert(conf.UpgradedToVersion(), tc.Equals, expectVers)
}

func (*suite) TestSetAPIHostPorts(c *tc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, tc.ErrorIsNil)

	addrs, err := conf.APIAddresses()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addrs, tc.DeepEquals, attributeParams.APIAddresses)

	// All the best candidate addresses for each server are
	// used. Cloud-local addresses are preferred.  Otherwise, public
	// or unknown scope addresses are used.
	//
	// If a server has only machine-local addresses, or none
	// at all, then it will be excluded.
	server1 := network.NewSpaceAddresses("0.1.0.1", "0.1.0.2", "host.com")
	server1[0].Scope = network.ScopeCloudLocal
	server1[1].Scope = network.ScopeCloudLocal
	server1[2].Scope = network.ScopePublic

	server2 := network.NewSpaceAddresses("0.2.0.1", "0.2.0.2")
	server2[0].Scope = network.ScopePublic
	server2[1].Scope = network.ScopePublic

	server3 := network.NewSpaceAddresses("127.0.0.1")
	server3[0].Scope = network.ScopeMachineLocal

	server4 := network.NewSpaceAddresses("0.4.0.1", "elsewhere.net")
	server4[0].Scope = network.ScopeUnknown
	server4[1].Scope = network.ScopeUnknown

	conf.SetAPIHostPorts([]network.HostPorts{
		network.SpaceAddressesWithPort(server1, 1111).HostPorts(),
		network.SpaceAddressesWithPort(server2, 2222).HostPorts(),
		network.SpaceAddressesWithPort(server3, 3333).HostPorts(),
		network.SpaceAddressesWithPort(server4, 4444).HostPorts(),
	})
	addrs, err = conf.APIAddresses()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addrs, tc.DeepEquals, []string{
		"0.1.0.1:1111",
		"0.1.0.2:1111",
		"host.com:1111",
		"0.2.0.1:2222",
		"0.2.0.2:2222",
		"0.4.0.1:4444",
		"elsewhere.net:4444",
	})
}

func (*suite) TestSetAPIHostPortsErrorOnEmpty(c *tc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, tc.ErrorIsNil)

	err = conf.SetAPIHostPorts([]network.HostPorts{})
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
}

func (*suite) TestSetCACert(c *tc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(conf.CACert(), tc.Equals, "ca cert")

	conf.SetCACert("new ca cert")
	c.Assert(conf.CACert(), tc.Equals, "new ca cert")
}

func (*suite) TestSetJujuDBSnapChannel(c *tc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, tc.ErrorIsNil)

	snapChannel := conf.JujuDBSnapChannel()
	c.Assert(snapChannel, tc.Equals, attributeParams.JujuDBSnapChannel)

	conf.SetJujuDBSnapChannel("latest/candidate")
	snapChannel = conf.JujuDBSnapChannel()
	c.Assert(snapChannel, tc.Equals, "latest/candidate", tc.Commentf("juju db snap channel setting not updated"))
}

func (*suite) TestSetQueryTracingEnabled(c *tc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, tc.ErrorIsNil)

	queryTracingEnabled := conf.QueryTracingEnabled()
	c.Assert(queryTracingEnabled, tc.Equals, attributeParams.QueryTracingEnabled)

	conf.SetQueryTracingEnabled(true)
	queryTracingEnabled = conf.QueryTracingEnabled()
	c.Assert(queryTracingEnabled, tc.Equals, true, tc.Commentf("query tracing enabled setting not updated"))
}

func (*suite) TestSetQueryTracingThreshold(c *tc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, tc.ErrorIsNil)

	queryTracingThreshold := conf.QueryTracingThreshold()
	c.Assert(queryTracingThreshold, tc.Equals, attributeParams.QueryTracingThreshold)

	conf.SetQueryTracingThreshold(time.Second * 10)
	queryTracingThreshold = conf.QueryTracingThreshold()
	c.Assert(queryTracingThreshold, tc.Equals, time.Second*10, tc.Commentf("query tracing threshold setting not updated"))
}

func (*suite) TestSetOpenTelemetryEnabled(c *tc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, tc.ErrorIsNil)

	queryTracingEnabled := conf.OpenTelemetryEnabled()
	c.Assert(queryTracingEnabled, tc.Equals, attributeParams.OpenTelemetryEnabled)

	conf.SetOpenTelemetryEnabled(true)
	queryTracingEnabled = conf.OpenTelemetryEnabled()
	c.Assert(queryTracingEnabled, tc.Equals, true, tc.Commentf("open telemetry enabled setting not updated"))
}

func (*suite) TestSetOpenTelemetryEndpoint(c *tc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, tc.ErrorIsNil)

	queryTracingEndpoint := conf.OpenTelemetryEndpoint()
	c.Assert(queryTracingEndpoint, tc.Equals, attributeParams.OpenTelemetryEndpoint)

	conf.SetOpenTelemetryEndpoint("http://foo.bar")
	queryTracingEndpoint = conf.OpenTelemetryEndpoint()
	c.Assert(queryTracingEndpoint, tc.Equals, "http://foo.bar", tc.Commentf("open telemetry endpoint setting not updated"))
}

func (*suite) TestSetOpenTelemetryInsecure(c *tc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, tc.ErrorIsNil)

	queryTracingInsecure := conf.OpenTelemetryInsecure()
	c.Assert(queryTracingInsecure, tc.Equals, attributeParams.OpenTelemetryInsecure)

	conf.SetOpenTelemetryInsecure(true)
	queryTracingInsecure = conf.OpenTelemetryInsecure()
	c.Assert(queryTracingInsecure, tc.Equals, true, tc.Commentf("open telemetry insecure setting not updated"))
}

func (*suite) TestSetOpenTelemetryStackTraces(c *tc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, tc.ErrorIsNil)

	queryTracingStackTraces := conf.OpenTelemetryStackTraces()
	c.Assert(queryTracingStackTraces, tc.Equals, attributeParams.OpenTelemetryStackTraces)

	conf.SetOpenTelemetryStackTraces(true)
	queryTracingStackTraces = conf.OpenTelemetryStackTraces()
	c.Assert(queryTracingStackTraces, tc.Equals, true, tc.Commentf("open telemetry stack traces setting not updated"))
}

func (*suite) TestSetOpenTelemetrySampleRatio(c *tc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, tc.ErrorIsNil)

	queryTracingSampleRatio := conf.OpenTelemetrySampleRatio()
	c.Assert(queryTracingSampleRatio, tc.Equals, attributeParams.OpenTelemetrySampleRatio)

	conf.SetOpenTelemetrySampleRatio(.42)
	queryTracingSampleRatio = conf.OpenTelemetrySampleRatio()
	c.Assert(queryTracingSampleRatio, tc.Equals, .42, tc.Commentf("open telemetry sample ratio setting not updated"))
}

func (*suite) TestSetOpenTelemetryTailSamplingThreshold(c *tc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, tc.ErrorIsNil)

	queryTracingTailSamplingThreshold := conf.OpenTelemetryTailSamplingThreshold()
	c.Assert(queryTracingTailSamplingThreshold, tc.Equals, attributeParams.OpenTelemetryTailSamplingThreshold)

	conf.SetOpenTelemetryTailSamplingThreshold(time.Second)
	queryTracingTailSamplingThreshold = conf.OpenTelemetryTailSamplingThreshold()
	c.Assert(queryTracingTailSamplingThreshold, tc.Equals, time.Second, tc.Commentf("open telemetry tail sampling threshold setting not updated"))
}

func (*suite) TestSetObjectStoreType(c *tc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, tc.ErrorIsNil)

	objectStoreType := conf.ObjectStoreType()
	c.Assert(objectStoreType, tc.Equals, attributeParams.ObjectStoreType)

	conf.SetObjectStoreType("s3")
	objectStoreType = conf.ObjectStoreType()
	c.Assert(objectStoreType, tc.Equals, objectstore.S3Backend, tc.Commentf("object store type setting not updated"))
}
