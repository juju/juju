// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	"os"
	"os/exec"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
)

type suite struct {
	coretesting.LoggingSuite
}

var _ = gc.Suite(suite{})

var confTests = []struct {
	about    string
	conf     agent.Conf
	checkErr string
}{{
	about: "state info only",
	conf: agent.Conf{
		OldPassword:  "old password",
		MachineNonce: "dummy",
		StateInfo: &state.Info{
			Addrs:    []string{"foo.com:355", "bar:545"},
			CACert:   []byte("ca cert"),
			Tag:      "entity",
			Password: "current password",
		},
	},
}, {
	about: "state info and api info",
	conf: agent.Conf{
		StateServerCert: []byte("server cert"),
		StateServerKey:  []byte("server key"),
		StatePort:       1234,
		APIPort:         4321,
		OldPassword:     "old password",
		StateInfo: &state.Info{
			Addrs:    []string{"foo.com:355", "bar:545"},
			CACert:   []byte("ca cert"),
			Tag:      "entity",
			Password: "current password",
		},
		APIInfo: &api.Info{
			Tag:      "entity",
			Password: "other password",
			Addrs:    []string{"foo.com:555", "bar:555"},
			CACert:   []byte("api ca cert"),
		},
	},
}, {
	about: "API info and state info sharing CA cert",
	conf: agent.Conf{
		OldPassword: "old password",
		StateInfo: &state.Info{
			Addrs:    []string{"foo.com:355", "bar:545"},
			CACert:   []byte("ca cert"),
			Tag:      "entity",
			Password: "current password",
		},
		APIInfo: &api.Info{
			Tag:    "entity",
			Addrs:  []string{"foo.com:555"},
			CACert: []byte("ca cert"),
		},
	},
}, {
	about: "no api entity tag",
	conf: agent.Conf{
		StateServerCert: []byte("server cert"),
		StateServerKey:  []byte("server key"),
		OldPassword:     "old password",
		StateInfo: &state.Info{
			Addrs:    []string{"foo.com:355", "bar:545"},
			CACert:   []byte("ca cert"),
			Tag:      "entity",
			Password: "current password",
		},
		APIInfo: &api.Info{
			Addrs:  []string{"foo.com:555"},
			CACert: []byte("api ca cert"),
		},
	},
	checkErr: "API entity tag not found in configuration",
}, {
	about: "mismatched entity tags",
	conf: agent.Conf{
		StateServerCert: []byte("server cert"),
		StateServerKey:  []byte("server key"),
		OldPassword:     "old password",
		StateInfo: &state.Info{
			Addrs:    []string{"foo.com:355", "bar:545"},
			CACert:   []byte("ca cert"),
			Tag:      "entity",
			Password: "current password",
		},
		APIInfo: &api.Info{
			Tag:    "other",
			Addrs:  []string{"foo.com:555"},
			CACert: []byte("api ca cert"),
		},
	},
	checkErr: "mismatched entity tags",
}, {
	about: "no state entity tag",
	conf: agent.Conf{
		OldPassword: "old password",
		StateInfo: &state.Info{
			Addrs:    []string{"foo.com:355", "bar:545"},
			CACert:   []byte("ca cert"),
			Password: "current password",
		},
	},
	checkErr: "state entity tag not found in configuration",
}, {
	about: "no state server address",
	conf: agent.Conf{
		OldPassword: "old password",
		StateInfo: &state.Info{
			CACert:   []byte("ca cert"),
			Password: "current password",
			Tag:      "entity",
		},
	},
	checkErr: "state server address not found in configuration",
}, {
	about: "state server address with no port",
	conf: agent.Conf{
		OldPassword: "old password",
		StateInfo: &state.Info{
			Addrs:    []string{"foo"},
			CACert:   []byte("ca cert"),
			Tag:      "entity",
			Password: "current password",
		},
	},
	checkErr: "invalid state server address \"foo\"",
}, {
	about: "state server address with non-numeric port",
	conf: agent.Conf{
		OldPassword: "old password",
		StateInfo: &state.Info{
			Addrs:    []string{"foo:bar"},
			CACert:   []byte("ca cert"),
			Tag:      "entity",
			Password: "current password",
		},
	},
	checkErr: "invalid state server address \"foo:bar\"",
}, {
	about: "state server address with bad port",
	conf: agent.Conf{
		OldPassword: "old password",
		StateInfo: &state.Info{
			Addrs:    []string{"foo:345d"},
			CACert:   []byte("ca cert"),
			Tag:      "entity",
			Password: "current password",
		},
	},
	checkErr: "invalid state server address \"foo:345d\"",
}, {
	about: "invalid api server address",
	conf: agent.Conf{
		OldPassword: "old password",
		StateInfo: &state.Info{
			Addrs:    []string{"foo:345"},
			CACert:   []byte("ca cert"),
			Tag:      "entity",
			Password: "current password",
		},
		APIInfo: &api.Info{
			Tag:    "entity",
			Addrs:  []string{"bar.com:455", "foo"},
			CACert: []byte("ca cert"),
		},
	},
	checkErr: "invalid API server address \"foo\"",
}, {
	about: "no api CA cert",
	conf: agent.Conf{
		OldPassword: "old password",
		StateInfo: &state.Info{
			Addrs:    []string{"foo:345"},
			CACert:   []byte("ca cert"),
			Tag:      "entity",
			Password: "current password",
		},
		APIInfo: &api.Info{
			Tag:    "entity",
			Addrs:  []string{"foo:3"},
			CACert: []byte{},
		},
	},
	checkErr: "API CA certficate not found in configuration",
}, {
	about: "no state or API info",
	conf: agent.Conf{
		OldPassword: "old password",
	},
	checkErr: "state info or API info not found in configuration",
}}

