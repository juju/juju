package agent_test
import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/agent"
	"testing"
)

type suite struct{}

func Test(t *testing.T) {
	TestingT(t)
}

var _ = Suite(suite{})

var confTests = []struct{
	conf agent.Conf
	checkErr string
}{{
	conf: agent.Conf{
		OldPassword: "old password",
		StateInfo: state.Info{
			Addrs: []string{"foo:355", "bar:545"},
			CACert: []byte{"ca cert"},
			EntityName: "entity",
			Password: "current password",
		},
	},
},
}

func (suite) TestConfReadWriteCheck(c *C) {
	d := c.MkDir()
	dataDir := filepath.Join(d, "data")
	for i, test := range confTests {
		err := os.Mkdir(dataDir, 0777)
		c.Assert(err, IsNil)
		conf := test.conf
		conf.DataDir = dataDir
		err = test.conf.Check()
		if test.checkErr != "" {
			c.Assert(err, ErrorMatches, test.checkErr)
			continue
		}
		c.Assert(err, IsNil)
		err = conf.Write()
		c.Assert(err, IsNil)
		_, err = os.Stat(conf.File("agent.conf"))
		c.Assert(err, IsNil)

		// move the configuration file to a different directory
		// to check that the entity name gets set correctly when
		// reading.
		newDir := filepath.Join(dataDir, "agents", "another")
		err = os.Mkdir(newDir)
		c.Assert(err, IsNil)
		err = os.Rename(conf.File("agent.conf"), filepath.Join(newDir, "agent.conf"))
		c.Assert(err, IsNil)

		rconf, err := agent.ReadConf(dataDir, "another")
		c.Assert(err, IsNil)
		c.Assert(rconf.StateInfo.EntityName, Equals, "another")
		rconf.StateInfo.EntityName = conf.EntityName
		c.Assert(rconf, DeepEquals, conf)
	}
}

func (suite) TestConfDir(c *C) {
	conf := agent.Conf{
		DataDir: "/foo",
		StateInfo: state.Info{
			Addrs: []string{"x:4"},
			CACert: []byte("xxx"),
			EntityName: "bar",
			Password: "pass",
		},
	}
	c.Assert(conf.Dir(), Equals, "/foo/agents/bar")
}

func (suite) TestConfFile(c *C) {
	conf := agent.Conf{
		DataDir: "/foo",
		StateInfo: state.Info{
			Addrs: []string{"x:4"},
			CACert: []byte("xxx"),
			EntityName: "bar",
			Password: "pass",
		},
	}
	c.Assert(conf.File("x/y"), Equals, "/foo/agents/bar/x/y")
}
