package config_test

import (
	. "launchpad.net/gocheck"
	"os"
	"path/filepath"

	"launchpad.net/juju-core/environs/config"
)

type JujuHomeSuite struct{}

var _ = Suite(&JujuHomeSuite{})

func (s *JujuHomeSuite) TestStandardHome(c *C) {
	c.Assert(config.Init(), IsNil)
	jujuHome := config.JujuHome()
	homeVar := os.Getenv("HOME")
	jujuHomeVar := os.Getenv("JUJU_HOME")
	if jujuHomeVar != "" {
		c.Assert(jujuHome, Equals, jujuHomeVar)
	} else {
		c.Assert(jujuHome, Equals, filepath.Join(homeVar, ".juju"))
	}
}

func (s *JujuHomeSuite) TestErrorHome(c *C) {
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
}

func (s *JujuHomeSuite) TestHomePath(c *C) {
	envPath := config.JujuHomePath("environments.yaml")
	c.Assert(envPath, Equals, filepath.Join(config.JujuHome(), "environments.yaml"))
}

func (s *JujuHomeSuite) TestFakeHome(c *C) {
	// Init needed to retrieve the original value and check
	// the restoring.
	c.Assert(config.Init(), IsNil)
	origJujuHome := config.JujuHome()
	fakeJujuHome := c.MkDir()
	fake := config.SetFakeJujuHome(fakeJujuHome)
	jujuHome := config.JujuHome()
	c.Assert(jujuHome, Equals, fakeJujuHome)
	fake.Restore()
	jujuHome = config.JujuHome()
	c.Assert(jujuHome, Equals, origJujuHome)
}