func (suite) TestConfReadWriteCheck(c *gc.C) {
	d := c.MkDir()
	dataDir := filepath.Join(d, "data")
	for i, test := range confTests {
		c.Logf("test %d; %s", i, test.about)
		conf := test.conf
		conf.DataDir = dataDir
		err := conf.Check()
		if test.checkErr != "" {
			c.Assert(err, gc.ErrorMatches, test.checkErr)
			c.Assert(conf.Write(), gc.ErrorMatches, test.checkErr)
			cmds, err := conf.WriteCommands()
			c.Assert(cmds, gc.IsNil)
			c.Assert(err, gc.ErrorMatches, test.checkErr)
			continue
		}
		c.Assert(err, gc.IsNil)
		err = os.Mkdir(dataDir, 0777)
		c.Assert(err, gc.IsNil)
		err = conf.Write()
		c.Assert(err, gc.IsNil)
		info, err := os.Stat(conf.File("agent.conf"))
		c.Assert(err, gc.IsNil)
		c.Assert(info.Mode()&os.ModePerm, gc.Equals, os.FileMode(0600))

		// Move the configuration file to a different directory
		// to check that the entity name gets set correctly when
		// reading.
		newDir := filepath.Join(dataDir, "agents", "another")
		err = os.Mkdir(newDir, 0777)
		c.Assert(err, gc.IsNil)
		err = os.Rename(conf.File("agent.conf"), filepath.Join(newDir, "agent.conf"))
		c.Assert(err, gc.IsNil)

		rconf, err := agent.ReadConf(dataDir, "another")
		c.Assert(err, gc.IsNil)
		c.Assert(rconf.StateInfo.Tag, gc.Equals, "another")
		if rconf.StateInfo != nil {
			rconf.StateInfo.Tag = conf.Tag()
		}
		if rconf.APIInfo != nil {
			rconf.APIInfo.Tag = conf.Tag()
		}
		c.Assert(rconf, gc.DeepEquals, &conf)

		err = os.RemoveAll(dataDir)
		c.Assert(err, gc.IsNil)

		// Try the equivalent shell commands.
		cmds, err := conf.WriteCommands()
		c.Assert(err, gc.IsNil)
		for _, cmd := range cmds {
			out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
			c.Assert(err, gc.IsNil, gc.Commentf("command %q; output %q", cmd, out))
		}
		info, err = os.Stat(conf.File("agent.conf"))
		c.Assert(err, gc.IsNil)
		c.Assert(info.Mode()&os.ModePerm, gc.Equals, os.FileMode(0600))

		rconf, err = agent.ReadConf(dataDir, conf.StateInfo.Tag)
		c.Assert(err, gc.IsNil)

		c.Assert(rconf, gc.DeepEquals, &conf)

		err = os.RemoveAll(dataDir)
		c.Assert(err, gc.IsNil)
	}
}

