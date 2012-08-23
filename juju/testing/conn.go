package testing

import (
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/juju"
	state "launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"net/url"
	"os"
	"path/filepath"
)

// JujuConnSuite provides a freshly bootstrapped juju.Conn
// for each test. It also includes testing.LoggingSuite.
//
// It also sets up $HOME and environs.VarDir to
// temporary directories; the former is primed to
// hold the dummy environments.yaml file.
//
// The name of the dummy environment is "jujutest".
type JujuConnSuite struct {
	testing.LoggingSuite
	testing.ZkSuite
	Conn   *juju.Conn
	State  *state.State
	home   string
	varDir string
}

var config = []byte(`
environments:
    jujutest:
        type: dummy
        zookeeper: true
        authorized-keys: 'i-am-a-key'
`)

func (s *JujuConnSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)

	s.home = os.Getenv("HOME")
	home := c.MkDir()
	os.Setenv("HOME", home)

	s.varDir = environs.VarDir
	environs.VarDir = c.MkDir()

	err := os.Mkdir(filepath.Join(home, ".juju"), 0777)
	c.Assert(err, IsNil)

	err = ioutil.WriteFile(filepath.Join(home, ".juju", "environments.yaml"), config, 0666)
	c.Assert(err, IsNil)
	conn, err := juju.NewConn("jujutest")
	c.Assert(err, IsNil)

	// sanity check we've got the correct environment.
	c.Assert(conn.Environ.Name(), Equals, "jujutest")

	c.Assert(conn.Bootstrap(false), IsNil)
	s.Conn = conn
	s.State, err = conn.State()
	c.Assert(err, IsNil)
}

func (s *JujuConnSuite) TearDownTest(c *C) {
	dummy.Reset()
	c.Assert(s.Conn.Close(), IsNil)
	s.Conn = nil
	s.State = nil
	os.Setenv("HOME", s.home)
	s.home = ""
	environs.VarDir = s.varDir
	s.varDir = ""
	s.LoggingSuite.TearDownTest(c)
}

// WriteConfig writes a juju config file to the "home" directory.
func (s *JujuConnSuite) WriteConfig(config string) {
	if s.home == "" {
		panic("SetUpTest has not been called; will not overwrite $HOME/.juju/environments.yaml")
	}
	path := filepath.Join(os.Getenv("HOME"), ".juju", "environments.yaml")
	err := ioutil.WriteFile(path, []byte(config), 0666)
	if err != nil {
		panic(err)
	}
}

func (s *JujuConnSuite) StateInfo(c *C) *state.Info {
	return &state.Info{Addrs: []string{testing.ZkAddr}}
}

func (s *JujuConnSuite) AddTestingCharm(c *C, name string) *state.Charm {
	ch := testing.Charms.Dir(name)
	ident := fmt.Sprintf("%s-%d", name, ch.Revision())
	curl := charm.MustParseURL("local:series/" + ident)
	bundleURL, err := url.Parse("http://bundles.example.com/" + ident)
	c.Assert(err, IsNil)
	sch, err := s.State.AddCharm(ch, curl, bundleURL, ident+"-sha256")
	c.Assert(err, IsNil)
	return sch
}
