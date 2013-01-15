package agent_test

import (
	"errors"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/agent"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/trivial"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	stdtesting "testing"
	"time"
)

type suite struct{}

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

var _ = Suite(suite{})

var confTests = []struct {
	about    string
	conf     agent.Conf
	checkErr string
}{{
	about: "state info only",
	conf: agent.Conf{
		OldPassword: "old password",
		StateInfo: state.Info{
			Addrs:      []string{"foo.com:355", "bar:545"},
			CACert:     []byte("ca cert"),
			EntityName: "entity",
			Password:   "current password",
		},
	},
}, {
	about: "state info and api info",
	conf: agent.Conf{
		OldPassword: "old password",
		StateInfo: state.Info{
			Addrs:      []string{"foo.com:355", "bar:545"},
			CACert:     []byte("ca cert"),
			EntityName: "entity",
			Password:   "current password",
		},
		APIInfo: api.Info{
			Addr:   "foo.com:555",
			CACert: []byte("api ca cert"),
		},
	},
}, {
	about: "API info and state info sharing CA cert",
	conf: agent.Conf{
		OldPassword: "old password",
		StateInfo: state.Info{
			Addrs:      []string{"foo.com:355", "bar:545"},
			CACert:     []byte("ca cert"),
			EntityName: "entity",
			Password:   "current password",
		},
		APIInfo: api.Info{
			Addr:   "foo.com:555",
			CACert: []byte("ca cert"),
		},
	},
}, {
	about: "no state entity name",
	conf: agent.Conf{
		OldPassword: "old password",
		StateInfo: state.Info{
			Addrs:    []string{"foo.com:355", "bar:545"},
			CACert:   []byte("ca cert"),
			Password: "current password",
		},
	},
	checkErr: "state entity name not found in configuration",
}, {
	about: "no state server address",
	conf: agent.Conf{
		OldPassword: "old password",
		StateInfo: state.Info{
			CACert:     []byte("ca cert"),
			Password:   "current password",
			EntityName: "entity",
		},
	},
	checkErr: "state server address not found in configuration",
}, {
	about: "state server address with no port",
	conf: agent.Conf{
		OldPassword: "old password",
		StateInfo: state.Info{
			Addrs:      []string{"foo"},
			CACert:     []byte("ca cert"),
			EntityName: "entity",
			Password:   "current password",
		},
	},
	checkErr: "invalid state server address \"foo\"",
}, {
	about: "state server address with non-numeric port",
	conf: agent.Conf{
		OldPassword: "old password",
		StateInfo: state.Info{
			Addrs:      []string{"foo:bar"},
			CACert:     []byte("ca cert"),
			EntityName: "entity",
			Password:   "current password",
		},
	},
	checkErr: "invalid state server address \"foo:bar\"",
}, {
	about: "state server address with bad port",
	conf: agent.Conf{
		OldPassword: "old password",
		StateInfo: state.Info{
			Addrs:      []string{"foo:345d"},
			CACert:     []byte("ca cert"),
			EntityName: "entity",
			Password:   "current password",
		},
	},
	checkErr: "invalid state server address \"foo:345d\"",
}, {
	about: "invalid api server address",
	conf: agent.Conf{
		OldPassword: "old password",
		StateInfo: state.Info{
			Addrs:      []string{"foo:345"},
			CACert:     []byte("ca cert"),
			EntityName: "entity",
			Password:   "current password",
		},
		APIInfo: api.Info{
			Addr:   "foo",
			CACert: []byte("ca cert"),
		},
	},
	checkErr: "invalid API server address \"foo\"",
}, {
	about: "no api CA cert",
	conf: agent.Conf{
		OldPassword: "old password",
		StateInfo: state.Info{
			Addrs:      []string{"foo:345"},
			CACert:     []byte("ca cert"),
			EntityName: "entity",
			Password:   "current password",
		},
		APIInfo: api.Info{
			Addr:   "foo:3",
			CACert: []byte{},
		},
	},
	checkErr: "API CA certficate not found in configuration",
},
}

