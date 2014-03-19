// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	"reflect"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/state/api/params"
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

func (*suite) TestMigrateConfig(c *gc.C) {
	initialParams := agent.AgentConfigParams{
		DataDir:           c.MkDir(),
		LogDir:            c.MkDir(),
		Tag:               "omg",
		Nonce:             "nonce",
		Password:          "secret",
		UpgradedToVersion: version.MustParse("1.16.5"),
		Jobs: []params.MachineJob{
			params.JobManageEnviron,
			params.JobHostUnits,
		},
		CACert:         []byte("ca cert"),
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
		newParams    agent.MigrateConfigParams
		expectValues map[string]string
		expectErr    string
	}{{
		comment:   "nothing to change",
		fields:    nil,
		newParams: agent.MigrateConfigParams{},
	}, {
		fields: []string{"DataDir"},
		newParams: agent.MigrateConfigParams{
			DataDir: c.MkDir(),
		},
	}, {
		fields: []string{"DataDir", "LogDir"},
		newParams: agent.MigrateConfigParams{
			DataDir: c.MkDir(),
			LogDir:  c.MkDir(),
		},
	}, {
		fields: []string{"Jobs"},
		newParams: agent.MigrateConfigParams{
			Jobs: []params.MachineJob{params.JobHostUnits},
		},
	}, {
		comment:   "invalid/immutable field specified",
		fields:    []string{"InvalidField"},
		newParams: agent.MigrateConfigParams{},
		expectErr: `unknown field "InvalidField"`,
	}, {
		comment: "Values can be added, changed or removed",
		fields:  []string{"Values", "DeleteValues"},
		newParams: agent.MigrateConfigParams{
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
		c.Check(err, gc.IsNil)

		newConfig, err := agent.NewAgentConfig(initialParams)
		c.Check(err, gc.IsNil)

		c.Check(initialConfig.Write(), gc.IsNil)
		c.Check(agent.ConfigFileExists(initialConfig), jc.IsTrue)

		err = agent.MigrateConfig(newConfig, test.newParams)
		c.Check(err, gc.IsNil)
		c.Check(agent.ConfigFileExists(newConfig), jc.IsTrue)
		if test.newParams.DataDir != "" {
			// If we're changing where the new config is saved,
			// we can verify the old config got removed.
			c.Check(agent.ConfigFileExists(initialConfig), jc.IsFalse)
		}

		// Make sure we can read it back successfully and it
		// matches what we wrote.
		configPath := agent.ConfigPath(newConfig.DataDir(), newConfig.Tag())
		readConfig, err := agent.ReadConf(configPath)
		c.Check(err, gc.IsNil)
		c.Check(newConfig, jc.DeepEquals, readConfig)

		// Make sure only the specified fields were changed and
		// the rest matches.
		for _, field := range test.fields {
			switch field {
			case "Values":
				err = agent.PatchConfig(initialConfig, field, test.expectValues)
				c.Check(err, gc.IsNil)
			case "DeleteValues":
				err = agent.PatchConfig(initialConfig, field, test.newParams.DeleteValues)
				c.Check(err, gc.IsNil)
			default:
				value := reflect.ValueOf(test.newParams).FieldByName(field)
				if value.IsValid() && test.expectErr == "" {
					err = agent.PatchConfig(initialConfig, field, value.Interface())
					c.Check(err, gc.IsNil)
				} else {
					err = agent.PatchConfig(initialConfig, field, value)
					c.Check(err, gc.ErrorMatches, test.expectErr)
				}
			}
		}
		c.Check(newConfig, jc.DeepEquals, initialConfig)
	}
}

func (*suite) TestNewStateMachineConfig(c *gc.C) {
	type testStruct struct {
		about         string
		params        agent.AgentConfigParams
		checkErr      string
		inspectConfig func(*gc.C, agent.Config)
	}
	var tests = []testStruct{{
		about:    "missing state server cert",
		checkErr: "state server cert not found in configuration",
	}, {
		about: "missing state server key",
		params: agent.AgentConfigParams{
			StateServerCert: []byte("server cert"),
		},
		checkErr: "state server key not found in configuration",
	}}

	for _, test := range agentConfigTests {
		p := test.params
		p.StateServerCert = []byte("server cert")
		p.StateServerKey = []byte("server key")

		tests = append(tests, testStruct{
			about:    test.about,
			params:   p,
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

func (*suite) TestWriteAndRead(c *gc.C) {
	testParams := attributeParams
	testParams.DataDir = c.MkDir()
	testParams.LogDir = c.MkDir()
	conf, err := agent.NewAgentConfig(testParams)
	c.Assert(err, gc.IsNil)

	c.Assert(conf.Write(), gc.IsNil)
	reread, err := agent.ReadConf(agent.ConfigPath(conf.DataDir(), conf.Tag()))
	c.Assert(err, gc.IsNil)
	c.Assert(reread, jc.DeepEquals, conf)
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
	c.Assert(reread, jc.DeepEquals, conf)
}

// Actual opening of state and api requires a lot more boiler plate to make
// sure they are valid connections.  This is done in the cmd/jujud tests for
// bootstrap, machine and unit tests.
