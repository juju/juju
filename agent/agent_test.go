// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/version"
)

type suite struct {
	testbase.LoggingSuite
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
		DataDir: "/data/dir",
	},
	checkErr: "entity tag not found in configuration",
}, {
	about: "missing upgraded to version",
	params: agent.AgentConfigParams{
		DataDir: "/data/dir",
		Tag:     "omg",
	},
	checkErr: "upgradedToVersion not found in configuration",
}, {
	about: "missing password",
	params: agent.AgentConfigParams{
		DataDir:           "/data/dir",
		Tag:               "omg",
		UpgradedToVersion: version.Current.Number,
	},
	checkErr: "password not found in configuration",
}, {
	about: "missing CA cert",
	params: agent.AgentConfigParams{
		DataDir:           "/data/dir",
		Tag:               "omg",
		UpgradedToVersion: version.Current.Number,
		Password:          "sekrit",
	},
	checkErr: "CA certificate not found in configuration",
}, {
	about: "need either state or api addresses",
	params: agent.AgentConfigParams{
		DataDir:           "/data/dir",
		Tag:               "omg",
		UpgradedToVersion: version.Current.Number,
		Password:          "sekrit",
		CACert:            []byte("ca cert"),
	},
	checkErr: "state or API addresses not found in configuration",
}, {
	about: "invalid state address",
	params: agent.AgentConfigParams{
		DataDir:           "/data/dir",
		Tag:               "omg",
		UpgradedToVersion: version.Current.Number,
		Password:          "sekrit",
		CACert:            []byte("ca cert"),
		StateAddresses:    []string{"localhost:8080", "bad-address"},
	},
	checkErr: `invalid state server address "bad-address"`,
}, {
	about: "invalid api address",
	params: agent.AgentConfigParams{
		DataDir:           "/data/dir",
		Tag:               "omg",
		UpgradedToVersion: version.Current.Number,
		Password:          "sekrit",
		CACert:            []byte("ca cert"),
		APIAddresses:      []string{"localhost:8080", "bad-address"},
	},
	checkErr: `invalid API server address "bad-address"`,
}, {
	about: "good state addresses",
	params: agent.AgentConfigParams{
		DataDir:           "/data/dir",
		Tag:               "omg",
		UpgradedToVersion: version.Current.Number,
		Password:          "sekrit",
		CACert:            []byte("ca cert"),
		StateAddresses:    []string{"localhost:1234"},
	},
}, {
	about: "good api addresses",
	params: agent.AgentConfigParams{
		DataDir:           "/data/dir",
		Tag:               "omg",
		UpgradedToVersion: version.Current.Number,
		Password:          "sekrit",
		CACert:            []byte("ca cert"),
		APIAddresses:      []string{"localhost:1234"},
	},
}, {
	about: "both state and api addresses",
	params: agent.AgentConfigParams{
		DataDir:           "/data/dir",
		Tag:               "omg",
		UpgradedToVersion: version.Current.Number,
		Password:          "sekrit",
		CACert:            []byte("ca cert"),
		StateAddresses:    []string{"localhost:1234"},
		APIAddresses:      []string{"localhost:1235"},
	},
}, {
	about: "everything...",
	params: agent.AgentConfigParams{
		DataDir:           "/data/dir",
		Tag:               "omg",
		Password:          "sekrit",
		UpgradedToVersion: version.Current.Number,
		CACert:            []byte("ca cert"),
		StateAddresses:    []string{"localhost:1234"},
		APIAddresses:      []string{"localhost:1235"},
		Nonce:             "a nonce",
	},
}, {
	about: "missing logDir sets default",
	params: agent.AgentConfigParams{
		DataDir:           "/data/dir",
		Tag:               "omg",
		Password:          "sekrit",
		UpgradedToVersion: version.Current.Number,
		CACert:            []byte("ca cert"),
		StateAddresses:    []string{"localhost:1234"},
		APIAddresses:      []string{"localhost:1235"},
		Nonce:             "a nonce",
	},
	inspectConfig: func(c *gc.C, cfg agent.Config) {
		c.Check(cfg.LogDir(), gc.Equals, agent.DefaultLogDir)
	},
}}

func (*suite) TestNewAgentConfig(c *gc.C) {

	for i, test := range agentConfigTests {
		c.Logf("%v: %s", i, test.about)
		_, err := agent.NewAgentConfig(test.params)
		if test.checkErr == "" {
			c.Assert(err, gc.IsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, test.checkErr)
		}
	}
}

func (*suite) TestNewStateMachineConfig(c *gc.C) {
	type testStruct struct {
		about         string
		params        agent.StateMachineConfigParams
		checkErr      string
		inspectConfig func(*gc.C, agent.Config)
	}
	var tests = []testStruct{{
		about:    "missing state server cert",
		checkErr: "state server cert not found in configuration",
	}, {
		about: "missing state server key",
		params: agent.StateMachineConfigParams{
			StateServerCert: []byte("server cert"),
		},
		checkErr: "state server key not found in configuration",
	}}

	for _, test := range agentConfigTests {
		tests = append(tests, testStruct{
			about: test.about,
			params: agent.StateMachineConfigParams{
				StateServerCert:   []byte("server cert"),
				StateServerKey:    []byte("server key"),
				AgentConfigParams: test.params,
			},
			checkErr: test.checkErr,
		})
	}

	for i, test := range tests {
		c.Logf("%v: %s", i, test.about)
		cfg, err := agent.NewStateMachineConfig(test.params)
		if test.checkErr == "" {
			c.Assert(err, gc.IsNil)
			if test.inspectConfig != nil {
				test.inspectConfig(c, cfg)
			}
		} else {
			c.Assert(err, gc.ErrorMatches, test.checkErr)
		}
	}
}