func (suite) TestConfReadWriteCheck(c *C) {
	d := c.MkDir()
	dataDir := filepath.Join(d, "data")
	for i, test := range confTests {
		c.Logf("test %d; %s", i, test.about)
		conf := test.conf
		conf.DataDir = dataDir
		err := conf.Check()
		if test.checkErr != "" {
			c.Assert(err, ErrorMatches, test.checkErr)
			c.Assert(conf.Write(), ErrorMatches, test.checkErr)
			cmds, err := conf.WriteCommands()
			c.Assert(cmds, IsNil)
			c.Assert(err, ErrorMatches, test.checkErr)
			continue
		}
		c.Assert(err, IsNil)
		err = os.Mkdir(dataDir, 0777)
		c.Assert(err, IsNil)
		err = conf.Write()
		c.Assert(err, IsNil)
		info, err := os.Stat(conf.File("agent.conf"))
		c.Assert(err, IsNil)
		c.Assert(info.Mode()&os.ModePerm, Equals, os.FileMode(0600))

		// Move the configuration file to a different directory
		// to check that the entity name gets set correctly when
		// reading.
		newDir := filepath.Join(dataDir, "agents", "another")
		err = os.Mkdir(newDir, 0777)
		c.Assert(err, IsNil)
		err = os.Rename(conf.File("agent.conf"), filepath.Join(newDir, "agent.conf"))
		c.Assert(err, IsNil)

		rconf, err := agent.ReadConf(dataDir, "another")
		c.Assert(err, IsNil)
		c.Assert(rconf.StateInfo.EntityName, Equals, "another")
		rconf.StateInfo.EntityName = conf.StateInfo.EntityName
		c.Assert(rconf, DeepEquals, &conf)

		err = os.RemoveAll(dataDir)
		c.Assert(err, IsNil)

		// Try the equivalent shell commands.
		cmds, err := conf.WriteCommands()
		c.Assert(err, IsNil)
		for _, cmd := range cmds {
			out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
			c.Assert(err, IsNil, Commentf("command %q; output %q", cmd, out))
		}
		info, err = os.Stat(conf.File("agent.conf"))
		c.Assert(err, IsNil)
		c.Assert(info.Mode()&os.ModePerm, Equals, os.FileMode(0600))

		rconf, err = agent.ReadConf(dataDir, conf.StateInfo.EntityName)
		c.Assert(err, IsNil)

		c.Assert(rconf, DeepEquals, &conf)

		err = os.RemoveAll(dataDir)
		c.Assert(err, IsNil)
	}
}

func (suite) TestCheckNoDataDir(c *C) {
	conf := agent.Conf{
		StateInfo: state.Info{
			Addrs:      []string{"x:4"},
			CACert:     []byte("xxx"),
			EntityName: "bar",
			Password:   "pass",
		},
	}
	c.Assert(conf.Check(), ErrorMatches, "data directory not found in configuration")
}

func (suite) TestConfDir(c *C) {
	conf := agent.Conf{
		DataDir: "/foo",
		StateInfo: state.Info{
			Addrs:      []string{"x:4"},
			CACert:     []byte("xxx"),
			EntityName: "bar",
			Password:   "pass",
		},
	}
	c.Assert(conf.Dir(), Equals, "/foo/agents/bar")
}

func (suite) TestConfFile(c *C) {
	conf := agent.Conf{
		DataDir: "/foo",
		StateInfo: state.Info{
			Addrs:      []string{"x:4"},
			CACert:     []byte("xxx"),
			EntityName: "bar",
			Password:   "pass",
		},
	}
	c.Assert(conf.File("x/y"), Equals, "/foo/agents/bar/x/y")
}

