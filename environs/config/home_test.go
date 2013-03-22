package config_test

import (
	. "launchpad.net/gocheck"
	"os"
	"path/filepath"

	"launchpad.net/juju-core/environs/config"
)

type JujuHomeSuite struct{}

var _ = Suite(&JujuHomeSuite{})

func (s *JujuHomeSuite) SetUpSuite(c *C) {
	err := config.Init()
	c.Assert(err, IsNil)
}

func (s *JujuHomeSuite) TearDownTest(c *C) {
	config.RestoreJujuHome()
}

func (s *JujuHomeSuite) TestStandardHome(c *C) {
	// Environment variable is at least set by the initialization. 
	jujuHome := config.JujuHome()
	jujuHomeVar := os.Getenv("JUJU_HOME")
	c.Assert(jujuHome, Equals, jujuHomeVar)
	// Changing the environment variable has no effect. 
	err := os.Setenv("JUJU_HOME", "")
	c.Assert(err, IsNil)
	jujuHome = config.JujuHome()
	c.Assert(jujuHome, Equals, jujuHomeVar)
}

func (s *JujuHomeSuite) TestErrorHome(c *C) {
	defer config.Init()
	config.ResetJujuHome()
	homeVar := os.Getenv("HOME")
	os.Setenv("HOME", "")
	defer os.Setenv("HOME", homeVar)
	jujuHomeVar := os.Getenv("JUJU_HOME")
	os.Setenv("JUJU_HOME", "")
	defer os.Setenv("JUJU_HOME", jujuHomeVar)
	// Init() needs $JUJU_HOME or $HOME.
	err := config.Init()
	c.Assert(err, ErrorMatches, "cannot determine juju home, neither \\$JUJU_HOME nor \\$HOME are set")
	// Invalid (or missing) Init() leads to panic when retrieving, setting or restoring.
	f := func() { _ = config.JujuHome() }
	c.Assert(f, PanicMatches, "juju home hasn't been initialized")
	f = func() { _ = config.SetTestJujuHome("/somwhere/in/the/filesystem") }
	c.Assert(f, PanicMatches, "juju home hasn't been initialized")
	f = func() { _ = config.RestoreJujuHome() }
	c.Assert(f, PanicMatches, "juju home hasn't been initialized")
}

func (s *JujuHomeSuite) TestHomePath(c *C) {
	envPath := config.JujuHomePath("environments.yaml")
	c.Assert(envPath, Equals, filepath.Join(config.JujuHome(), "environments.yaml"))
}

func (s *JujuHomeSuite) TestTestHome(c *C) {
	jujuHome := config.JujuHome()
	testJujuHome := c.MkDir()
	origJujuHome := config.SetTestJujuHome(testJujuHome)
	c.Assert(jujuHome, Equals, origJujuHome)
	jujuHomeVar := os.Getenv("JUJU_HOME")
	c.Assert(jujuHomeVar, Equals, testJujuHome)
	jujuHome = config.JujuHome()
	c.Assert(jujuHome, Equals, testJujuHome)
	jujuHome = config.RestoreJujuHome()
	c.Assert(jujuHome, Equals, origJujuHome)
}
