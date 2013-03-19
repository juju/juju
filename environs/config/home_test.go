package config_test

import (
	. "launchpad.net/gocheck"
	"os"
	"path/filepath"

	"launchpad.net/juju-core/environs/config"
)

type JujuHomeSuite struct {
	origJujuHome string
}

var _ = Suite(&JujuHomeSuite{})

func (s *JujuHomeSuite) SetUpSuite(c *C) {
	s.origJujuHome = config.JujuHome()
}

func (s *JujuHomeSuite) SetUpTest(c *C) {
	config.RestoreJujuHome()
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

func (s *JujuHomeSuite) TestHomePath(c *C) {
	envPath := config.JujuHomePath("environments.yaml")
	c.Assert(envPath, Equals, filepath.Join(s.origJujuHome, "environments.yaml"))
}

func (s *JujuHomeSuite) TestTestHome(c *C) {
	jujuHome := config.JujuHome()
	testJujuHome := c.MkDir()
	origJujuHome := config.SetTestJujuHome(testJujuHome)
	c.Assert(jujuHome, Equals, origJujuHome)
	jujuHome = config.JujuHome()
	c.Assert(jujuHome, Equals, testJujuHome)
	jujuHome = config.RestoreJujuHome()
	c.Assert(jujuHome, Equals, origJujuHome)
}
