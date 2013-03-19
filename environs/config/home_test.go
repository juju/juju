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

func (s *JujuHomeSuite) TearDownSuite(c *C) {
	config.RestoreJujuHome()
}

func (s *JujuHomeSuite) SetUpTest(c *C) {
	config.RestoreJujuHome()
}

func (s *JujuHomeSuite) TearDownTest(c *C) {
	err := os.Setenv("JUJU_HOME", s.origJujuHome)
	c.Assert(err, IsNil)
}

func (s *JujuHomeSuite) TestStandardHome(c *C) {
	home := os.Getenv("HOME")
	err := os.Setenv("JUJU_HOME", "")
	c.Assert(err, IsNil)
	jujuHome := config.RestoreJujuHome()
	newJujuHome := filepath.Join(home, ".juju")
	c.Assert(jujuHome, Equals, newJujuHome)
	jujuHome = config.JujuHome()
	c.Assert(jujuHome, Equals, newJujuHome)
	testJujuHome := c.MkDir()
	err = os.Setenv("JUJU_HOME", testJujuHome)
	jujuHome = config.RestoreJujuHome()
	c.Assert(jujuHome, Equals, testJujuHome)
	jujuHome = config.JujuHome()
	c.Assert(jujuHome, Equals, testJujuHome)
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