var attributeParams = agent.AgentConfigParams{
	DataDir:           "/data/dir",
	Tag:               "omg",
	UpgradedToVersion: version.Current.Number,
	Password:          "sekrit",
	CACert:            []byte("ca cert"),
	StateAddresses:    []string{"localhost:1234"},
	APIAddresses:      []string{"localhost:1235"},
	Nonce:             "a nonce",
}

func (*suite) TestAttributes(c *gc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, gc.IsNil)
	c.Assert(conf.DataDir(), gc.Equals, "/data/dir")
	c.Assert(conf.Tag(), gc.Equals, "omg")
	c.Assert(conf.Dir(), gc.Equals, "/data/dir/agents/omg")
	c.Assert(conf.Nonce(), gc.Equals, "a nonce")
	c.Assert(conf.UpgradedToVersion(), gc.DeepEquals, version.Current.Number)
}

func (s *suite) TestApiAddressesCantWriteBack(c *gc.C) {
	conf, err := agent.NewAgentConfig(attributeParams)
	c.Assert(err, gc.IsNil)
	value, err := conf.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(value, gc.DeepEquals, []string{"localhost:1235"})
	value[0] = "invalidAdr"
	//Check out change hasn't gone back into the internals
	newValue, err := conf.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(newValue, gc.DeepEquals, []string{"localhost:1235"})
}

func assertConfigEqual(c *gc.C, c1, c2 agent.Config) {
	// Since we can't directly poke the internals, we'll use the WriteCommands
	// method.
	conf1Commands, err := c1.WriteCommands()
	c.Assert(err, gc.IsNil)
	conf2Commands, err := c2.WriteCommands()
	c.Assert(err, gc.IsNil)
	c.Assert(conf1Commands, gc.DeepEquals, conf2Commands)
}

func (*suite) TestWriteAndRead(c *gc.C) {
	testParams := attributeParams
	testParams.DataDir = c.MkDir()
	testParams.LogDir = c.MkDir()
	conf, err := agent.NewAgentConfig(testParams)
	c.Assert(err, gc.IsNil)

	c.Assert(conf.Write(), gc.IsNil)
	reread, err := agent.ReadConf(agent.ConfigPath(conf.DataDir(), conf.Tag()))
	c.Assert(err, gc.IsNil)
	assertConfigEqual(c, conf, reread)
}

func (*suite) TestWriteNewPassword(c *gc.C) {

	for i, test := range []struct {
		about  string
		params agent.AgentConfigParams
	}{{
		about: "good state addresses",
		params: agent.AgentConfigParams{
			DataDir:           c.MkDir(),
			Tag:               "omg",
			UpgradedToVersion: version.Current.Number,
			Password:          "sekrit",
			CACert:            []byte("ca cert"),
			StateAddresses:    []string{"localhost:1234"},
		},
	}, {
		about: "good api addresses",
		params: agent.AgentConfigParams{
			DataDir:           c.MkDir(),
			Tag:               "omg",
			UpgradedToVersion: version.Current.Number,
			Password:          "sekrit",
			CACert:            []byte("ca cert"),
			APIAddresses:      []string{"localhost:1234"},
		},
	}, {
		about: "both state and api addresses",
		params: agent.AgentConfigParams{
			DataDir:           c.MkDir(),
			Tag:               "omg",
			UpgradedToVersion: version.Current.Number,
			Password:          "sekrit",
			CACert:            []byte("ca cert"),
			StateAddresses:    []string{"localhost:1234"},
			APIAddresses:      []string{"localhost:1235"},
		},
	}} {
		c.Logf("%v: %s", i, test.about)

		conf, err := agent.NewAgentConfig(test.params)
		c.Assert(err, gc.IsNil)
		newPass, err := agent.WriteNewPassword(conf)
		c.Assert(err, gc.IsNil)
		// Show that the password is saved.
		reread, err := agent.ReadConf(agent.ConfigPath(conf.DataDir(), conf.Tag()))
		c.Assert(agent.Password(conf), gc.Equals, agent.Password(reread))
		c.Assert(newPass, gc.Equals, agent.Password(conf))
	}
}

func (*suite) TestWriteUpgradedToVersion(c *gc.C) {
	testParams := attributeParams
	testParams.DataDir = c.MkDir()
	conf, err := agent.NewAgentConfig(testParams)
	c.Assert(err, gc.IsNil)
	c.Assert(conf.Write(), gc.IsNil)

	newVersion := version.Current.Number
	newVersion.Major++
	c.Assert(conf.WriteUpgradedToVersion(newVersion), gc.IsNil)
	c.Assert(conf.UpgradedToVersion(), gc.DeepEquals, newVersion)

	// Show that the upgradedToVersion is saved.
	reread, err := agent.ReadConf(agent.ConfigPath(conf.DataDir(), conf.Tag()))
	assertConfigEqual(c, conf, reread)
}

// Actual opening of state and api requires a lot more boiler plate to make
// sure they are valid connections.  This is done in the cmd/jujud tests for
// bootstrap, machine and unit tests.