func (suite) TestChange(c *C) {
	dataDir := c.MkDir()
	conf := agent.Conf{
		DataDir:     dataDir,
		OldPassword: "old",
		StateInfo: state.Info{
			Addrs:      []string{"x:4"},
			CACert:     []byte("xxx"),
			EntityName: "bar",
			Password:   "pass",
		},
	}
	err := conf.Write()
	c.Assert(err, IsNil)

	conf2 := agent.Conf{
		DataDir: dataDir,
		StateInfo: state.Info{
			Addrs:      []string{"y:8"},
			CACert:     []byte("yyy"),
			EntityName: "bar",
			Password:   "again",
		},
	}

	mutate1 := func(c *agent.Conf) error {
		c.StateInfo.Password += "-1"
		time.Sleep(10 * time.Millisecond)
		return nil
	}
	mutate2 := func(c *agent.Conf) error {
		c.OldPassword += "-2"
		time.Sleep(10 * time.Millisecond)
		return nil
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, f := range []func(*agent.Conf) error{mutate1, mutate1, mutate2, mutate2} {
		f := f
		wg.Add(1)
		go func() {
			<-start
			err := conf2.Change(f)
			c.Check(err, IsNil)
			wg.Done()
		}()
	}
	close(start)
	wg.Wait()
	c.Assert(conf2, DeepEquals, agent.Conf{
		DataDir:     dataDir,
		OldPassword: "old-2-2",
		StateInfo: state.Info{
			Addrs:      []string{"x:4"},
			CACert:     []byte("xxx"),
			EntityName: "bar",
			Password:   "pass-1-1",
		},
	})
}

func (suite) TestChangeError(c *C) {
	dataDir := c.MkDir()
	conf := agent.Conf{
		DataDir: dataDir,
		StateInfo: state.Info{
			Addrs:      []string{"x:4"},
			CACert:     []byte("xxx"),
			EntityName: "bar",
			Password:   "pass",
		},
	}
	err := conf.Write()
	c.Assert(err, IsNil)

	// Check that an error returned from the mutate function
	// is returned from Change.
	changeErr := errors.New("an error")
	err = conf.Change(func(c *agent.Conf) error {
		c.StateInfo.EntityName = "not-written"
		return changeErr
	})
	c.Assert(err, Equals, changeErr)

	// Check that an invalid change yields an error.
	err = conf.Change(func(c *agent.Conf) error {
		c.StateInfo.Addrs = nil
		return nil
	})
	c.Assert(err, ErrorMatches, "state server address not found in configuration")

	// Check the file hasn't changed
	conf2, err := agent.ReadConf(dataDir, "bar")
	c.Assert(err, IsNil)

	c.Assert(conf2, DeepEquals, &conf)
}

type openSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&openSuite{})

func (s *openSuite) TestOpenStateNormal(c *C) {
	conf := agent.Conf{
		StateInfo: *s.StateInfo(c),
	}
	conf.OldPassword = "irrelevant"

	st, changed, err := conf.OpenState()
	c.Assert(err, IsNil)
	defer st.Close()
	c.Assert(changed, Equals, false)
	c.Assert(st, NotNil)
}

func (s *openSuite) TestOpenStateFallbackPassword(c *C) {
	conf := agent.Conf{
		StateInfo: *s.StateInfo(c),
	}
	conf.OldPassword = conf.StateInfo.Password
	conf.StateInfo.Password = "not the right password"

	st, changed, err := conf.OpenState()
	c.Assert(err, IsNil)
	defer st.Close()
	c.Assert(changed, Equals, true)
	c.Assert(st, NotNil)
	p, err := trivial.RandomPassword()
	c.Assert(err, IsNil)
	c.Assert(conf.StateInfo.Password, HasLen, len(p))
	c.Assert(conf.OldPassword, Equals, s.StateInfo(c).Password)
}

func (s *openSuite) TestOpenStateNoPassword(c *C) {
	conf := agent.Conf{
		StateInfo: *s.StateInfo(c),
	}
	conf.OldPassword = conf.StateInfo.Password
	conf.StateInfo.Password = ""

	st, changed, err := conf.OpenState()
	c.Assert(err, IsNil)
	defer st.Close()
	c.Assert(changed, Equals, true)
	c.Assert(st, NotNil)
	p, err := trivial.RandomPassword()
	c.Assert(err, IsNil)
	c.Assert(conf.StateInfo.Password, HasLen, len(p))
	c.Assert(conf.OldPassword, Equals, s.StateInfo(c).Password)
}