func (suite) TestCheckNoDataDir(c *gc.C) {
	conf := agent.Conf{
		StateInfo: &state.Info{
			Addrs:    []string{"x:4"},
			CACert:   []byte("xxx"),
			Tag:      "bar",
			Password: "pass",
		},
	}
	c.Assert(conf.Check(), gc.ErrorMatches, "data directory not found in configuration")
}

func (suite) TestConfDir(c *gc.C) {
	conf := agent.Conf{
		DataDir: "/foo",
		StateInfo: &state.Info{
			Addrs:    []string{"x:4"},
			CACert:   []byte("xxx"),
			Tag:      "bar",
			Password: "pass",
		},
	}
	c.Assert(conf.Dir(), gc.Equals, "/foo/agents/bar")
}

func (suite) TestConfFile(c *gc.C) {
	conf := agent.Conf{
		DataDir: "/foo",
		StateInfo: &state.Info{
			Addrs:    []string{"x:4"},
			CACert:   []byte("xxx"),
			Tag:      "bar",
			Password: "pass",
		},
	}
	c.Assert(conf.File("x/y"), gc.Equals, "/foo/agents/bar/x/y")
}

type openSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&openSuite{})

func (s *openSuite) TestOpenStateNormal(c *gc.C) {
	conf := agent.Conf{
		StateInfo: s.StateInfo(c),
	}
	conf.OldPassword = "irrelevant"
	st, err := conf.OpenState()
	c.Assert(err, gc.IsNil)
	st.Close()
}

func (s *openSuite) TestOpenStateFallbackPassword(c *gc.C) {
	conf := agent.Conf{
		StateInfo: s.StateInfo(c),
	}
	conf.OldPassword = conf.StateInfo.Password
	conf.StateInfo.Password = "not the right password"

	st, err := conf.OpenState()
	c.Assert(err, gc.IsNil)
	c.Assert(st, gc.NotNil)
	st.Close()
}

func (s *openSuite) TestOpenStateNoPassword(c *gc.C) {
	conf := agent.Conf{
		StateInfo: s.StateInfo(c),
	}
	conf.OldPassword = conf.StateInfo.Password
	conf.StateInfo.Password = ""

	st, err := conf.OpenState()
	c.Assert(err, gc.IsNil)
	c.Assert(st, gc.NotNil)
	st.Close()
}

func (s *openSuite) TestOpenAPINormal(c *gc.C) {
	conf := agent.Conf{
		APIInfo: s.APIInfo(c),
	}
	conf.OldPassword = "irrelevant"

	st, newPassword, err := conf.OpenAPI(api.DialOpts{})
	c.Assert(err, gc.IsNil)
	defer st.Close()
	c.Assert(newPassword, gc.Equals, "")
	c.Assert(st, gc.NotNil)
}

func (s *openSuite) TestOpenAPIFallbackPassword(c *gc.C) {
	conf := agent.Conf{
		APIInfo: s.APIInfo(c),
	}
	conf.OldPassword = conf.APIInfo.Password
	conf.APIInfo.Password = "not the right password"

	st, newPassword, err := conf.OpenAPI(api.DialOpts{})
	c.Assert(err, gc.IsNil)
	defer st.Close()
	c.Assert(newPassword, gc.Matches, ".+")
	c.Assert(st, gc.NotNil)
	p, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	c.Assert(newPassword, gc.HasLen, len(p))
	c.Assert(conf.OldPassword, gc.Equals, s.APIInfo(c).Password)
}

func (s *openSuite) TestOpenAPINoPassword(c *gc.C) {
	conf := agent.Conf{
		APIInfo: s.APIInfo(c),
	}
	conf.OldPassword = conf.APIInfo.Password
	conf.APIInfo.Password = ""

	st, newPassword, err := conf.OpenAPI(api.DialOpts{})
	c.Assert(err, gc.IsNil)
	defer st.Close()
	c.Assert(newPassword, gc.Matches, ".+")
	c.Assert(st, gc.NotNil)
	p, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	c.Assert(newPassword, gc.HasLen, len(p))
	c.Assert(conf.OldPassword, gc.Equals, s.APIInfo(c).Password)
}
