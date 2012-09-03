package main

import (
	"bytes"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
)

type UpgradeJujuSuite struct {
	repoSuite
}

var _ = Suite(&UpgradeJujuSuite{})

var upgradeJujuTests = []struct {
	about string
	private []string
	public []string
	currentVersion string
	agentVersion string

	args []string
	expectVersion string
	expectDevVersion bool
	expectPrivate []string
	err string
} {{
	about: "default upgrade from public storage",
	public: []string{"2.0.0-foo-bar", "2.0.2-arble-bletch", "2.0.3-foo-bar"},
	currentVersion: "2.0.0-foo-bar",
	agentVersion: "2.0.0-foo-bar",
	expectVersion: "2.0.2",
}, {
	about: "current dev version upgrade from public storage",
	public: []string{"2.0.0-foo-bar", "2.0.2-arble-bletch", "2.0.3-foo-bar"},
	currentVersion: "2.0.1-foo-bar",
	agentVersion: "2.0.0-foo-bar",
	expectVersion: "2.0.3",
},  {
	about: "dev version flag upgrade from public storage",
	public: []string{"2.0.0-foo-bar", "2.0.2-arble-bletch", "2.0.3-foo-bar"},
	currentVersion: "2.0.0-foo-bar",
	args: "--dev",
	agentVersion: "2.0.0-foo-bar",
	expectVersion: "2.0.3",
}, {
	about: "current dev version upgrade from private storage",
	private: []string{"2.0.0-foo-bar", "2.0.2-foo-bar", "2.0.3-foo-bar"},
	public: []string{"2.0.0-foo-bar", "2.0.4-foo-bar", "2.0.5-foo-bar"},
	currentVersion: "2.0.1-foo-bar",
	agentVersion: "2.0.0-foo-bar",
	expectVersion: "2.0.5",
},  
 {
	about: "default upgrade from private storage",
	private: []string{"2.0.0-foo-bar", "2.0.2-foo-bar", "2.0.3-foo-bar"},
	public: []string{"2.0.0-foo-bar", "2.0.4-foo-bar", "2.0.5-foo-bar"},
	currentVersion: "2.0.0-foo-bar",
	agentVersion: "2.0.0-foo-bar",
	expectVersion: "2.0.2",
}, 

func (s *UpgradeJujuSuite) TestVanillaUpgrade(c *C) {
	m0, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	m1, err := s.State.AddMachine()
	c.Assert(err, IsNil)
}


test with already uploaded tools in various configurations.

test with uploading, and with bump version
