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
	"os"
	"path/filepath"
)

// JujuConnSuite provides a freshly bootstrapped juju.Conn
// for each test. It also includes testing.LoggingSuite.
//
// It also sets up RootDir to point to a directory hierarchy
// mirroring the intended juju directory structure, including
// the following:
// 	RootDir/home/ubuntu/.juju/environments.yaml
//		The dummy environments.yaml file, holding
//		a default environment named "dummyenv"
//		which uses the "dummy" environment type.
//	RootDir/var/lib/juju
//		An empty directory returned as DataDir - the
//		root of the juju data storage space.
// $HOME is set to point to RootDir/home/ubuntu.
type JujuConnSuite struct {
	testing.LoggingSuite
	testing.ZkSuite
	Conn    *juju.Conn
	State   *state.State
	RootDir string // The faked-up root directory.
	oldHome string
}

var config = []byte(`
environments:
    dummyenv:
        type: dummy
        zookeeper: true
        authorized-keys: 'i-am-a-key'
        default-series: decrepit
`)

func (s *JujuConnSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	s.ZkSuite.SetUpSuite(c)
}

func (s *JujuConnSuite) TearDownSuite(c *C) {
	s.ZkSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *JujuConnSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.ZkSuite.SetUpTest(c)
	s.setUpConn(c)
}

func (s *JujuConnSuite) TearDownTest(c *C) {
	s.tearDownConn(c)
	s.ZkSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

// Reset returns environment state to that which existed at the start of
// the test.
func (s *JujuConnSuite) Reset(c *C) {
	s.tearDownConn(c)
	s.setUpConn(c)
}

func (s *JujuConnSuite) setUpConn(c *C) {
	if s.RootDir != "" {
		panic("JujuConnSuite.setUpConn without teardown")
	}
	s.RootDir = c.MkDir()
	s.oldHome = os.Getenv("HOME")
	home := filepath.Join(s.RootDir, "/home/ubuntu")
	err := os.MkdirAll(home, 0777)
	c.Assert(err, IsNil)
	os.Setenv("HOME", home)

	dataDir := filepath.Join(s.RootDir, "/var/lib/juju")
	err = os.MkdirAll(dataDir, 0777)
	c.Assert(err, IsNil)

	err = os.Mkdir(filepath.Join(home, ".juju"), 0777)
	c.Assert(err, IsNil)

	err = ioutil.WriteFile(filepath.Join(home, ".juju", "environments.yaml"), config, 0600)
	c.Assert(err, IsNil)

	environ, err := environs.NewFromName("dummyenv")
	c.Assert(err, IsNil)
	// sanity check we've got the correct environment.
	c.Assert(environ.Name(), Equals, "dummyenv")
	c.Assert(environ.Bootstrap(false), IsNil)

	conn, err := juju.NewConnFromName("dummyenv")
	c.Assert(err, IsNil)
	s.Conn = conn
	s.State = conn.State
	c.Assert(err, IsNil)
}

func (s *JujuConnSuite) tearDownConn(c *C) {
	dummy.Reset()
	c.Assert(s.Conn.Close(), IsNil)
	s.Conn = nil
	s.State = nil
	os.Setenv("HOME", s.oldHome)
	s.oldHome = ""
	s.RootDir = ""
}

func (s *JujuConnSuite) DataDir() string {
	if s.RootDir == "" {
		panic("DataDir called out of test context")
	}
	return filepath.Join(s.RootDir, "/var/lib/juju")
}

// WriteConfig writes a juju config file to the "home" directory.
func (s *JujuConnSuite) WriteConfig(config string) {
	if s.RootDir == "" {
		panic("SetUpTest has not been called; will not overwrite $HOME/.juju/environments.yaml")
	}
	path := filepath.Join(os.Getenv("HOME"), ".juju", "environments.yaml")
	err := ioutil.WriteFile(path, []byte(config), 0600)
	if err != nil {
		panic(err)
	}
}

func (s *JujuConnSuite) StateInfo(c *C) *state.Info {
	return &state.Info{Addrs: []string{testing.ZkAddr}}
}

func (s *JujuConnSuite) AddTestingCharm(c *C, name string) *state.Charm {
	ch := testing.Charms.Dir("series", name)
	ident := fmt.Sprintf("%s-%d", name, ch.Revision())
	curl := charm.MustParseURL("local:series/" + ident)
	repo, err := charm.InferRepository(curl, testing.Charms.Path)
	c.Assert(err, IsNil)
	sch, err := s.Conn.PutCharm(curl, repo, false)
	c.Assert(err, IsNil)
	return sch
}
